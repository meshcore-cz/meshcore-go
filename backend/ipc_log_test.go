package backend

import (
	"encoding/json"
	"strings"
	"testing"
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
