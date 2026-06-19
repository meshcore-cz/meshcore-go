package streamframe

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
)

// MeshCore companion stream framing. Each logical packet is wrapped with a
// one-byte direction marker, a little-endian uint16 length and the payload:
//
//	host -> device:  '<' | len(2, LE) | payload
//	device -> host:  '>' | len(2, LE) | payload
const (
	ToDevice byte = '<' // 0x3c
	ToHost   byte = '>' // 0x3e

	MaxLen = 8192
)

func Write(w io.Writer, marker byte, payload []byte) error {
	if len(payload) > MaxLen {
		return fmt.Errorf("stream frame: payload too large (%d bytes)", len(payload))
	}
	hdr := [3]byte{marker, 0, 0}
	binary.LittleEndian.PutUint16(hdr[1:], uint16(len(payload)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

func Read(r *bufio.Reader, marker byte) ([]byte, error) {
	return read(r, marker, false)
}

func ReadResync(r *bufio.Reader, marker byte) ([]byte, error) {
	return read(r, marker, true)
}

func read(r *bufio.Reader, marker byte, resyncBadLength bool) ([]byte, error) {
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
	if n > MaxLen {
		if resyncBadLength {
			return read(r, marker, true)
		}
		return nil, fmt.Errorf("stream frame: frame length %d exceeds maximum", n)
	}

	payload := make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}
