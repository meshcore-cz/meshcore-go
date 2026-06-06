package ui

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestRelativeTime(t *testing.T) {
	if got := RelativeTime(time.Now().Add(-3 * time.Second)); got != "3s ago" {
		t.Fatalf("got %q", got)
	}
	if got := RelativeTime(time.Time{}); got != "never" {
		t.Fatalf("zero = %q", got)
	}
}

func TestRenderStatusLayout(t *testing.T) {
	data := StatusData{
		Device: DeviceInfo{
			Name:            "MeshCore",
			PublicKey:       "eff01ef21805abcd",
			Firmware:        "MeshCore (Heltec V3)",
			FirmwareVersion: "v1.16.0-07a3ca9",
			Protocol:        "companion-v3",
			Transport:       "ble",
			TransportURI:    "ble://C4:20:12:34:56:78",
			Available:       true,
		},
		Backend: BackendInfo{
			Running: true,
			Healthy: true,
			State:   "ready",
			PID:     41148,
			Contacts: ReplicaInfo{
				Count:    286,
				SyncedAt: time.Now().Add(-3 * time.Second),
			},
			Channels: ReplicaInfo{
				Count:    2,
				SyncedAt: time.Now().Add(-2 * time.Second),
			},
		},
	}
	var buf bytes.Buffer
	printer := NewPrinter(&buf)
	out := RenderStatus(data, printer)
	for _, want := range []string{
		"Device:        EFF01EF2",
		"Firmware:      MeshCore (Heltec V3) v1.16.0-07a3ca9",
		"Protocol:      companion-v3",
		"Transport:     ble://C4:20:12:34:56:78",
		"Public key:    eff01ef21805abcd",
		"Backend:       ready (pid 41148)",
		"Replica:       fresh",
		"Contacts:      286, updated",
		"Channels:      2, updated",
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
