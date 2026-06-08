package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/x/ansi"
	meshcore "github.com/meshcore-cz/meshcore-go"
	localbackend "github.com/meshcore-cz/meshcore-go/backend"
)

func TestChatKeyPrefix(t *testing.T) {
	if got := chatKeyPrefix("ABCDEF0123456789"); got != "abcdef012345" {
		t.Fatalf("chatKeyPrefix = %q", got)
	}
	if got := chatKeyPrefix("abc"); got != "abc" {
		t.Fatalf("short key = %q", got)
	}
}

func TestChatHeaderParts(t *testing.T) {
	direct := chatModel{target: chatTarget{name: "bob", keyPrefix: "abcdef012345"}}
	plain, styled := direct.chatHeaderParts()
	if plain != "Chat with bob abcdef012345" {
		t.Fatalf("direct plain = %q", plain)
	}
	if stripANSI(styled) != plain {
		t.Fatalf("direct styled = %q", stripANSI(styled))
	}

	channel := chatModel{target: chatTarget{name: "#test", isChannel: true, keyPrefix: "0123456789ab"}}
	plain, _ = channel.chatHeaderParts()
	if plain != "Channel #test 0123456789ab" {
		t.Fatalf("channel plain = %q", plain)
	}
}

func TestExtractChannelAuthor(t *testing.T) {
	cases := []struct {
		text       string
		wantAuthor string
		wantBody   string
	}{
		{"OK2AWO.meshcore.cz: ping", "OK2AWO.meshcore.cz", "ping"},
		{"Panda: test", "Panda", "test"},
		{"EL Pong: @[Panda] relay", "EL Pong", "@[Panda] relay"},
		{"no colon here", "", "no colon here"},
		{"", "", ""},
		{
			"EL Pong: line one\ncontinuation line",
			"EL Pong",
			"line one\ncontinuation line",
		},
	}
	for _, c := range cases {
		author, body := extractChannelAuthor(c.text)
		if author != c.wantAuthor || body != c.wantBody {
			t.Fatalf("extractChannelAuthor(%q) = (%q, %q), want (%q, %q)",
				c.text, author, body, c.wantAuthor, c.wantBody)
		}
	}
}

func TestChatMessageFromRecordChannelAuthor(t *testing.T) {
	tgt := chatTarget{name: "#test", isChannel: true, channelIndex: 0}
	rec := localbackend.MessageRecord{
		Direction: localbackend.MessageIn,
		Kind:      localbackend.MessageChannel,
		PeerName:  "test",
		Text:      "Panda: hello channel",
		Timestamp: time.Unix(100, 0),
		SNR:       12,
	}
	got := chatMessageFromRecord(rec, tgt)
	if got.sender != "Panda" || got.text != "hello channel" {
		t.Fatalf("channel author extraction = %+v", got)
	}
}

func TestChatMessageFromRecord(t *testing.T) {
	tgt := chatTarget{name: "alice", peerKey: "abc123"}

	in := localbackend.MessageRecord{
		ID: 1, Direction: localbackend.MessageIn, Peer: "abc123", PeerName: "alice",
		Text: "hi", Timestamp: time.Unix(100, 0), SNR: 6, Status: localbackend.StatusReceived,
	}
	got := chatMessageFromRecord(in, tgt)
	if got.mine || got.sender != "alice" || got.text != "hi" || got.snr != 6 {
		t.Fatalf("incoming conversion = %+v", got)
	}
	if got.status != "" {
		t.Fatalf("received status should be cleared, got %q", got.status)
	}

	out := localbackend.MessageRecord{
		ID: 2, Direction: localbackend.MessageOut, Peer: "abc123", PeerName: "alice",
		Text: "yo", Timestamp: time.Unix(200, 0), Status: localbackend.StatusDelivered,
	}
	got = chatMessageFromRecord(out, tgt)
	if !got.mine || got.status != localbackend.StatusDelivered {
		t.Fatalf("outgoing conversion = %+v", got)
	}

	// Incoming without a resolved name falls back to the peer key.
	anon := localbackend.MessageRecord{Direction: localbackend.MessageIn, Peer: "ff00"}
	if got := chatMessageFromRecord(anon, tgt); got.sender != "ff00" {
		t.Fatalf("anon sender = %q, want peer key", got.sender)
	}
}

func TestChatMentionPlain(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"@[Seven] Praha Strahov - OK", "Seven Praha Strahov - OK"},
		{"@[OK2AWO.meshcore.cz] ->278.2km ping", "OK2AWO.meshcore.cz ->278.2km ping"},
		{"no mention here", "no mention here"},
		{"@[a] x @[b]", "a x b"},
	}
	for _, c := range cases {
		if got := chatMentionPlain(c.in); got != c.want {
			t.Fatalf("chatMentionPlain(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFormatChatMentions(t *testing.T) {
	got := stripANSI(formatChatMentions("@[Seven] hi"))
	if got != "Seven hi" {
		t.Fatalf("formatChatMentions = %q, want %q", got, "Seven hi")
	}
}

func TestChatAuthorColorConsistent(t *testing.T) {
	if chatAuthorColor("Panda") != chatAuthorColor("Panda") {
		t.Fatal("same author should always get the same color")
	}
}

func TestChatAuthorColorDistinct(t *testing.T) {
	names := []string{"alice", "bob", "charlie", "Panda", "EL Pong", "OK2AWO.meshcore.cz"}
	colors := make(map[string]struct{}, len(names))
	for _, name := range names {
		colors[chatAuthorColor(name)] = struct{}{}
	}
	if len(colors) < 2 {
		t.Fatal("expected authors to use at least two different colors")
	}
}

func TestChatTimestampPlain(t *testing.T) {
	now := time.Date(2026, 6, 8, 17, 0, 0, 0, time.Local)
	today := time.Date(2026, 6, 8, 16, 16, 7, 0, time.Local)
	older := time.Date(2026, 6, 7, 16, 16, 7, 0, time.Local)

	if got := chatTimestampPlain(today, now); got != "16:16:07" {
		t.Fatalf("today = %q, want 16:16:07", got)
	}
	if got := chatTimestampPlain(older, now); got != "2026-06-07 16:16:07" {
		t.Fatalf("older = %q, want 2026-06-07 16:16:07", got)
	}
}

func TestAddOrUpdateOutgoing(t *testing.T) {
	m := chatModel{seen: map[string]bool{}}
	m.addMessage(chatMessage{
		mine: true, text: "hi", ts: time.Unix(1, 0), status: localbackend.StatusQueued,
	})
	m.addOrUpdateOutgoing(chatMessage{
		mine: true, text: "hi", ts: time.Unix(2, 0),
		status: localbackend.StatusSent, ackCode: "abc",
	})
	if len(m.msgs) != 1 {
		t.Fatalf("len = %d, want 1", len(m.msgs))
	}
	if m.msgs[0].status != localbackend.StatusSent || m.msgs[0].ackCode != "abc" {
		t.Fatalf("updated = %+v", m.msgs[0])
	}
}

func TestChatRenderQueuedOutgoing(t *testing.T) {
	model := chatModel{
		target:   chatTarget{name: "rem"},
		selfName: "tree",
		viewport: viewportWithWidth(72),
		now:      time.Date(2026, 6, 8, 17, 0, 0, 0, time.Local),
	}
	msg := chatMessage{
		mine: true, text: "pending", ts: time.Date(2026, 6, 8, 16, 50, 0, 0, time.Local),
		status: localbackend.StatusQueued,
	}
	got := stripANSI(model.renderMessage(msg))
	if got != "16:50:00  tree  pending … sending…" {
		t.Fatalf("rendered queued = %q", got)
	}
}

func TestChatRenderMessageInlineMetadata(t *testing.T) {
	model := chatModel{
		target:   chatTarget{name: "rem"},
		selfName: "tree",
		viewport: viewportWithWidth(72),
		now:      time.Date(2026, 6, 8, 17, 0, 0, 0, time.Local),
	}
	msg := chatMessage{
		mine:   false,
		sender: "rem",
		text:   "neco",
		ts:     time.Date(2026, 6, 8, 16, 16, 7, 0, time.Local),
		snr:    11,
	}
	got := stripANSI(model.renderMessage(msg))
	if got != "16:16:07  rem  neco 11 dB" {
		t.Fatalf("rendered message = %q", got)
	}

	out := chatMessage{
		mine:   true,
		text:   "delivered message",
		ts:     time.Date(2026, 6, 8, 16, 41, 11, 0, time.Local),
		status: localbackend.StatusDelivered,
	}
	got = stripANSI(model.renderMessage(out))
	if got != "16:41:11  tree  delivered message ✓✓" {
		t.Fatalf("rendered outgoing = %q", got)
	}
}

func TestChatRenderMessageWrapsMetadataWithText(t *testing.T) {
	model := chatModel{
		target:   chatTarget{name: "rem"},
		viewport: viewportWithWidth(54),
		now:      time.Date(2026, 6, 8, 17, 0, 0, 0, time.Local),
	}
	msg := chatMessage{
		sender: "rem",
		text:   "this is a longer message that wraps onto the next line because the terminal is relatively narrow",
		ts:     time.Date(2026, 6, 8, 16, 42, 3, 0, time.Local),
		snr:    10,
	}
	got := stripANSI(model.renderMessage(msg))
	lines := strings.Split(got, "\n")
	if len(lines) < 2 {
		t.Fatalf("message did not wrap:\n%s", got)
	}
	if !strings.HasPrefix(lines[0], "16:42:03  rem  this is a longer message") {
		t.Fatalf("first line = %q", lines[0])
	}
	if !strings.HasSuffix(lines[len(lines)-1], " 10 dB") {
		t.Fatalf("metadata should be attached to final line:\n%s", got)
	}
	if !strings.HasPrefix(lines[1], "               ") {
		t.Fatalf("wrapped line should align under text:\n%s", got)
	}
}

func TestStatusGlyphCompact(t *testing.T) {
	cases := map[string]string{
		"queued":       "… sending…",
		"transmitting": "↑",
		"sent":         "✓",
		"delivered":    "✓✓",
		"failed":       "!",
	}
	for status, want := range cases {
		if got := statusGlyph(status); got != want {
			t.Fatalf("statusGlyph(%q) = %q, want %q", status, got, want)
		}
	}
}

func viewportWithWidth(width int) viewport.Model {
	v := viewport.New(width, 10)
	v.Width = width
	return v
}

func stripANSI(s string) string {
	return ansi.Strip(s)
}

func TestDirectChatSessionMatches(t *testing.T) {
	direct := chatTarget{peerKey: "abcdef0123456789"}
	channel := chatTarget{isChannel: true, channelIndex: 2}

	cases := []struct {
		name string
		tgt  chatTarget
		m    meshcore.MessageReceived
		want bool
	}{
		{"direct sender prefix", direct, meshcore.MessageReceived{From: meshcore.Contact{Name: "abcdef01"}}, true},
		{"direct wrong sender", direct, meshcore.MessageReceived{From: meshcore.Contact{Name: "deadbeef"}}, false},
		{"direct ignores channel msg", direct, meshcore.MessageReceived{Channel: "0", From: meshcore.Contact{Name: "abcdef01"}}, false},
		{"channel match", channel, meshcore.MessageReceived{Channel: "2"}, true},
		{"channel mismatch", channel, meshcore.MessageReceived{Channel: "1"}, false},
	}
	for _, c := range cases {
		if got := c.tgt.matchesEvent(c.m.From.Name, c.m.Channel); got != c.want {
			t.Errorf("%s: matches = %v, want %v", c.name, got, c.want)
		}
	}
}
