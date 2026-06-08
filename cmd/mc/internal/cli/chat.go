package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	meshcore "github.com/meshcore-cz/meshcore-go"
	localbackend "github.com/meshcore-cz/meshcore-go/backend"
)

// chatTarget identifies the contact or channel a chat is bound to.
type chatTarget struct {
	name         string
	isChannel    bool
	peerKey      string // contact full public key (direct chats)
	keyPrefix    string // 12-char identity prefix shown in the header
	channelIndex int    // device channel slot (channel chats)
	channelName  string // raw channel name (channel chats)
}

// matchesEvent reports whether a received message (identified by the sender
// field and channel slot from a live event) belongs to this chat target.
func (t chatTarget) matchesEvent(from, channel string) bool {
	if t.isChannel {
		return channel == strconv.Itoa(t.channelIndex)
	}
	if channel != "" {
		return false
	}
	// Direct messages carry the sender's key prefix; match it against the
	// contact's public key.
	return from != "" && strings.HasPrefix(strings.ToLower(t.peerKey), strings.ToLower(from))
}

// chatSession is the data surface the chat UI talks to. Two implementations
// exist: one over the local backend (with persisted history) and a direct
// radio connection (live messages only). History is loaded once via load; new
// activity arrives on the events stream so the UI never re-queries storage.
type chatSession interface {
	info() chatTarget
	selfName() string
	// load returns the conversation history once, oldest first.
	load(ctx context.Context) ([]chatMessage, error)
	// send transmits text to the target and echoes it onto events on success.
	send(ctx context.Context, text string) error
	// events streams live messages and acknowledgements. It is closed when the
	// underlying stream ends.
	events() <-chan chatStreamEvent
	close() error
}

// targetResolver looks up a contact or channel by name/prefix. Both the backend
// IPC client and a direct meshcore.Client satisfy it.
type targetResolver interface {
	Contact(ctx context.Context, query string) (meshcore.Contact, error)
	Channel(ctx context.Context, query string) (meshcore.Channel, error)
}

func chatKeyPrefix(key string) string {
	k := strings.ToLower(strings.TrimSpace(key))
	if k == "" {
		return ""
	}
	if len(k) > 12 {
		return k[:12]
	}
	return k
}

func chatChannelKeyPrefix(ch meshcore.Channel) string {
	secret := ch.Secret
	if len(secret) == 0 && ch.Name != "" {
		secret = meshcore.DeriveChannelSecret(ch.Name)
	}
	if len(secret) == 0 {
		return ""
	}
	sum := sha256.Sum256(secret)
	return chatKeyPrefix(hex.EncodeToString(sum[:]))
}

func resolveChatTarget(ctx context.Context, r targetResolver, query string) (chatTarget, error) {
	if ct, err := r.Contact(ctx, query); err == nil && ct.PublicKey != "" {
		return chatTarget{
			name:    ct.Name,
			peerKey: ct.PublicKey,
			keyPrefix: chatKeyPrefix(ct.PublicKey),
		}, nil
	}
	if ch, err := r.Channel(ctx, query); err == nil {
		return chatTarget{
			name:         meshcore.ChannelDisplayName(ch.Name),
			isChannel:    true,
			channelIndex: ch.Index,
			channelName:  ch.Name,
			keyPrefix:    chatChannelKeyPrefix(ch),
		}, nil
	}
	return chatTarget{}, fmt.Errorf("no contact or channel matching %q", query)
}

// cmdChat implements `mc chat <target>`.
func cmdChat(ctx context.Context, e *env) error {
	target := e.restArg(0)
	if target == "" {
		return fmt.Errorf("usage: mc chat <contact|channel>")
	}
	if e.out.JSON {
		return fmt.Errorf("mc chat is interactive and cannot be used with --json")
	}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()

	session, err := newChatSession(ctx, e, target)
	if err != nil {
		return err
	}
	defer session.close()

	model := newChatModel(ctx, session)
	prog := tea.NewProgram(model, tea.WithAltScreen(), tea.WithContext(ctx))
	_, err = prog.Run()
	return err
}

// newChatSession builds a backend-backed session when the local daemon is
// healthy, otherwise falls back to a direct radio connection.
func newChatSession(ctx context.Context, e *env, target string) (chatSession, error) {
	if preferIPCBackend(e) {
		client := backendClientForEnv(e)
		st, err := client.Status(ctx)
		if err == nil && st.Healthy {
			tgt, terr := resolveChatTarget(ctx, client, target)
			if terr != nil {
				return nil, terr
			}
			return newIPCChatSession(ctx, client, tgt, resolveIPCSelfName(ctx, client)), nil
		}
		if e.exec.RequireIPC {
			if err != nil {
				return nil, fmt.Errorf("shell backend unavailable: %w", err)
			}
			return nil, fmt.Errorf("shell backend unavailable: backend is %s", st.State)
		}
		e.dbg.Log("ipc backend unavailable for chat", "error", err)
	}

	uri, _, err := resolveURI(e)
	if err != nil {
		return nil, err
	}
	opts := append(e.dbg.DialOptions(), meshcore.WithClientOptions(meshcore.WithMessageSync()))
	client, err := meshcore.Dial(ctx, uri, opts...)
	if err != nil {
		return nil, fmt.Errorf("connecting to %s: %w", uri, err)
	}
	tgt, terr := resolveChatTarget(ctx, client, target)
	if terr != nil {
		client.Close()
		return nil, terr
	}
	return newDirectChatSession(ctx, client, tgt, resolveDirectSelfName(ctx, client)), nil
}

func resolveIPCSelfName(ctx context.Context, client *localbackend.Client) string {
	if info, err := client.DeviceInfo(ctx); err == nil {
		if name := strings.TrimSpace(info.Name); name != "" {
			return name
		}
	}
	if st, err := client.Status(ctx); err == nil {
		if name := strings.TrimSpace(st.Device.Name); name != "" {
			return name
		}
	}
	return ""
}

func resolveDirectSelfName(ctx context.Context, client *meshcore.Client) string {
	if info, err := client.DeviceInfo(ctx); err == nil {
		return strings.TrimSpace(info.Name)
	}
	return ""
}

// extractChannelAuthor splits "name: body" channel message text. Channel
// messages carry the sender in the text; returns ("", text) when no prefix.
func extractChannelAuthor(text string) (author, body string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", text
	}
	first := text
	if idx := strings.IndexByte(first, '\n'); idx >= 0 {
		first = first[:idx]
	}
	colon := strings.Index(first, ": ")
	if colon <= 0 {
		return "", text
	}
	author = strings.TrimSpace(first[:colon])
	if author == "" {
		return "", text
	}
	rest := strings.TrimSpace(first[colon+2:])
	if len(text) > len(first) {
		rest += text[len(first):]
	}
	return author, strings.TrimSpace(rest)
}

func incomingChannelMessage(text string, ts time.Time, snr float64) chatMessage {
	sender, body := extractChannelAuthor(text)
	return chatMessage{sender: sender, text: body, ts: ts, snr: snr}
}

// chatMessageFromRecord converts a stored backend message record to a UI line.
func chatMessageFromRecord(rec localbackend.MessageRecord, tgt chatTarget) chatMessage {
	mine := rec.Direction == localbackend.MessageOut
	sender := rec.PeerName
	if !mine && sender == "" {
		sender = rec.Peer
	}
	text := rec.Text
	if tgt.isChannel && !mine {
		if author, body := extractChannelAuthor(text); author != "" {
			sender = author
			text = body
		}
	}
	status := rec.Status
	if status == localbackend.StatusReceived {
		status = ""
	}
	return chatMessage{
		mine:    mine,
		sender:  sender,
		text:    text,
		ts:      rec.Timestamp,
		status:  status,
		ackCode: rec.AckCode,
		snr:     rec.SNR,
	}
}

// ipcChatSession serves history from the backend and streams live updates from
// the backend's event watch. The store is read exactly once (load); thereafter
// new messages and acks arrive on the watch stream, never by re-querying.
type ipcChatSession struct {
	client   *localbackend.Client
	target   chatTarget
	self     string
	filter   localbackend.MessageFilter
	ctx      context.Context
	cancel   context.CancelFunc
	ch       chan chatStreamEvent
}

func newIPCChatSession(ctx context.Context, client *localbackend.Client, tgt chatTarget, selfName string) *ipcChatSession {
	filter := localbackend.MessageFilter{}
	if tgt.isChannel {
		filter.Kind = localbackend.MessageChannel
		filter.Channel = strconv.Itoa(tgt.channelIndex)
	} else {
		filter.Kind = localbackend.MessageDirect
		filter.Peer = tgt.peerKey
	}

	watchCtx, cancel := context.WithCancel(ctx)
	s := &ipcChatSession{
		client: client,
		target: tgt,
		self:   selfName,
		filter: filter,
		ctx:    watchCtx,
		cancel: cancel,
		ch:     make(chan chatStreamEvent, 64),
	}
	go s.watch()
	return s
}

func (s *ipcChatSession) info() chatTarget  { return s.target }
func (s *ipcChatSession) selfName() string  { return s.self }

func (s *ipcChatSession) load(ctx context.Context) ([]chatMessage, error) {
	records, err := s.client.Messages(ctx, s.filter)
	if err != nil {
		return nil, err
	}
	out := make([]chatMessage, 0, len(records))
	for _, rec := range records {
		out = append(out, chatMessageFromRecord(rec, s.target))
	}
	return out, nil
}

func (s *ipcChatSession) send(ctx context.Context, text string) error {
	var receipt meshcore.Receipt
	var err error
	if s.target.isChannel {
		receipt, err = s.client.SendChannelText(ctx, strconv.Itoa(s.target.channelIndex), text)
	} else {
		receipt, err = s.client.SendText(ctx, s.target.peerKey, text)
	}
	if err != nil {
		return err
	}
	msg := chatMessage{mine: true, text: text, ts: time.Now(), status: localbackend.StatusSent}
	if !s.target.isChannel {
		msg.ackCode = receipt.ID()
	}
	s.push(chatStreamEvent{message: &msg})
	return nil
}

func (s *ipcChatSession) events() <-chan chatStreamEvent { return s.ch }

// watch forwards matching backend message and ack events onto the session
// stream. The event payload, not the store, drives live updates.
func (s *ipcChatSession) watch() {
	defer close(s.ch)
	events, err := s.client.Watch(s.ctx)
	if err != nil {
		return
	}
	for ev := range events {
		switch ev.Type {
		case "message":
			if !s.target.matchesEvent(ev.From, ev.Channel) {
				continue
			}
			var msg chatMessage
			if s.target.isChannel {
				msg = incomingChannelMessage(ev.Text, ev.Timestamp, 0)
			} else {
				msg = chatMessage{sender: s.target.name, text: ev.Text, ts: ev.Timestamp}
			}
			s.push(chatStreamEvent{message: &msg})
		case "ack":
			if ev.Code != "" {
				s.push(chatStreamEvent{ackCode: ev.Code})
			}
		}
	}
}

func (s *ipcChatSession) push(ev chatStreamEvent) {
	select {
	case s.ch <- ev:
	case <-s.ctx.Done():
	}
}

func (s *ipcChatSession) close() error {
	s.cancel()
	return nil
}

// directChatSession holds a live radio connection and streams messages from the
// SDK event channel. It has no persisted history.
type directChatSession struct {
	client *meshcore.Client
	target chatTarget
	self   string
	ctx    context.Context
	cancel context.CancelFunc
	ch     chan chatStreamEvent
}

func newDirectChatSession(ctx context.Context, client *meshcore.Client, tgt chatTarget, selfName string) *directChatSession {
	watchCtx, cancel := context.WithCancel(ctx)
	s := &directChatSession{
		client: client,
		target: tgt,
		self:   selfName,
		ctx:    watchCtx,
		cancel: cancel,
		ch:     make(chan chatStreamEvent, 64),
	}
	go s.watch()
	return s
}

func (s *directChatSession) info() chatTarget { return s.target }
func (s *directChatSession) selfName() string { return s.self }

// load has no persisted history without a backend.
func (s *directChatSession) load(context.Context) ([]chatMessage, error) {
	return nil, nil
}

func (s *directChatSession) send(ctx context.Context, text string) error {
	var receipt meshcore.Receipt
	var err error
	if s.target.isChannel {
		receipt, err = s.client.SendChannelText(ctx, strconv.Itoa(s.target.channelIndex), text)
	} else {
		receipt, err = s.client.SendText(ctx, s.target.peerKey, text)
	}
	if err != nil {
		return err
	}
	msg := chatMessage{mine: true, text: text, ts: time.Now(), status: "sent"}
	if !s.target.isChannel {
		msg.ackCode = receipt.ID()
	}
	s.push(chatStreamEvent{message: &msg})
	return nil
}

func (s *directChatSession) events() <-chan chatStreamEvent { return s.ch }

// watch reads live SDK events and forwards those addressed to this target.
func (s *directChatSession) watch() {
	defer close(s.ch)
	events := s.client.Events()
	for {
		select {
		case <-s.ctx.Done():
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			switch m := ev.(type) {
			case meshcore.MessageReceived:
				if !s.target.matchesEvent(m.From.Name, m.Channel) {
					continue
				}
				var msg chatMessage
				if s.target.isChannel {
					msg = incomingChannelMessage(m.Text, m.Timestamp, 0)
				} else {
					msg = chatMessage{sender: s.target.name, text: m.Text, ts: m.Timestamp}
				}
				s.push(chatStreamEvent{message: &msg})
			case meshcore.MessageAcknowledged:
				if m.Code != "" {
					s.push(chatStreamEvent{ackCode: m.Code})
				}
			}
		}
	}
}

func (s *directChatSession) push(ev chatStreamEvent) {
	select {
	case s.ch <- ev:
	case <-s.ctx.Done():
	}
}

func (s *directChatSession) close() error {
	s.cancel()
	return s.client.Close()
}
