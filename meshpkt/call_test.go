package meshpkt

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCallJSON_encodeGroupText(t *testing.T) {
	out := CallJSON("encodeGroupText", `["#test","Alice","hi"]`)
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["error"]; ok {
		t.Fatalf("unexpected error: %v", m["error"])
	}
	hex, ok := m["hex"].(string)
	if !ok || len(hex) < 4 {
		t.Fatalf("want hex result, got %#v", m)
	}
}

func TestCallJSON_unknownOp(t *testing.T) {
	out := CallJSON("nope", `[]`)
	if !strings.Contains(out, `"error"`) {
		t.Fatalf("want error JSON, got %s", out)
	}
}
