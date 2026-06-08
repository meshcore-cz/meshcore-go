package cli

import (
	"testing"
	"time"

	meshcore "github.com/meshcore-cz/meshcore-go"
	localbackend "github.com/meshcore-cz/meshcore-go/backend"
	"github.com/meshcore-cz/meshcore-go/cmd/mc/internal/ui"
)

func TestBackendStatusForOutputIsCompact(t *testing.T) {
	now := time.Now()
	out := backendStatusForOutput(localbackend.Status{
		Running: true,
		Healthy: true,
		State:   "ready",
		PID:     1234,
		Radio: localbackend.RadioStatus{
			LastAt:         now,
			LastMethod:     "stats",
			LastDurationMs: 179,
		},
		Device: localbackend.DeviceStatus{Name: "EFF01EF2"},
		Stats:  meshcore.LocalStats{},
	}, true)

	for _, key := range []string{"device", "stats", "contacts", "channels", "radio", "bridges"} {
		if _, ok := out[key]; ok {
			t.Fatalf("backend output should not contain redundant %q: %#v", key, out)
		}
	}
	activity, ok := out["activity"].(map[string]any)
	if !ok {
		t.Fatalf("activity missing: %#v", out)
	}
	last, ok := activity["last_request"].(map[string]any)
	if !ok || last["method"] != "stats" {
		t.Fatalf("last request missing method: %#v", activity)
	}
}

func TestLocalStateForOutputUsesSnakeCase(t *testing.T) {
	now := time.Now()
	out := localStateForOutput(ui.LocalStateInfo{
		Initialized:      true,
		Contacts:         42,
		Channels:         3,
		RepeaterSessions: 1,
		UpdatedAt:        now,
	})
	for _, key := range []string{"Initialized", "Contacts", "Channels", "RepeaterSessions", "UpdatedAt"} {
		if _, ok := out[key]; ok {
			t.Fatalf("local state should not contain Go field %q: %#v", key, out)
		}
	}
	if out["repeater_sessions"] != 1 || out["contacts"] != 42 || out["channels"] != 3 {
		t.Fatalf("local state counts missing: %#v", out)
	}
}
