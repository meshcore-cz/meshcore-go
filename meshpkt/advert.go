package meshpkt

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"time"
)

// Advert holds the decoded content of an ADVERT (node advertisement) packet.
// ADVERT payloads are unencrypted.
//
// Wire layout: [pubkey:32][ts:4 LE][sig:64][appdata...]
// Appdata: [flags:1][lat?:4 LE float32][lon?:4 LE float32][name...]
type Advert struct {
	PublicKey []byte    // 32-byte Ed25519 public key
	Timestamp time.Time // broadcast timestamp
	Signature []byte    // 64-byte Ed25519 signature
	Flags     byte      // appdata flags byte (0 if no appdata present)
	HasGPS    bool      // true when Lat/Lon are valid
	Lat, Lon  float64   // GPS coordinates in degrees (valid when HasGPS)
	Name      string    // node name extracted from appdata (best-effort)
}

// DecodeAdvertPayload decodes an ADVERT payload. Returns an error only if the
// payload is shorter than the required 100-byte fixed prefix; optional appdata
// fields are parsed on a best-effort basis.
func DecodeAdvertPayload(payload []byte) (Advert, error) {
	// Fixed prefix: pubkey(32) + ts(4) + sig(64) = 100 bytes minimum.
	if len(payload) < 100 {
		return Advert{}, fmt.Errorf("meshpkt: ADVERT payload too short (%d bytes, need at least 100)", len(payload))
	}

	a := Advert{
		PublicKey: make([]byte, 32),
		Signature: make([]byte, 64),
	}
	copy(a.PublicKey, payload[0:32])
	a.Timestamp = time.Unix(int64(binary.LittleEndian.Uint32(payload[32:36])), 0)
	copy(a.Signature, payload[36:100])

	if len(payload) <= 100 {
		return a, nil
	}

	// Appdata starts at offset 100.
	off := 100
	a.Flags = payload[off]
	off++

	// Bit 0: GPS coordinates present (lat:4 float32 LE, lon:4 float32 LE).
	if a.Flags&0x01 != 0 {
		if off+8 <= len(payload) {
			a.Lat = float64(math.Float32frombits(binary.LittleEndian.Uint32(payload[off:])))
			a.Lon = float64(math.Float32frombits(binary.LittleEndian.Uint32(payload[off+4:])))
			a.HasGPS = true
		}
		off += 8
	}

	// Remaining bytes are the node name (null-terminated or to end of payload).
	if off < len(payload) {
		name := string(payload[off:])
		if idx := strings.IndexByte(name, 0); idx >= 0 {
			name = name[:idx]
		}
		a.Name = strings.TrimSpace(name)
	}
	return a, nil
}
