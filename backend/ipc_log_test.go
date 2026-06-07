package backend

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestFormatIPCParamsRedactsSecrets(t *testing.T) {
	t.Parallel()

	login, _ := json.Marshal(repeaterLoginParams{Repeater: "mc.example", Password: "secret"})
	if got := formatIPCParams("repeater_login", login); !strings.Contains(got, "password=<redacted>") || strings.Contains(got, "secret") {
		t.Fatalf("repeater_login = %q", got)
	}

	send, _ := json.Marshal(sendTextParams{Recipient: "alice", Text: "hello"})
	if got := formatIPCParams("send_text", send); got != "recipient=alice text_len=5" {
		t.Fatalf("send_text = %q", got)
	}

	raw, _ := json.Marshal(rawParams{Payload: []byte{1, 2, 3}})
	if got := formatIPCParams("raw_send", raw); got != "payload_len=3" {
		t.Fatalf("raw_send = %q", got)
	}
}

func TestFormatIPCParamsTruncatesLongJSON(t *testing.T) {
	t.Parallel()

	params, _ := json.Marshal(map[string]string{"query": strings.Repeat("x", 300)})
	got := formatIPCParams("contact", params)
	if len(got) != 203 || !strings.HasSuffix(got, "...") {
		t.Fatalf("truncated length = %d suffix=%q", len(got), got[len(got)-3:])
	}
}

func TestFormatIPCDurationMs(t *testing.T) {
	t.Parallel()

	if got := formatIPCDurationMs(450 * time.Microsecond); got != "1ms" {
		t.Fatalf("got %q, want 1ms", got)
	}
	if got := formatIPCDurationMs(450 * time.Millisecond); got != "450ms" {
		t.Fatalf("got %q, want 450ms", got)
	}
	if got := formatIPCDurationMs(1234 * time.Millisecond); got != "1234ms" {
		t.Fatalf("got %q, want 1234ms", got)
	}
}
