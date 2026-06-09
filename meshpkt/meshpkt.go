package meshpkt

import (
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

const (
	defaultPathHashSize = 2 // default path hash size for new packets
	txtTypePlain        = 0 // TXT_TYPE_PLAIN
)

// Option configures packet-building behaviour.
type Option func(*packetOptions)

type packetOptions struct {
	pathHashSize int
}

// WithPathHashSize sets the path hash size in bytes (1–4; default 2).
// This controls the path_len encoding (bits 7-6). For a fresh flood packet
// with 0 hops there are no path bytes, so this only affects the path_len byte.
func WithPathHashSize(n int) Option {
	return func(o *packetOptions) {
		o.pathHashSize = n
	}
}

// GroupText holds the decoded content of a GRP_TXT (channel text) message.
type GroupText struct {
	Timestamp time.Time
	TxtType   byte   // upper 6 bits of flags byte (0 = plain text)
	Attempt   byte   // lower 2 bits of flags byte
	Sender    string // extracted from "Sender: Text" plaintext format
	Text      string
}

// GroupTextPacket builds a flooded GRP_TXT wire packet for the given channel,
// sender name, message text, and timestamp (zero = now).
//
// secret must be the 16-byte channel PSK — use DeriveChannelSecret or the
// Secret field from a Channel returned by Client.Channels(). The returned
// bytes are ready to pass to Client.SendMeshPacket.
//
// The default path hash size is 2 bytes; use WithPathHashSize to override.
func GroupTextPacket(secret []byte, senderName, text string, ts time.Time, opts ...Option) ([]byte, error) {
	o := &packetOptions{pathHashSize: defaultPathHashSize}
	for _, opt := range opts {
		opt(o)
	}
	if o.pathHashSize < 1 || o.pathHashSize > 4 {
		return nil, fmt.Errorf("meshpkt: unsupported path hash size %d (use 1–4)", o.pathHashSize)
	}
	if len(secret) < cipherKeySize {
		return nil, fmt.Errorf("meshpkt: channel secret too short (%d bytes, need %d)", len(secret), cipherKeySize)
	}
	if ts.IsZero() {
		ts = time.Now()
	}

	plaintext := buildGroupTextPlaintext(senderName, text, ts)
	payload, err := buildGroupTextPayload(secret, plaintext)
	if err != nil {
		return nil, err
	}
	return EncodePacket(Packet{
		Route:        RouteFlood,
		Type:         PayloadGrpTxt,
		PathHashSize: o.pathHashSize,
		Payload:      payload,
	})
}

// DecodeGroupTextPayload decodes a GRP_TXT payload using the given channel
// secret. Returns an error if the secret is wrong (MAC mismatch), the payload
// is malformed, or decryption fails.
//
// payload is the Payload field of a Packet returned by DecodePacket.
func DecodeGroupTextPayload(secret, payload []byte) (GroupText, error) {
	if len(secret) < cipherKeySize {
		return GroupText{}, fmt.Errorf("meshpkt: channel secret too short")
	}
	// payload = [channel_hash:1][mac:2][ciphertext]
	if len(payload) < 1+cipherMACSize+cipherKeySize {
		return GroupText{}, fmt.Errorf("meshpkt: GRP_TXT payload too short (%d bytes)", len(payload))
	}
	gotHash := payload[0]
	wantHash := ChannelHash(secret)
	if gotHash != wantHash {
		return GroupText{}, fmt.Errorf("meshpkt: channel hash mismatch (got %02x, want %02x) — wrong channel secret?", gotHash, wantHash)
	}
	mac := payload[1:3]
	ciphertext := payload[3:]
	plaintext, ok, err := openMAC(secret[:cipherKeySize], mac, ciphertext)
	if err != nil {
		return GroupText{}, fmt.Errorf("meshpkt: GRP_TXT decrypt: %w", err)
	}
	if !ok {
		return GroupText{}, fmt.Errorf("meshpkt: GRP_TXT MAC verification failed — wrong channel secret?")
	}
	return parseGroupTextPlaintext(plaintext)
}

func buildGroupTextPlaintext(senderName, text string, ts time.Time) []byte {
	combined := senderName + ": " + text
	buf := make([]byte, 5+len(combined))
	binary.LittleEndian.PutUint32(buf[0:4], uint32(ts.Unix()))
	buf[4] = txtTypePlain
	copy(buf[5:], combined)
	return buf
}

func buildGroupTextPayload(secret, plaintext []byte) ([]byte, error) {
	mac, ciphertext, err := sealMAC(secret[:cipherKeySize], plaintext)
	if err != nil {
		return nil, err
	}
	payload := make([]byte, 0, 1+cipherMACSize+len(ciphertext))
	payload = append(payload, ChannelHash(secret))
	payload = append(payload, mac...)
	payload = append(payload, ciphertext...)
	return payload, nil
}

// GroupTextPacketFromName derives the channel secret from name then encodes a
// GRP_TXT packet. This is a convenience wrapper around DeriveChannelSecret +
// GroupTextPacket.
func GroupTextPacketFromName(name, senderName, text string, ts time.Time, opts ...Option) ([]byte, error) {
	return GroupTextPacket(DeriveChannelSecret(name), senderName, text, ts, opts...)
}

// DecodeGroupTextPayloadByName derives the channel secret from name then
// decodes a GRP_TXT payload. This is a convenience wrapper around
// DeriveChannelSecret + DecodeGroupTextPayload.
func DecodeGroupTextPayloadByName(name string, payload []byte) (GroupText, error) {
	return DecodeGroupTextPayload(DeriveChannelSecret(name), payload)
}

func parseGroupTextPlaintext(plain []byte) (GroupText, error) {
	if len(plain) < 5 {
		return GroupText{}, fmt.Errorf("meshpkt: plaintext too short (%d bytes)", len(plain))
	}
	tsUnix := int64(binary.LittleEndian.Uint32(plain[0:4]))
	flags := plain[4]
	body := strings.TrimRight(string(plain[5:]), "\x00")

	var sender, text string
	if idx := strings.Index(body, ": "); idx >= 0 {
		sender = body[:idx]
		text = body[idx+2:]
	} else {
		text = body
	}
	return GroupText{
		Timestamp: time.Unix(tsUnix, 0),
		TxtType:   flags >> 2,
		Attempt:   flags & 0x03,
		Sender:    sender,
		Text:      text,
	}, nil
}
