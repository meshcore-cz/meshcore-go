package ui

import (
	"testing"
	"time"
)

func TestActivityLabel(t *testing.T) {
	theme := Theme{enabled: false}

	if got := ActivityLabel(RadioIOInfo{}, theme); got != "idle" {
		t.Fatalf("got %q", got)
	}
	if got := ActivityLabel(RadioIOInfo{
		LastAt:         time.Now().Add(-4 * time.Second),
		LastMethod:     "trace",
		LastDurationMs: 450,
	}, theme); got != "idle · last: trace (450ms) · 4s ago" {
		t.Fatalf("got %q", got)
	}
	if got := ActivityLabel(RadioIOInfo{Active: true, Method: "trace", DurationMs: 1234}, theme); got != "trace (1.2s)" {
		t.Fatalf("got %q", got)
	}
	if got := ActivityLabel(RadioIOInfo{Active: true, Method: "trace", DurationMs: 12000}, theme); got != "trace (12.0s)" {
		t.Fatalf("got %q", got)
	}
	if got := ActivityLabel(RadioIOInfo{Active: true, Method: "repeater_status", DurationMs: 65000}, theme); got != "repeater status (1m5.0s)" {
		t.Fatalf("got %q", got)
	}
}

func TestRadioMethodLabel(t *testing.T) {
	if got := radioMethodLabel("watch_raw"); got != "watch raw" {
		t.Fatalf("got %q", got)
	}
}

func TestFormatBandwidthHz(t *testing.T) {
	if got := FormatBandwidthHz(62500); got != "62.5 kHz" {
		t.Fatalf("got %q", got)
	}
	if got := FormatBandwidthHz(250000); got != "250 kHz" {
		t.Fatalf("got %q", got)
	}
}

func TestFormatDuration(t *testing.T) {
	if got := formatDuration(450 * time.Millisecond); got != "450ms" {
		t.Fatalf("got %q", got)
	}
	if got := formatDuration(12 * time.Second); got != "12s" {
		t.Fatalf("got %q", got)
	}
}

func TestFormatRunningDuration(t *testing.T) {
	if got := formatRunningDuration(450 * time.Millisecond); got != "450ms" {
		t.Fatalf("got %q", got)
	}
	if got := formatRunningDuration(1234 * time.Millisecond); got != "1.2s" {
		t.Fatalf("got %q", got)
	}
	if got := formatRunningDuration(65 * time.Second); got != "1m5.0s" {
		t.Fatalf("got %q", got)
	}
}
