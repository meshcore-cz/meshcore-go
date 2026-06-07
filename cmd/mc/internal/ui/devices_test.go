package ui

import (
	"strings"
	"testing"
	"time"
)

func TestRenderDeviceListDefault(t *testing.T) {
	data := DeviceListData{
		Devices: []DeviceListRow{
			{Profile: "handheld", Selected: true, Device: "EFF01EF2", Backend: "ready", Radio: "connected", Replica: "fresh", Transport: "BLE", Activity: "idle"},
			{Profile: "desk-radio", Device: "A82F910C", Backend: "ready", Radio: "connected", Replica: "fresh", Transport: "SERIAL", Activity: "idle"},
			{Profile: "field-node", Device: "B71A44C2", Backend: "stopped", Radio: "-", Replica: "cached", Transport: "BLE", Activity: "-"},
			{Profile: "test-node", Device: "C40A109E", Backend: "degraded", Radio: "reconnecting", Replica: "stale", Transport: "BLE", Activity: "reconnecting"},
		},
		Meta: DeviceListMeta{
			Count:    4,
			Selected: "handheld",
			Ready:    2,
			Degraded: 1,
			Stopped:  1,
		},
	}

	var buf strings.Builder
	out := RenderDeviceList(data, NewPrinter(&buf))
	for _, want := range []string{
		"PROFILE",
		"DEVICE",
		"BACKEND",
		"RADIO",
		"REPLICA",
		"TRANSPORT",
		"ACTIVITY",
		"handheld",
		"EFF01EF2",
		"ready",
		"connected",
		"fresh",
		"BLE",
		"desk-radio",
		"SERIAL",
		"field-node",
		"stopped",
		"cached",
		"test-node",
		"degraded",
		"reconnecting",
		"stale",
		"idle",
		"4 devices · handheld selected · 2 ready · 1 degraded · 1 stopped",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output:\n%s", want, out)
		}
	}
	if strings.Contains(out, "ENDPOINT") {
		t.Fatalf("default output should not include ENDPOINT:\n%s", out)
	}
	if !strings.Contains(out, "* ") || !strings.Contains(out, "  desk-radio") {
		t.Fatalf("expected separate selection marker column:\n%s", out)
	}
}

func TestRenderDeviceListWide(t *testing.T) {
	data := DeviceListData{
		Devices: []DeviceListRow{
			{
				Profile:   "handheld",
				Selected:  true,
				Device:    "EFF01EF2",
				Backend:   "ready",
				Radio:     "connected",
				Replica:   "fresh",
				Transport: "BLE",
				Activity:  "idle",
				Endpoint:  "ble://90d56c84-42ef-36f3-89ae-9e8f42231b00",
			},
		},
		Meta: DeviceListMeta{Count: 1, Selected: "handheld", Ready: 1},
		Wide: true,
	}

	var buf strings.Builder
	out := RenderDeviceList(data, NewPrinter(&buf))
	for _, want := range []string{"ENDPOINT", "ACTIVITY", "ble://90d56c84-42ef-36f3-89ae-9e8f42231b00"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestDeviceListBackendState(t *testing.T) {
	if got := DeviceListBackendState(true, true, "ready"); got != "ready" {
		t.Fatalf("got %q, want ready", got)
	}
	if got := DeviceListBackendState(false, true, "ready"); got != "stopped" {
		t.Fatalf("got %q, want stopped", got)
	}
}

func TestDeviceListRadioState(t *testing.T) {
	if got := DeviceListRadioState(true, true, true, "ready"); got != "connected" {
		t.Fatalf("got %q, want connected", got)
	}
	if got := DeviceListRadioState(true, true, false, "degraded"); got != "reconnecting" {
		t.Fatalf("got %q, want reconnecting", got)
	}
	if got := DeviceListRadioState(false, true, true, "ready"); got != "-" {
		t.Fatalf("got %q, want -", got)
	}
}

func TestDeviceListReplicaState(t *testing.T) {
	now := time.Now()
	contacts := ReplicaInfo{Count: 10, SyncedAt: now}
	channels := ReplicaInfo{Count: 2, SyncedAt: now}

	if got := DeviceListReplicaState(true, true, true, "ready", contacts, channels); got != "fresh" {
		t.Fatalf("got %q, want fresh", got)
	}
	if got := DeviceListReplicaState(false, true, true, "ready", contacts, channels); got != "cached" {
		t.Fatalf("got %q, want cached", got)
	}
	if got := DeviceListReplicaState(true, true, false, "degraded", contacts, channels); got != "stale" {
		t.Fatalf("got %q, want stale", got)
	}
	syncing := contacts
	syncing.Syncing = true
	if got := DeviceListReplicaState(true, true, true, "ready", syncing, channels); got != "syncing" {
		t.Fatalf("got %q, want syncing", got)
	}
}

func TestDeviceListActivityState(t *testing.T) {
	radio := RadioIOInfo{Active: true, Method: "trace"}
	if got := DeviceListActivityState(true, true, "ready", ReplicaInfo{}, ReplicaInfo{}, radio); got != "trace" {
		t.Fatalf("got %q, want trace", got)
	}
	if got := DeviceListActivityState(true, true, "degraded", ReplicaInfo{}, ReplicaInfo{}, RadioIOInfo{}); got != "reconnecting" {
		t.Fatalf("got %q, want reconnecting", got)
	}
	if got := DeviceListActivityState(false, true, "ready", ReplicaInfo{}, ReplicaInfo{}, RadioIOInfo{}); got != "-" {
		t.Fatalf("got %q, want -", got)
	}
	syncing := ReplicaInfo{Syncing: true}
	if got := DeviceListActivityState(true, true, "ready", syncing, ReplicaInfo{}, RadioIOInfo{}); got != "syncing contacts" {
		t.Fatalf("got %q, want syncing contacts", got)
	}
}

func TestHumanTransport(t *testing.T) {
	if got := HumanTransport("ble"); got != "BLE" {
		t.Fatalf("got %q, want BLE", got)
	}
	if got := HumanTransport("serial"); got != "SERIAL" {
		t.Fatalf("got %q, want SERIAL", got)
	}
}

func TestDeviceShortIDExport(t *testing.T) {
	if got := DeviceShortID("eff01ef21805fb30"); got != "EFF01EF2" {
		t.Fatalf("got %q, want EFF01EF2", got)
	}
}
