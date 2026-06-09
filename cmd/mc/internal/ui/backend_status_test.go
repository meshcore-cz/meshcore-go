package ui

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestRenderBackendStatusDaemonOverview(t *testing.T) {
	now := time.Now()
	data := BackendStatusData{
		Running:           true,
		Healthy:           true,
		PID:               74256,
		StartedAt:         now.Add(-2*time.Hour - 19*time.Minute),
		UptimeSec:         int64((2*time.Hour + 19*time.Minute).Seconds()),
		Socket:            "/tmp/mc/backend.sock",
		Clients:           2,
		RequestsCompleted: 1842,
		RequestsFailed:    0,
		Sessions: []BackendSessionInfo{
			{
				Name:       "handheld",
				Active:     true,
				State:      "ready",
				Healthy:    true,
				Transport:  "ble://90d56c84",
				StartedAt:  now.Add(-37 * time.Minute),
				Activity:   RadioIOInfo{LastAt: now.Add(-4 * time.Second), LastMethod: "stats"},
				LocalState: LocalStateInfo{Initialized: true, UpdatedAt: now.Add(-2 * time.Minute)},
			},
			{
				Name:       "travel-node",
				State:      "stopped",
				Transport:  "ble://a81192",
				LocalState: LocalStateInfo{Initialized: true, UpdatedAt: now.Add(-21 * 24 * time.Hour)},
			},
		},
	}
	var buf bytes.Buffer
	out := RenderBackendStatus(data, NewPrinter(&buf))
	for _, want := range []string{
		"Backend:       running",
		"PID:           74256",
		"Uptime:        2h 19m",
		"Socket:        /tmp/mc/backend.sock",
		"Started:",
		"Sessions:      2 total · 1 ready · 1 stopped",
		"IPC clients:   2 connected",
		"Requests:      1,842 handled · 0 failed",
		"Sessions\n",
		"  handheld  (active)",
		"    State:       ready · connected 37m ago",
		"    Transport:   ble://90d56c84",
		"    Activity:    idle · last request 4s ago (stats)",
		"    Local state: updated 2m ago",
		"  travel-node",
		"    State:       stopped",
		"    Transport:   ble://a81192",
		"    Local state: updated 21d ago",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
	for _, absent := range []string{"Radio:", "Replica:", "Device stats:"} {
		if strings.Contains(out, absent) {
			t.Fatalf("backend status should not include %q:\n%s", absent, out)
		}
	}
}

func TestRenderBackendStatusProblemSession(t *testing.T) {
	now := time.Now()
	data := BackendStatusData{
		Running: true,
		Healthy: true,
		Socket:  "/tmp/mc/backend.sock",
		Sessions: []BackendSessionInfo{
			{
				Name:       "handheld",
				Active:     true,
				State:      "degraded",
				Transport:  "ble://90d56c84",
				LastActive: now.Add(-4 * time.Minute),
				LastError:  "device unavailable",
				LocalState: LocalStateInfo{Initialized: true, UpdatedAt: now.Add(-4 * time.Minute)},
			},
			{Name: "travel-node", State: "stopped"},
		},
	}
	var buf bytes.Buffer
	out := RenderBackendStatus(data, NewPrinter(&buf))
	for _, want := range []string{
		"Sessions:      2 total · 1 retrying · 1 stopped",
		"  handheld  (active)",
		"    State:       retrying · last active 4m ago",
		"    Activity:    last active 4m ago",
		"    Error:       device unavailable",
		"    Local state: updated 4m ago",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRenderBackendStatusNotRunning(t *testing.T) {
	data := BackendStatusData{Socket: "/tmp/mc/backend.sock"}
	var buf bytes.Buffer
	out := RenderBackendStatus(data, NewPrinter(&buf))
	for _, want := range []string{
		"Backend:       not running",
		"Socket:        /tmp/mc/backend.sock",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Sessions:") {
		t.Fatalf("not-running backend status should not print sessions:\n%s", out)
	}
}

func TestRenderBackendStatusVerbose(t *testing.T) {
	now := time.Now()
	data := BackendStatusData{
		Running:           true,
		Healthy:           true,
		PID:               74256,
		Socket:            "/tmp/mc/backend.sock",
		ConfigPath:        "/tmp/mc/config.yaml",
		LogPath:           "/tmp/mc/backend.log",
		Clients:           2,
		RequestsCompleted: 1842,
		RequestsFailed:    0,
		QueuePending:      4,
		Verbose:           true,
		Sessions: []BackendSessionInfo{
			{
				Name:              "handheld",
				Active:            true,
				State:             "ready",
				Transport:         "ble://90d56c84",
				StartedAt:         now.Add(-37 * time.Minute),
				RequestsCompleted: 412,
				RequestsFailed:    0,
				Activity:          RadioIOInfo{LastAt: now.Add(-4 * time.Second), LastMethod: "stats"},
				LocalState:        LocalStateInfo{Initialized: true, Contacts: 42, Channels: 3, UpdatedAt: now.Add(-2 * time.Minute)},
				LocalStatePath:    "/tmp/mc/devices/eff01ef21805.db",
			},
		},
	}
	var buf bytes.Buffer
	out := RenderBackendStatus(data, NewPrinter(&buf))
	for _, want := range []string{
		"Config:        /tmp/mc/config.yaml",
		"Log:           /tmp/mc/backend.log",
		"IPC\n",
		"  Clients:     2 connected",
		"  Requests:    1,842 handled · 4 active · 0 failed",
		"    Requests:    412 handled · 0 failed",
		"    Local state: /tmp/mc/devices/eff01ef21805.db",
		"    Updated:     2m ago",
		"    Contacts:    42",
		"    Channels:    3",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}
