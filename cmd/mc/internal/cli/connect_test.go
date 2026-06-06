package cli

import "testing"

func TestDefaultProfileName(t *testing.T) {
	got := defaultProfileName("EFF01EF21805ABCD", "serial:///dev/cu.usbserial-0001")
	if got != "serial:eff01ef2" {
		t.Fatalf("got %q, want serial:eff01ef2", got)
	}
	got = defaultProfileName("EFF01EF21805ABCD", "ble://C4:20:12:34:56:78")
	if got != "ble:eff01ef2" {
		t.Fatalf("got %q, want ble:eff01ef2", got)
	}
}
