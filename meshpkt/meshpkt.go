// Package meshpkt builds MeshCore packet bytes for use with
// Client.SendMeshPacket (CMD_SEND_RAW_PACKET, firmware PR #2543).
//
// Packet structure (Packet::writeTo wire format):
//
//	[header][path_len][path bytes][payload bytes]
//
// Header byte layout:
//
//	bits 1-0  route type  (0=transport-flood, 1=flood, 2=direct, 3=transport-direct)
//	bits 5-2  payload type
//	bits 7-6  payload version (0 = v1)
//
// path_len byte layout:
//
//	bits 7-6  path hash size - 1  (0=1-byte, 1=2-byte, 2=3-byte, 3=4-byte)
//	bits 5-0  hop count (0 for a fresh flood packet — no path bytes follow)
//
// Group channel text message payload layout (PAYLOAD_TYPE_GRP_TXT):
//
//	[channel_hash(1)][hmac_sha256[:2]][aes128ecb(plaintext)]
//
// The channel_hash is always 1 byte and is independent of the path hash size.
// Plaintext layout:
//
//	[timestamp(4 LE)][txt_type(1)][sender_name ": " message_text]
package meshpkt

import (
	"crypto/aes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"time"
)

// Packet header constants (from MeshCore Packet.h).
const (
	routeTypeFlood    byte = 0x01
	payloadTypeGRPTxt byte = 0x05
	phTypeShift       byte = 2

	cipherKeySize = 16 // AES-128 key length (CIPHER_KEY_SIZE)
	cipherMACSize = 2  // truncated HMAC length (CIPHER_MAC_SIZE)
	cipherKeyFull = 32 // full key buffer size used for HMAC (PUB_KEY_SIZE)
	txtTypePlain  = 0  // TXT_TYPE_PLAIN

	defaultPathHashSize = 2 // 2-byte path hashes
)

// Option configures GroupTextPacket behavior.
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

// GroupTextPacket builds a flooded GRP_TXT wire packet for the given channel,
// sender name, message text, and timestamp (zero = now).
//
// secret must be the 16-byte channel PSK — use meshcore.DeriveChannelSecret or
// the Secret field from a Channel returned by Client.Channels(). The returned
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
	payload, err := encryptGroupPayload(secret, plaintext)
	if err != nil {
		return nil, err
	}

	// path_len: bits 7-6 = (pathHashSize-1), bits 5-0 = hop count (0).
	header := (payloadTypeGRPTxt << phTypeShift) | routeTypeFlood
	pathLen := byte((o.pathHashSize - 1) << 6)
	pkt := make([]byte, 0, 2+len(payload))
	pkt = append(pkt, header, pathLen)
	pkt = append(pkt, payload...)
	return pkt, nil
}

// buildGroupTextPlaintext assembles the unencrypted payload for a GRP_TXT
// message: [timestamp(4 LE)][txt_type(1)]["sender_name: text"].
func buildGroupTextPlaintext(senderName, text string, ts time.Time) []byte {
	prefix := senderName + ": " + text
	buf := make([]byte, 5+len(prefix))
	binary.LittleEndian.PutUint32(buf[0:4], uint32(ts.Unix()))
	buf[4] = txtTypePlain
	copy(buf[5:], prefix)
	return buf
}

// encryptGroupPayload builds the channel payload:
// [channel_hash(1)][hmac[:2]][aes128ecb(plaintext)].
//
// AES-128-ECB with zero-padding to block size (matches firmware Utils::encrypt).
// HMAC-SHA256 over ciphertext with the full 32-byte key buffer (upper 16 bytes
// zeroed), truncated to 2 bytes (matches firmware Utils::encryptThenMAC).
func encryptGroupPayload(secret []byte, plaintext []byte) ([]byte, error) {
	ciphertext, err := aes128ECBEncrypt(secret[:cipherKeySize], plaintext)
	if err != nil {
		return nil, err
	}

	// HMAC key is the 32-byte buffer: secret[:16] + zero[16].
	// Firmware passes channel.secret (PUB_KEY_SIZE=32 bytes) to resetHMAC.
	hmacKey := make([]byte, cipherKeyFull)
	copy(hmacKey, secret[:cipherKeySize])

	mac := hmacSHA256Truncated(hmacKey, ciphertext, cipherMACSize)
	hash := channelHash(secret)

	const pathHashSize = 1 // channel_hash is always 1 byte in the payload
	payload := make([]byte, 0, pathHashSize+cipherMACSize+len(ciphertext))
	payload = append(payload, hash)
	payload = append(payload, mac...)
	payload = append(payload, ciphertext...)
	return payload, nil
}

// channelHash returns SHA256(secret[:16])[0] — the 1-byte routing tag
// the firmware uses to match packets to channel slots.
func channelHash(secret []byte) byte {
	sum := sha256.Sum256(secret[:cipherKeySize])
	return sum[0]
}

// aes128ECBEncrypt encrypts src with AES-128-ECB (no IV, zero-padded to block
// size), matching the firmware's Utils::encrypt implementation.
func aes128ECBEncrypt(key, src []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("meshpkt: AES init: %w", err)
	}
	bs := block.BlockSize()
	padded := make([]byte, (len(src)+bs-1)/bs*bs)
	copy(padded, src)
	dst := make([]byte, len(padded))
	for i := 0; i < len(padded); i += bs {
		block.Encrypt(dst[i:i+bs], padded[i:i+bs])
	}
	return dst, nil
}

// hmacSHA256Truncated returns the first n bytes of HMAC-SHA256(key, data).
func hmacSHA256Truncated(key, data []byte, n int) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)[:n]
}
