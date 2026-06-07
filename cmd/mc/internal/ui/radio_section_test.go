package ui

import (
	"testing"
	"time"

	meshcore "github.com/meshcore-cz/meshcore-go"
)

func TestRadioSectionHealth(t *testing.T) {
	dev := DeviceInfo{
		Stats: DeviceStatsFromLocal(meshcore.LocalStats{}, true, time.Now()),
	}
	health, word := radioSectionHealth(dev, BackendInfo{Running: true, Healthy: true})
	if health != HealthOK || word != "ok" {
		t.Fatalf("got %v %q", health, word)
	}

	dev.Stats.Core.ErrorFlags = 1
	health, word = radioSectionHealth(dev, BackendInfo{Running: true, Healthy: true})
	if health != HealthError || word != "error" {
		t.Fatalf("got %v %q", health, word)
	}
}

func TestRelativeTimeMillis(t *testing.T) {
	if got := RelativeTime(time.Now().Add(-124 * time.Millisecond)); got != "124ms ago" {
		t.Fatalf("got %q", got)
	}
}
