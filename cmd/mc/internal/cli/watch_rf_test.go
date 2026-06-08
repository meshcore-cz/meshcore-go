package cli

import (
	"testing"
	"time"

	meshcore "github.com/meshcore-cz/meshcore-go"
)

func TestWatchRFIgnoresAdvert(t *testing.T) {
	if _, ok := rfPacketFromEvent(meshcore.AdvertisementReceived{}); ok {
		t.Fatal("advertisement event should not be emitted by --rf")
	}
}

func TestWatchRFAcceptsRFPacket(t *testing.T) {
	want := meshcore.RFPacketReceived{
		Timestamp: time.Now(),
		SNR:       12,
		RSSI:      -57,
		Bytes:     []byte{0x80, 0x8e, 0xa8},
	}
	got, ok := rfPacketFromEvent(want)
	if !ok {
		t.Fatal("RF packet event was not accepted")
	}
	if got.SNR != want.SNR || got.RSSI != want.RSSI || string(got.Bytes) != string(want.Bytes) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}
