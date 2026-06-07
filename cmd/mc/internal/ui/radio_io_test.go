package ui

import "testing"

func TestRadioIOLabel(t *testing.T) {
	if got := RadioIOLabel(RadioIOInfo{}); got != "idle" {
		t.Fatalf("got %q", got)
	}
	if got := RadioIOLabel(RadioIOInfo{Active: true, Method: "trace", DurationMs: 12000}); got != "trace (12s)" {
		t.Fatalf("got %q", got)
	}
	if got := RadioIOLabel(RadioIOInfo{Active: true, Method: "repeater_status", DurationMs: 65000}); got != "repeater status (1m5s)" {
		t.Fatalf("got %q", got)
	}
}

func TestRadioMethodLabel(t *testing.T) {
	if got := radioMethodLabel("watch_raw"); got != "watch raw" {
		t.Fatalf("got %q", got)
	}
}
