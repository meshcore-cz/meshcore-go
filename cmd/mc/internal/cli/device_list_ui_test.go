package cli

import (
	"testing"

	localbackend "github.com/meshcore-cz/meshcore-go/backend"
	"github.com/meshcore-cz/meshcore-go/cmd/mc/internal/config"
)

func testConfigOneDevice() *config.Config {
	return &config.Config{
		Version: 1,
		Current: "handheld",
		Devices: map[string]config.Device{
			"handheld": {
				Name:               "Handheld",
				PreferredTransport: "ble",
				Transports:         []config.Endpoint{{URI: "ble://AA-BB"}},
			},
		},
	}
}

// TestDeviceListDataDaemonEntries verifies that a running session reported by
// the daemon's device_list renders as connected, keyed by profile name.
func TestDeviceListDataDaemonEntries(t *testing.T) {
	cfg := testConfigOneDevice()
	st := localbackend.Status{Running: true, Healthy: true, State: "ready", URI: "ble://AA-BB"}
	entries := map[string]localbackend.DeviceListEntry{
		"handheld": {ID: "handheld", Default: true, Session: "ready", Connected: true, Replica: "fresh", Transport: "ble"},
	}

	data := deviceListData(cfg, st, entries, true)
	if len(data.Devices) != 1 {
		t.Fatalf("rows = %d, want 1", len(data.Devices))
	}
	row := data.Devices[0]
	if row.Backend != "ready" {
		t.Fatalf("Backend = %q, want ready", row.Backend)
	}
	if row.Radio != "connected" {
		t.Fatalf("Radio = %q, want connected", row.Radio)
	}
	if data.Meta.Ready != 1 || data.Meta.Stopped != 0 {
		t.Fatalf("meta ready/stopped = %d/%d, want 1/0", data.Meta.Ready, data.Meta.Stopped)
	}
}

// TestDeviceListDataStoppedSession verifies a registered-but-stopped session
// renders as stopped even while the daemon runs.
func TestDeviceListDataStoppedSession(t *testing.T) {
	cfg := testConfigOneDevice()
	st := localbackend.Status{Running: true, Healthy: true, State: "ready"}
	entries := map[string]localbackend.DeviceListEntry{
		"handheld": {ID: "handheld", Session: "stopped"},
	}

	data := deviceListData(cfg, st, entries, true)
	if data.Devices[0].Backend != "stopped" {
		t.Fatalf("Backend = %q, want stopped", data.Devices[0].Backend)
	}
}

// TestDeviceListDataLegacyFallback verifies that when device_list is
// unavailable (older daemon, entries == nil) the connected device is still
// shown by matching the single status snapshot's URI to the profile.
func TestDeviceListDataLegacyFallback(t *testing.T) {
	cfg := testConfigOneDevice()
	st := localbackend.Status{Running: true, Healthy: true, State: "ready", URI: "ble://AA-BB"}

	data := deviceListData(cfg, st, nil, true)
	if data.Devices[0].Backend != "ready" {
		t.Fatalf("Backend = %q, want ready (legacy fallback)", data.Devices[0].Backend)
	}
	if data.Devices[0].Radio != "connected" {
		t.Fatalf("Radio = %q, want connected (legacy fallback)", data.Devices[0].Radio)
	}
}
