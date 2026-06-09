package meshpkt

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"time"
)

// EncodeAdvertPayload encodes an Advert to its wire payload bytes.
//
// Wire layout: [pubkey:32][ts:4 LE][sig:64][flags:1][lat?:4][lon?:4][name]
//
// adv.PublicKey must be exactly 32 bytes. adv.Signature is zero-padded to
// 64 bytes if shorter. Lat/Lon are encoded as float32 LE when adv.HasGPS is true.
func EncodeAdvertPayload(adv Advert) ([]byte, error) {
	if len(adv.PublicKey) != 32 {
		return nil, fmt.Errorf("meshpkt: ADVERT public key must be 32 bytes, got %d", len(adv.PublicKey))
	}

	ts := adv.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}

	// Fixed prefix: pubkey(32) + ts(4) + sig(64)
	buf := make([]byte, 0, 101+len(adv.Name))
	buf = append(buf, adv.PublicKey...)

	var tsBytes [4]byte
	binary.LittleEndian.PutUint32(tsBytes[:], uint32(ts.Unix()))
	buf = append(buf, tsBytes[:]...)

	var sig [64]byte
	copy(sig[:], adv.Signature)
	buf = append(buf, sig[:]...)

	// Appdata: flags byte
	flags := adv.Flags
	if adv.HasGPS {
		flags |= 0x01
	}
	buf = append(buf, flags)

	// GPS coordinates (float32 LE)
	if adv.HasGPS {
		var latBytes, lonBytes [4]byte
		binary.LittleEndian.PutUint32(latBytes[:], math.Float32bits(float32(adv.Lat)))
		binary.LittleEndian.PutUint32(lonBytes[:], math.Float32bits(float32(adv.Lon)))
		buf = append(buf, latBytes[:]...)
		buf = append(buf, lonBytes[:]...)
	}

	// Node name
	if adv.Name != "" {
		buf = append(buf, []byte(adv.Name)...)
	}

	return buf, nil
}

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
