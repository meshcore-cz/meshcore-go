package ui

import (
	"strings"
	"testing"
)

func TestDeviceListColumnAlignment(t *testing.T) {
	data := DeviceListData{
		Devices: []DeviceListRow{
			{Profile: "handheld", Selected: true, Device: "EFF01EF2", Backend: "ready", Radio: "connected", Replica: "fresh", Transport: "BLE", Activity: "idle"},
			{Profile: "desk-radio", Device: "A82F910C", Backend: "ready", Radio: "connected", Replica: "fresh", Transport: "SERIAL", Activity: "idle"},
		},
		Meta: DeviceListMeta{Count: 2, Selected: "handheld", Ready: 2},
	}
	var buf strings.Builder
	out := RenderDeviceList(data, NewPrinter(&buf))
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	t.Logf("header: %q", lines[0])
	t.Logf("row1:   %q", lines[1])
	t.Logf("row2:   %q", lines[2])

	hProfile := strings.Index(lines[0], "PROFILE")
	hDevice := strings.Index(lines[0], "DEVICE")
	dProfile := strings.Index(lines[1], "handheld")
	dDevice := strings.Index(lines[1], "EFF01EF2")
	d2Device := strings.Index(lines[2], "A82F910C")
	if hProfile == 0 || hProfile > hDevice {
		t.Fatalf("PROFILE should precede DEVICE in header: profile@%d device@%d", hProfile, hDevice)
	}
	if hDevice != dDevice || hDevice != d2Device {
		t.Fatalf("DEVICE column misaligned: header@%d row1@%d row2@%d", hDevice, dDevice, d2Device)
	}
	if dProfile >= dDevice {
		t.Fatalf("profile name should precede device ID: profile@%d device@%d", dProfile, dDevice)
	}
	if lines[1][0] != '*' {
		t.Fatalf("selected row should start with selection marker, got %q", lines[1][:1])
	}

	// DEVICE header should occupy the same width as the device ID values below.
	deviceColEnd := strings.Index(lines[1], "EFF01EF2") + len("EFF01EF2")
	headerDeviceEnd := strings.Index(lines[0], "BACKEND") - 1 // space before BACKEND
	if headerDeviceEnd != deviceColEnd {
		t.Fatalf("DEVICE column width mismatch: header ends@%d data ends@%d", headerDeviceEnd, deviceColEnd)
	}
}
