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
		Backend: BackendInfo{
			Running: true,
			Healthy: true,
			State:   "ready",
			PID:     41148,
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
		"Device:        EFF01EF2",
		"Firmware:      MeshCore (Heltec V3) v1.16.0-07a3ca9",
		"Protocol:      companion-v3",
		"Transport:     ble://C4:20:12:34:56:78",
		"Public key:    eff01ef21805abcd",
		"Radio:         ok · updated 124ms ago",
		"  Modem:       869.525 MHz · BW 250 kHz · SF11 · CR 4/5 · TX 22 dBm",
		"  Signal:      -104 dBm RSSI · +7.5 dB SNR · -118 dBm noise",
		"  Battery:     4.08 V",
		"  Uptime:      10h 57m",
		"  Packets:     12,846 rx · 1,284 tx · 7 errors",
		"  Airtime:     2h 14m rx · 8m 37s tx",
		"  Queue:       0 pending",
		"Backend:       running (pid 41148)",
		"  Activity:    idle · last: stats (120ms) · 4s ago",
		"  Replica:     fresh · 286 contacts · 2 channels · updated 48s ago",
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
