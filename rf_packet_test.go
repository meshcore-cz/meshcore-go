package meshcore

import (
	"strings"
	"testing"
	"time"

	"github.com/meshcore-cz/meshcore-go/protocol"
)

func TestDecodeRFPacketReceived(t *testing.T) {
	ts := time.Now()
	got, err := DecodeRFPacketReceived([]byte{0x88, 48, 0xc7, 0x80, 0x8e, 0xa8}, ts)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Timestamp.Equal(ts) {
		t.Fatalf("timestamp = %s, want %s", got.Timestamp, ts)
	}
	if got.SNR != 12.0 {
		t.Fatalf("snr = %.1f, want 12.0", got.SNR)
	}
	if got.RSSI != -57 {
		t.Fatalf("rssi = %d, want -57", got.RSSI)
	}
	if string(got.Bytes) != string([]byte{0x80, 0x8e, 0xa8}) {
		t.Fatalf("bytes = % x", got.Bytes)
	}
}

func TestDecodeRFPacketReceivedTruncated(t *testing.T) {
	_, err := DecodeRFPacketReceived([]byte{0x88, 48}, time.Now())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "truncated 0x88 frame") {
		t.Fatalf("error = %q", err)
	}
}

func TestTranslateRFPacketRawMessage(t *testing.T) {
	ev := translate(protocol.RawMessage{
		Type:    0x88,
		Payload: []byte{48, 0xc7, 0x80, 0x8e, 0xa8},
		Push:    true,
	})
	rf, ok := ev.(RFPacketReceived)
	if !ok {
		t.Fatalf("translate = %T, want RFPacketReceived", ev)
	}
	if rf.SNR != 12 || rf.RSSI != -57 || string(rf.Bytes) != string([]byte{0x80, 0x8e, 0xa8}) {
		t.Fatalf("rf = %+v", rf)
	}
}
