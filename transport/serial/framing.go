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
//	host -> device:  '>' | len(2, LE) | payload
//	device -> host:  '<' | len(2, LE) | payload
const (
	frameToDevice byte = '>' // 0x3e
	frameToHost   byte = '<' // 0x3c

	// maxFrameLen guards against runaway lengths from a desynchronised stream.
	maxFrameLen = 8192
)

// writeFrame writes a host->device frame for payload.
func writeFrame(w io.Writer, payload []byte) error {
	if len(payload) > maxFrameLen {
		return fmt.Errorf("serial: payload too large (%d bytes)", len(payload))
	}
	hdr := [3]byte{frameToDevice, 0, 0}
	binary.LittleEndian.PutUint16(hdr[1:], uint16(len(payload)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

// readFrame reads the next device->host frame, resynchronising on the '<'
// marker so leading noise (e.g. boot banners) is skipped.
func readFrame(r *bufio.Reader) ([]byte, error) {
	// Resynchronise to the next inbound marker.
	for {
		b, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		if b == frameToHost {
			break
		}
	}

	var lenBuf [2]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		return nil, err
	}
	n := int(binary.LittleEndian.Uint16(lenBuf[:]))
	if n > maxFrameLen {
		return nil, fmt.Errorf("serial: frame length %d exceeds maximum", n)
	}

	payload := make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}
