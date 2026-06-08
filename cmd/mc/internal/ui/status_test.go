package ui

import (
	"bytes"
	"strings"
	"testing"
	"time"

	meshcore "github.com/meshcore-cz/meshcore-go"
)

func TestRelativeTime(t *testing.T) {
	if got := RelativeTime(time.Now().Add(-3 * time.Second)); got != "3s ago" {
		t.Fatalf("got %q", got)
	}
	if got := RelativeTime(time.Time{}); got != "never" {
		t.Fatalf("zero = %q", got)
	}
	if got := RelativeTime(time.Now().Add(-23*time.Hour - 26*time.Minute)); got != "23h 26m ago" {
		t.Fatalf("compound hours = %q", got)
	}
}

func TestContactAge(t *testing.T) {
	if got := ContactAge(time.Time{}); got != "never" {
		t.Fatalf("zero = %q", got)
	}
	if got := ContactAge(time.Now().Add(-17 * time.Hour)); got != "17h" {
		t.Fatalf("hours = %q, want 17h", got)
	}
	if got := ContactAge(time.Now().Add(-23*time.Hour - 26*time.Minute)); got != "23h" {
		t.Fatalf("hours truncate = %q, want 23h", got)
	}
	if got := ContactAge(time.Now().Add(-706*24*time.Hour - 5*time.Hour)); got != "706d" {
		t.Fatalf("days = %q, want 706d", got)
	}
}

func TestStatsUpdatedRelative(t *testing.T) {
	if got := StatsUpdatedRelative(time.Now()); got != "now" {
		t.Fatalf("recent = %q, want now", got)
	}
	if got := StatsUpdatedRelative(time.Now().Add(-10 * time.Millisecond)); got != "10ms ago" {
		t.Fatalf("got %q, want 10ms ago", got)
	}
}

func TestStatsUpdateStale(t *testing.T) {
	now := time.Now()
	if StatsUpdateStale(time.Time{}) {
		t.Fatal("zero time should not be stale")
	}
	if StatsUpdateStale(now.Add(-30 * time.Second)) {
		t.Fatal("30s old stats should not be stale")
	}
	if !StatsUpdateStale(now.Add(-2 * time.Minute)) {
		t.Fatal("2m old stats should be stale")
	}
}

func TestRenderStatusLayout(t *testing.T) {
	data := StatusData{
		ProfileName: "handheld",
		Device: DeviceInfo{
			Name:            "MeshCore",
			PublicKey:       "eff01ef21805abcd",
			Firmware:        "MeshCore (Heltec V3)",
			FirmwareVersion: "v1.16.0-07a3ca9",
			Protocol:        "companion-v3",
			Transport:       "ble",
			TransportURI:    "ble://C4:20:12:34:56:78",
			Radio: RadioInfo{
				FrequencyKHz: 869525,
				BandwidthKHz: 250000,
				Spreading:    11,
				CodingRate:   5,
				TxPowerDBm:   22,
			},
			Stats: DeviceStatsFromLocal(meshcore.LocalStats{
				Core: meshcore.LocalStatsCore{
					BatteryMV:  4080,
					UptimeSecs: 39420,
					QueueLen:   0,
				},
				Radio: meshcore.LocalStatsRadio{
					LastRSSI:   -104,
					LastSNR:    7.5,
					NoiseFloor: -118,
					RxAirSecs:  8040,
					TxAirSecs:  517,
				},
				Packets: meshcore.LocalStatsPackets{
					Received:   12846,
					Sent:       1284,
					RecvErrors: 7,
				},
			}, true, time.Now().Add(-124*time.Millisecond)),
			Available: true,
		},
		LocalState: LocalStateInfo{
			Initialized:      true,
			Contacts:         286,
			Channels:         2,
			RepeaterSessions: 1,
			UpdatedAt:        time.Now().Add(-48 * time.Second),
		},
		Backend: BackendInfo{
			Running:   true,
			Healthy:   true,
			State:     "ready",
			PID:       41148,
			StartedAt: time.Now().Add(-39420 * time.Second),
			UptimeSec: 39420,
			RadioIO: RadioIOInfo{
				LastAt:         time.Now().Add(-4 * time.Second),
				LastMethod:     "stats",
				LastDurationMs: 120,
			},
			Contacts: ReplicaInfo{
				Count:    286,
				SyncedAt: time.Now().Add(-48 * time.Second),
			},
			Channels: ReplicaInfo{
				Count:    2,
				SyncedAt: time.Now().Add(-2 * time.Minute),
			},
		},
	}
	var buf bytes.Buffer
	printer := NewPrinter(&buf)
	out := RenderStatus(data, printer)
	for _, want := range []string{
		"handheld  (active)",
		"  Public key:    eff01ef21805abcd",
		"  Name:          EFF01EF2",
		"  Firmware:      MeshCore (Heltec V3) v1.16.0-07a3ca9",
		"  Protocol:      companion-v3",
		"  Transport:     ble://C4:20:12:34:56:78",
		"  Radio:         active · 10h 57m uptime · updated 124ms ago",
		"    Modem:       869.525 MHz · BW 250 kHz · SF11 · CR 4/5 · TX 22 dBm",
		"    Signal:      -104 dBm RSSI · +7.5 dB SNR · -118 dBm noise",
		"    Battery:     4.08 V",
		"    Packets:     12,846 rx · 1,284 tx · 7 errors",
		"    Airtime:     2h 14m rx · 8m 37s tx",
		"    Queue:       0 pending",
		"  Local state:   available · updated 48s ago",
		"    Contacts:    286",
		"    Channels:    2",
		"    Sessions:    1 repeater login",
		"  Backend:       running · 10h 57m uptime",
		"    Session:     ready · connected 10h 57m ago",
		"    Activity:    idle · last request 4s ago (stats)",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRenderStatusLocalStateNotInitialized(t *testing.T) {
	data := StatusData{
		Device: DeviceInfo{
			PublicKey: "eff01ef21805abcd",
			Available: true,
		},
	}
	var buf bytes.Buffer
	printer := NewPrinter(&buf)
	out := RenderStatus(data, printer)
	if !strings.Contains(out, "  Local state:   not initialized") {
		t.Fatalf("output missing local state not initialized:\n%s", out)
	}
}

func TestRenderStatusAllLayout(t *testing.T) {
	data := StatusAllData{Rows: []StatusAllRow{
		{
			Profile:   "handheld",
			Selected:  true,
			Session:   "ready",
			Transport: "ble",
			Radio: DeviceStatsFromLocal(meshcore.LocalStats{
				Core: meshcore.LocalStatsCore{BatteryMV: 4190},
			}, true, time.Now().Add(-4*time.Second)),
			LocalState: LocalStateInfo{
				Initialized: true,
				Contacts:    42,
				Channels:    3,
				UpdatedAt:   time.Now().Add(-2 * time.Minute),
			},
			ConnectedAt: time.Now().Add(-37 * time.Minute),
		},
		{
			Profile:   "travel-node",
			Session:   "stopped",
			Transport: "ble",
			LocalState: LocalStateInfo{
				Initialized: true,
				Contacts:    18,
				Channels:    2,
				UpdatedAt:   time.Now().Add(-21 * 24 * time.Hour),
			},
		},
	}}
	var buf bytes.Buffer
	printer := NewPrinter(&buf)
	out := RenderStatusAll(data, printer)
	for _, want := range []string{
		"* handheld",
		"    Session:     ready · connected 37m ago",
		"    Transport:   ble",
		"    Radio:       active · battery 4.19 V · updated 4s ago",
		"    Local state: 42 contacts · 3 channels · updated 2m ago",
		"  travel-node",
		"    Session:     not running",
		"    Local state: 18 contacts · 2 channels · updated 21d ago",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestStatusWordPlainWhenDisabled(t *testing.T) {
	theme := Theme{enabled: false}
	if got := theme.StatusWord(HealthOK, "ready"); got != "ready" {
		t.Fatalf("got %q", got)
	}
}

func TestStatusDimTimestamps(t *testing.T) {
	theme := Theme{enabled: true}
	now := time.Now()

	activity := ActivityLabel(RadioIOInfo{
		LastAt:         now.Add(-4 * time.Second),
		LastMethod:     "stats",
		LastDurationMs: 120,
	}, theme)
	if !strings.Contains(activity, "\x1b[1;32midle\x1b[0m") {
		t.Fatalf("idle state should stay prominent: %q", activity)
	}
	if !strings.Contains(activity, "\x1b[2mlast: stats (120ms)\x1b[0m") {
		t.Fatalf("last activity detail should be dim: %q", activity)
	}
	if !strings.Contains(activity, "\x1b[2m4s ago\x1b[0m") {
		t.Fatalf("last activity time should be dim: %q", activity)
	}

	active := ActivityLabel(RadioIOInfo{Active: true, Method: "trace", DurationMs: 1234}, theme)
	if !strings.Contains(active, "\x1b[2m(1.2s)\x1b[0m") {
		t.Fatalf("active duration should be dim: %q", active)
	}

	replica := replicaDetails(BackendInfo{
		Contacts: ReplicaInfo{Count: 286, SyncedAt: now.Add(-48 * time.Second)},
		Channels: ReplicaInfo{Count: 2, SyncedAt: now.Add(-2 * time.Minute)},
	}, theme)
	if !strings.Contains(replica, "286 contacts · 2 channels · ") {
		t.Fatalf("replica counts should stay plain: %q", replica)
	}
	if !strings.Contains(replica, "\x1b[2mupdated 48s ago\x1b[0m") {
		t.Fatalf("replica update time should be dim: %q", replica)
	}
}
