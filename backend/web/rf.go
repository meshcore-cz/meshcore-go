package web

import (
	"encoding/hex"
	"time"

	meshcore "github.com/meshcore-cz/meshcore-go"
	"github.com/meshcore-cz/meshpkt"
)

// rfLogEntry is a decoded over-the-air packet for the RF log: direction (rx/tx),
// the radio's measured signal (rx, from the companion 0x88 frame) or send
// priority (tx), plus the meshpkt-decoded envelope. Only the basic envelope is
// decoded; encrypted payloads stay opaque.
type rfLogEntry struct {
	Timestamp   time.Time `json:"timestamp"`
	Direction   string    `json:"direction"` // "rx" | "tx"
	SNR         float64   `json:"snr,omitempty"`
	RSSI        int       `json:"rssi,omitempty"`
	Priority    byte      `json:"priority,omitempty"`
	Type        string    `json:"type,omitempty"`
	Route       string    `json:"route,omitempty"`
	HopCount    int       `json:"hop_count"`
	Length      int       `json:"length"`
	Bytes       string    `json:"bytes"` // hex of the raw over-the-air packet
	DecodeError string    `json:"decode_error,omitempty"`
}

// rfEntry decodes an RF log packet into a display entry. A decode failure is
// surfaced (DecodeError) rather than dropped, so malformed/foreign packets still
// appear with their signal readings.
func rfEntry(p meshcore.RFPacket) rfLogEntry {
	e := rfLogEntry{
		Timestamp: p.Timestamp,
		Direction: p.Direction,
		SNR:       p.SNR,
		RSSI:      p.RSSI,
		Priority:  p.Priority,
		Length:    len(p.Bytes),
		Bytes:     hex.EncodeToString(p.Bytes),
	}
	pkt, err := meshpkt.DecodePacket(p.Bytes)
	if err != nil {
		e.DecodeError = err.Error()
		return e
	}
	e.Type = pkt.Type.String()
	e.Route = pkt.Route.String()
	e.HopCount = pkt.HopCount()
	return e
}
