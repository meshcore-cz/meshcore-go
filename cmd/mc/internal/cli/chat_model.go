package cli

import (
	"context"
	"fmt"
	"hash/fnv"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	localbackend "github.com/meshcore-cz/meshcore-go/backend"
)

// chatMessage is one rendered line of conversation history.
type chatMessage struct {
	mine    bool
	sender  string
	text    string
	ts      time.Time
	status  string // delivery status for outgoing messages
	ackCode string // ack code for outgoing messages, matched against ack events
	snr     float64
}

// chatStreamEvent is one live update from the session: either a new message to
// append or an acknowledgement upgrading a sent message to delivered.
type chatStreamEvent struct {
	message *chatMessage
	ackCode string
}

// tea messages exchanged inside the chat program.
type (
	chatLoadedMsg  struct{ msgs []chatMessage }
	chatLoadErrMsg struct{ err error }
	chatStreamMsg  struct{ ev chatStreamEvent }
	chatClosedMsg  struct{}
	chatSentMsg    struct {
		text string
		err  error
	}
)

var (
	chatHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	chatHintStyle   = lipgloss.NewStyle().Faint(true)
	chatTimeStyle   = lipgloss.NewStyle().Faint(true)
	chatStatusStyle = lipgloss.NewStyle().Faint(true)
	chatRuleStyle   = lipgloss.NewStyle().Faint(true)
	chatErrStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
)

// Distinct terminal colors assigned per author name.
var chatAuthorColors = []string{
	"14", "13", "11", "12", "39", "81", "99", "201", "208", "214", "172", "42",
}

func chatAuthorColor(name string) string {
	if name == "" {
		return "14"
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	return chatAuthorColors[h.Sum32()%uint32(len(chatAuthorColors))]
}

func chatAuthorStyle(name string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(chatAuthorColor(name))).Bold(true)
}

var chatMentionPattern = regexp.MustCompile(`@\[([^\]]+)\]`)

func chatMentionStyle(name string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(chatAuthorColor(name)))
}

// chatMentionPlain strips @[name] tags, leaving just the quoted name.
func chatMentionPlain(text string) string {
	return chatMentionPattern.ReplaceAllString(text, "$1")
}

// formatChatMentions replaces @[name] tags with the name in the quoted author's color.
func formatChatMentions(text string) string {
	return chatMentionPattern.ReplaceAllStringFunc(text, func(match string) string {
		sub := chatMentionPattern.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		return chatMentionStyle(sub[1]).Render(sub[1])
	})
}

// chatModel is the bubbletea model backing `mc chat`.
type chatModel struct {
	ctx     context.Context
	session chatSession
	target   chatTarget
	selfName string

	viewport viewport.Model
	input    textinput.Model
	msgs     []chatMessage
	seen     map[string]bool // dedup key -> seen, guards the load/stream race

	ready   bool
	width   int
	height  int
	status  string
	err     error
	sending bool
	now     time.Time // optional clock for rendering; zero uses time.Now()
}

func newChatModel(ctx context.Context, session chatSession) chatModel {
	in := textinput.New()
	in.Placeholder = "Type a message…"
	in.Prompt = "> "
	in.CharLimit = 0
	in.Focus()

	return chatModel{
		ctx:      ctx,
		session:  session,
		target:   session.info(),
		selfName: session.selfName(),
		input:    in,
		seen:     map[string]bool{},
	}
}

func (m chatModel) Init() tea.Cmd {
	return tea.Batch(
		chatLoadCmd(m.ctx, m.session),
		chatListenCmd(m.session.events()),
		textinput.Blink,
	)
}

// chatLoadCmd loads the conversation history once at startup.
func chatLoadCmd(ctx context.Context, s chatSession) tea.Cmd {
	return func() tea.Msg {
		msgs, err := s.load(ctx)
		if err != nil {
			return chatLoadErrMsg{err: err}
		}
		return chatLoadedMsg{msgs: msgs}
	}
}

// chatListenCmd waits for the next live update from the session's event stream.
func chatListenCmd(events <-chan chatStreamEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-events
		if !ok {
			return chatClosedMsg{}
		}
		return chatStreamMsg{ev: ev}
	}
}

// chatSendCmd transmits text to the target.
func chatSendCmd(ctx context.Context, s chatSession, text string) tea.Cmd {
	return func() tea.Msg {
		return chatSentMsg{text: text, err: s.send(ctx, text)}
	}
}

func (m chatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout()
		m.ready = true
		m.refreshViewport()

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			return m, tea.Quit
		case "enter":
			text := strings.TrimSpace(m.input.Value())
			if text == "" || m.sending {
				return m, nil
			}
			m.input.Reset()
			m.sending = true
			m.addMessage(chatMessage{
				mine: true, text: text, ts: time.Now(), status: localbackend.StatusQueued,
			})
			m.refreshViewport()
			return m, chatSendCmd(m.ctx, m.session, text)
		case "pgup", "pgdown", "ctrl+u", "ctrl+d", "home", "end":
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}

	case chatLoadedMsg:
		m.err = nil
		m.msgs = m.msgs[:0]
		for _, msg := range msg.msgs {
			m.addMessage(msg)
		}
		m.refreshViewport()

	case chatLoadErrMsg:
		m.err = msg.err

	case chatStreamMsg:
		m.applyStream(msg.ev)
		m.refreshViewport()
		return m, chatListenCmd(m.session.events())

	case chatClosedMsg:
		m.status = "session closed"

	case chatSentMsg:
		m.sending = false
		if msg.err != nil {
			m.setOutgoingStatus(msg.text, localbackend.StatusFailed)
			m.status = "send failed: " + msg.err.Error()
		} else {
			m.status = ""
		}
		m.refreshViewport()
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// applyStream appends a new message or upgrades a sent message to delivered.
func (m *chatModel) applyStream(ev chatStreamEvent) {
	if ev.message != nil {
		if ev.message.mine {
			m.addOrUpdateOutgoing(*ev.message)
		} else {
			m.addMessage(*ev.message)
		}
	}
	if ev.ackCode != "" {
		for i := range m.msgs {
			if m.msgs[i].mine && m.msgs[i].ackCode == ev.ackCode {
				m.msgs[i].status = "delivered"
			}
		}
	}
}

// addMessage appends a message unless an identical one was already recorded
// (guards against the same message arriving via both the initial load and the
// live stream during the subscription window).
func (m *chatModel) addMessage(msg chatMessage) {
	key := fmt.Sprintf("%t|%d|%s", msg.mine, msg.ts.Unix(), msg.text)
	if m.seen[key] {
		return
	}
	m.seen[key] = true
	m.msgs = append(m.msgs, msg)
}

func (m *chatModel) addOrUpdateOutgoing(msg chatMessage) {
	for i := len(m.msgs) - 1; i >= 0; i-- {
		if !m.msgs[i].mine || m.msgs[i].text != msg.text {
			continue
		}
		if m.msgs[i].status != localbackend.StatusQueued && m.msgs[i].status != localbackend.StatusFailed {
			break
		}
		m.msgs[i].status = msg.status
		m.msgs[i].ackCode = msg.ackCode
		if !msg.ts.IsZero() {
			m.msgs[i].ts = msg.ts
		}
		return
	}
	m.addMessage(msg)
}

func (m *chatModel) setOutgoingStatus(text, status string) {
	for i := len(m.msgs) - 1; i >= 0; i-- {
		if m.msgs[i].mine && m.msgs[i].text == text {
			m.msgs[i].status = status
			return
		}
	}
}

func (m *chatModel) layout() {
	headerHeight := 2
	footerHeight := 2
	vpHeight := m.height - headerHeight - footerHeight
	if vpHeight < 1 {
		vpHeight = 1
	}
	if !m.ready {
		m.viewport = viewport.New(m.width, vpHeight)
	} else {
		m.viewport.Width = m.width
		m.viewport.Height = vpHeight
	}
	m.input.Width = m.width - 4
}

func (m *chatModel) refreshViewport() {
	if !m.ready {
		return
	}
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
}

func (m chatModel) renderMessages() string {
	if len(m.msgs) == 0 {
		return chatHintStyle.Render("No messages yet. Say hello!")
	}
	var b strings.Builder
	for i, msg := range m.msgs {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(m.renderMessage(msg))
	}
	return b.String()
}

func (m chatModel) renderNow() time.Time {
	if !m.now.IsZero() {
		return m.now
	}
	return time.Now()
}

// chatTimestampPlain shows time-only for today's messages and full date-time
// for older ones.
func chatTimestampPlain(ts, now time.Time) string {
	local := ts.Local()
	loc := now.Location()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	msgDay := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	if msgDay.Before(today) {
		return local.Format("2006-01-02 15:04:05")
	}
	return local.Format("15:04:05")
}

func (m chatModel) senderDisplay(msg chatMessage) (plain, styled string) {
	if msg.mine {
		name := m.selfName
		if name == "" {
			name = "you"
		}
		return name, chatAuthorStyle(name).Render(name)
	}
	plain = msg.sender
	if plain == "" {
		plain = m.target.name
	}
	return plain, chatAuthorStyle(plain).Render(plain)
}

func (m chatModel) renderMessage(msg chatMessage) string {
	tsPlain := chatTimestampPlain(msg.ts, m.renderNow())
	ts := chatTimeStyle.Render(tsPlain)

	senderPlain, sender := m.senderDisplay(msg)

	prefixPlain := tsPlain + "  " + senderPlain + "  "
	prefix := ts + "  " + sender + "  "
	suffix := chatMessageSuffix(msg)
	return renderChatLine(prefix, prefixPlain, msg.text, suffix, max(m.viewport.Width, 1))
}

func chatMessageSuffix(msg chatMessage) string {
	if msg.mine && msg.status != "" {
		return statusGlyph(msg.status)
	}
	if !msg.mine && msg.snr != 0 {
		return fmt.Sprintf("%.0f dB", msg.snr)
	}
	return ""
}

func renderChatLine(prefix, prefixPlain, text, suffix string, width int) string {
	prefixWidth := ansi.StringWidth(prefixPlain)
	bodyWidth := width - prefixWidth
	if bodyWidth < 12 {
		bodyWidth = max(width-2, 1)
		prefix = ""
		prefixWidth = 0
	}

	lines := wrapWords(formatChatMentions(text), bodyWidth)
	if len(lines) == 0 {
		lines = []string{""}
	}

	if suffix != "" {
		suffixText := " " + suffix
		last := len(lines) - 1
		if ansi.StringWidth(lines[last])+ansi.StringWidth(suffixText) <= bodyWidth {
			lines[last] += chatStatusStyle.Render(suffixText)
		} else {
			lines = append(lines, chatStatusStyle.Render(suffixText))
		}
	}

	indent := strings.Repeat(" ", prefixWidth)
	for i := range lines {
		if i == 0 {
			lines[i] = prefix + lines[i]
			continue
		}
		lines[i] = indent + lines[i]
	}
	return strings.Join(lines, "\n")
}

func wrapWords(text string, width int) []string {
	if width <= 0 {
		width = 1
	}
	paragraphs := strings.Split(text, "\n")
	lines := make([]string, 0, len(paragraphs))
	for _, p := range paragraphs {
		words := strings.Fields(p)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		line := ""
		for _, word := range words {
			if line == "" {
				if ansi.StringWidth(word) <= width {
					line = word
				} else {
					chunks := hardWrap(word, width)
					lines = append(lines, chunks[:len(chunks)-1]...)
					line = chunks[len(chunks)-1]
				}
				continue
			}
			next := line + " " + word
			if ansi.StringWidth(next) <= width {
				line = next
				continue
			}
			lines = append(lines, line)
			if ansi.StringWidth(word) <= width {
				line = word
				continue
			}
			chunks := hardWrap(word, width)
			lines = append(lines, chunks[:len(chunks)-1]...)
			line = chunks[len(chunks)-1]
		}
		lines = append(lines, line)
	}
	return lines
}

func hardWrap(s string, width int) []string {
	var out []string
	var b strings.Builder
	lineWidth := 0
	for _, r := range s {
		rw := ansi.StringWidth(string(r))
		if lineWidth > 0 && lineWidth+rw > width {
			out = append(out, b.String())
			b.Reset()
			lineWidth = 0
		}
		b.WriteRune(r)
		lineWidth += rw
	}
	out = append(out, b.String())
	return out
}

func statusGlyph(status string) string {
	switch status {
	case "queued":
		return "… sending…"
	case "transmitting", "sending":
		return "↑"
	case "sent":
		return "✓"
	case "delivered":
		return "✓✓"
	case "unconfirmed":
		return "?"
	case "failed":
		return "!"
	default:
		return status
	}
}

func (m chatModel) View() string {
	if !m.ready {
		return "loading chat…"
	}

	titlePlain, titleStyled := m.chatHeaderParts()
	header := renderChatHeader(titlePlain, titleStyled, m.width)
	rule := chatRuleStyle.Render(strings.Repeat("─", max(m.width, 1)))

	status := m.status
	if m.err != nil {
		status = chatErrStyle.Render(m.err.Error())
	}
	if status != "" {
		status = "\n" + status
	}

	return strings.Join([]string{
		header,
		rule,
		m.viewport.View(),
		rule + status,
		m.input.View(),
	}, "\n")
}

func (m chatModel) chatHeaderParts() (plain, styled string) {
	if m.target.isChannel {
		plain = "Channel " + m.target.name
	} else {
		plain = "Chat with " + m.target.name
	}
	styled = chatHeaderStyle.Render(plain)
	if m.target.keyPrefix != "" {
		plain += " " + m.target.keyPrefix
		styled += " " + chatHintStyle.Render(m.target.keyPrefix)
	}
	return plain, styled
}

func renderChatHeader(titlePlain, titleStyled string, width int) string {
	hint := "pgup/pgdn scroll · ctrl+c quit"
	gap := width - ansi.StringWidth(titlePlain) - ansi.StringWidth(hint)
	if gap < 2 {
		return titleStyled
	}
	return titleStyled + strings.Repeat(" ", gap) + chatHintStyle.Render(hint)
}
