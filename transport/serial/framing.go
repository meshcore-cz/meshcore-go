package serial

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
)

// MeshCore companion "serial V3" framing. Each logical packet is wrapped with a
// one-byte direction marker, a little-endian uint16 length and the payload:
//
//	host -> device:  '<' | len(2, LE) | payload
//	device -> host:  '>' | len(2, LE) | payload
//
// (Verified against MeshCore firmware v1.15 on Heltec V3 hardware.)
const (
	frameToDevice byte = '<' // 0x3c
	frameToHost   byte = '>' // 0x3e

	// maxFrameLen guards against runaway lengths from a desynchronised stream.
	maxFrameLen = 8192
)

// WriteHostFrame writes a host->device frame for payload.
func WriteHostFrame(w io.Writer, payload []byte) error {
	return writeFrameWithMarker(w, frameToDevice, payload)
}

// WriteDeviceFrame writes a device->host frame for payload.
func WriteDeviceFrame(w io.Writer, payload []byte) error {
	return writeFrameWithMarker(w, frameToHost, payload)
}

// ReadHostFrame reads the next host->device frame.
func ReadHostFrame(r *bufio.Reader) ([]byte, error) {
	return readFrameWithMarker(r, frameToDevice)
}

// ReadHostFrameResync reads the next host->device frame and keeps scanning if
// an implausible length is found. This is useful for PTY bridges where opening
// the slave can produce terminal-control noise before the app sends binary
// MeshCore frames.
func ReadHostFrameResync(r *bufio.Reader) ([]byte, error) {
	return readFrameWithMarkerResync(r, frameToDevice)
}

// ReadDeviceFrame reads the next device->host frame.
func ReadDeviceFrame(r *bufio.Reader) ([]byte, error) {
	return readFrameWithMarker(r, frameToHost)
}

// writeFrame writes a host->device frame for payload.
func writeFrame(w io.Writer, payload []byte) error {
	return WriteHostFrame(w, payload)
}

func writeFrameWithMarker(w io.Writer, marker byte, payload []byte) error {
	if len(payload) > maxFrameLen {
		return fmt.Errorf("serial: payload too large (%d bytes)", len(payload))
	}
	hdr := [3]byte{marker, 0, 0}
	binary.LittleEndian.PutUint16(hdr[1:], uint16(len(payload)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

// readFrame reads the next device->host frame, resynchronising on the '>'
// marker so leading noise (e.g. boot banners) is skipped.
func readFrame(r *bufio.Reader) ([]byte, error) {
	return ReadDeviceFrame(r)
}

func readFrameWithMarker(r *bufio.Reader, marker byte) ([]byte, error) {
	return readFrameWithMarkerMode(r, marker, false)
}

func readFrameWithMarkerResync(r *bufio.Reader, marker byte) ([]byte, error) {
	return readFrameWithMarkerMode(r, marker, true)
}

func readFrameWithMarkerMode(r *bufio.Reader, marker byte, resyncBadLength bool) ([]byte, error) {
	// Resynchronise to the next inbound marker.
	for {
		b, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		if b == marker {
			break
		}
	}

	var lenBuf [2]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		return nil, err
	}
	n := int(binary.LittleEndian.Uint16(lenBuf[:]))
	if n > maxFrameLen {
		if resyncBadLength {
			return readFrameWithMarkerMode(r, marker, true)
		}
		return nil, fmt.Errorf("serial: frame length %d exceeds maximum", n)
	}

	payload := make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}
