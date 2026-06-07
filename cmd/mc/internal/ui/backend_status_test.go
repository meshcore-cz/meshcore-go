package ui

import (
	"bytes"
	"strings"
	"testing"
	"time"

	meshcore "github.com/meshcore-cz/meshcore-go"
)

func TestRenderBackendStatusReady(t *testing.T) {
	now := time.Now()
	data := BackendStatusData{
		Running:   true,
		Healthy:   true,
		State:     "ready",
		PID:       74256,
		UptimeSec: 139,
		Socket:    "/tmp/mc/backend.sock",
		URI:       "ble://90d56c84-42ef-36f3-89ae-9e8f42231b00",
		LastSeen:  now.Add(-18 * time.Second),
		Contacts: ReplicaInfo{
			Count:    360,
			SyncedAt: now.Add(-1 * time.Minute),
		},
		Channels: ReplicaInfo{
			Count:    2,
			SyncedAt: now.Add(-3 * time.Minute),
		},
		Stats: DeviceStatsFromLocal(meshcore.LocalStats{
			Core: meshcore.LocalStatsCore{
				BatteryMV:  3900,
				UptimeSecs: 39420,
				QueueLen:   0,
			},
			Radio: meshcore.LocalStatsRadio{
				RxAirSecs: 8040,
				TxAirSecs: 517,
			},
			Packets: meshcore.LocalStatsPackets{
				Received:   12846,
				Sent:       1284,
				RecvErrors: 7,
			},
		}, true, now.Add(-18*time.Second)),
		RadioIO: RadioIOInfo{
			LastAt:         now.Add(-18 * time.Second),
			LastMethod:     "stats",
			LastDurationMs: 177,
		},
	}
	var buf bytes.Buffer
	out := RenderBackendStatus(data, NewPrinter(&buf))
	for _, want := range []string{
		"Backend:",
		"  State:       ready",
		"  PID:         74256",
		"  Uptime:      2m 19s",
		"  Socket:      /tmp/mc/backend.sock",
		"Radio:",
		"  State:       connected",
		"  Transport:   BLE",
		"  Endpoint:    ble://90d56c84-42ef-36f3-89ae-9e8f42231b00",
		"  Last seen:   18s ago",
		"Replica:",
		"  Contacts:    360 · synced 1m ago",
		"  Channels:    2 · synced 3m ago",
		"Device stats:",
		"  Uptime:      10h 57m",
		"  Battery:     3.90 V",
		"  Packets:     12,846 rx · 1,284 tx · 7 errors",
		"  Airtime:     2h 14m rx · 8m 37s tx",
		"  Queue:       0 radio packets pending",
		"Diagnostics:",
		"  Activity:    idle",
		"  Last op:     stats · 177ms · 18s ago",
		"  Stats poll:  healthy · updated 18s ago",
		"  Queue:       0 backend requests pending",
		"  Reconnects:  0",
		"  Clients:     0 connected",
		"  Last error:  none",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRenderBackendStatusSyncing(t *testing.T) {
	now := time.Now()
	data := BackendStatusData{
		Running:   true,
		Healthy:   true,
		State:     "ready",
		PID:       74256,
		URI:       "ble://device",
		LastSeen:  now.Add(-4 * time.Second),
		Contacts: ReplicaInfo{
			Syncing:      true,
			SyncReceived: 183,
			SyncTotal:    449,
		},
		Channels: ReplicaInfo{
			Count:    2,
			SyncedAt: now.Add(-9 * time.Minute),
		},
		Stats: DeviceStatsFromLocal(meshcore.LocalStats{}, true, now.Add(-45*time.Second)),
		RadioIO: RadioIOInfo{
			Active:     true,
			Method:     "contacts",
			DurationMs: 14000,
			LastAt:     now.Add(-32 * time.Second),
			LastMethod: "stats",
			LastDurationMs: 162,
		},
		QueuePending: 1,
	}
	var buf bytes.Buffer
	out := RenderBackendStatus(data, NewPrinter(&buf))
	for _, want := range []string{
		"  Contacts:    syncing · 183/449 received",
		"  Activity:    syncing contacts · 14.0s",
		"  Stats poll:  delayed · radio busy",
		"  Queue:       1 backend requests pending",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRenderBackendStatusDegraded(t *testing.T) {
	now := time.Now()
	data := BackendStatusData{
		Running:   true,
		Healthy:   false,
		State:     "degraded",
		PID:       74256,
		URI:       "ble://device",
		LastSeen:  now.Add(-6 * time.Minute),
		LastError: "BLE connection lost",
		Contacts: ReplicaInfo{
			Count:    360,
			SyncedAt: now.Add(-2 * time.Hour),
		},
		Channels: ReplicaInfo{
			Count:    2,
			SyncedAt: now.Add(-2 * time.Hour),
		},
		RadioIO: RadioIOInfo{
			LastAt:     now.Add(-6 * time.Minute),
			LastMethod: "stats",
		},
		QueuePending: 2,
	}
	var buf bytes.Buffer
	out := RenderBackendStatus(data, NewPrinter(&buf))
	for _, want := range []string{
		"  State:       degraded",
		"  State:       unavailable",
		"  Contacts:    360 · cached · synced 2h ago",
		"  Activity:    reconnecting",
		"  Last op:     stats · failed · 6m ago",
		"  Stats poll:  failed",
		"  Last error:  BLE connection lost",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRenderBackendStatusStaleStatsPoll(t *testing.T) {
	now := time.Now()
	data := BackendStatusData{
		Running:  true,
		Healthy:  true,
		State:    "ready",
		URI:      "ble://device",
		LastSeen: now.Add(-2 * time.Minute),
		Stats:    DeviceStatsFromLocal(meshcore.LocalStats{}, true, now.Add(-2*time.Minute)),
	}
	var buf bytes.Buffer
	out := RenderBackendStatus(data, NewPrinter(&buf))
	for _, want := range []string{
		"  Stats poll:  stale · updated 2m ago",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Stats poll:  healthy") {
		t.Fatalf("stale stats should not show healthy:\n%s", out)
	}
}

func TestRenderBackendStatusBridges(t *testing.T) {
	data := BackendStatusData{
		Running: true,
		Healthy: true,
		State:   "ready",
		URI:     "ble://device",
		Bridges: []BridgeInfo{
			{Type: "tcp", Listen: "127.0.0.1:4403", Active: true},
			{Type: "pty", Path: "/dev/ttys012", Active: true},
		},
	}
	var buf bytes.Buffer
	out := RenderBackendStatus(data, NewPrinter(&buf))
	for _, want := range []string{
		"Bridges:",
		"  PTY:         listening · /dev/ttys012",
		"  TCP:         listening · 127.0.0.1:4403",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestBackendBridgeFailed(t *testing.T) {
	theme := Theme{enabled: false}
	got := backendBridgeLabel(BridgeInfo{
		Type:  "tcp",
		Error: "address already in use",
	}, theme)
	want := "failed · address already in use"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
