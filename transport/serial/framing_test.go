package serial

import (
	"bufio"
	"bytes"
	"testing"
)

func TestFrameRoundTrip(t *testing.T) {
	payload := []byte{0x05, 0x01, 0x02, 0x03, 0xff}

	var buf bytes.Buffer
	if err := writeFrame(&buf, payload); err != nil {
		t.Fatalf("writeFrame: %v", err)
	}

	// Host->device frames are marked '>'; rewrite to the inbound marker so the
	// reader (which expects device->host frames) can parse it back.
	framed := buf.Bytes()
	if framed[0] != frameToDevice {
		t.Fatalf("first byte = %#x, want %#x", framed[0], frameToDevice)
	}
	framed[0] = frameToHost

	got, err := readFrame(bufio.NewReader(bytes.NewReader(framed)))
	if err != nil {
		t.Fatalf("readFrame: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("round trip = %x, want %x", got, payload)
	}
}

func TestReadFrameResync(t *testing.T) {
	// Leading boot-banner noise before a valid inbound frame.
	payload := []byte{0x00}
	var frame bytes.Buffer
	frame.Write([]byte("garbage banner\r\n"))
	frame.WriteByte(frameToHost)
	frame.Write([]byte{0x01, 0x00}) // length 1, LE
	frame.Write(payload)

	got, err := readFrame(bufio.NewReader(&frame))
	if err != nil {
		t.Fatalf("readFrame: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("got %x, want %x", got, payload)
	}
}

func TestWriteFrameTooLarge(t *testing.T) {
	var buf bytes.Buffer
	if err := writeFrame(&buf, make([]byte, maxFrameLen+1)); err == nil {
		t.Error("expected error for oversized payload")
	}
}
