package meshpkt

import (
	"crypto/aes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"testing"
	"time"

	meshcore "github.com/meshcore-cz/meshcore-go"
)

func TestGroupTextPacket(t *testing.T) {
	secret := meshcore.DeriveChannelSecret("general")
	ts := time.Unix(1700000000, 0)

	pkt, err := GroupTextPacket(secret, "Alice", "hello", ts)
	if err != nil {
		t.Fatal(err)
	}
	if len(pkt) < 3 {
		t.Fatalf("packet too short: %d bytes", len(pkt))
	}

	// header = (PAYLOAD_TYPE_GRP_TXT=5 << 2) | ROUTE_TYPE_FLOOD=1 = 0x15
	if pkt[0] != 0x15 {
		t.Fatalf("header = %02x, want 0x15", pkt[0])
	}
	// path_len = 0x40: path hash size=2 (bits 7-6 = 01), hop_count=0
	if pkt[1] != 0x40 {
		t.Fatalf("path_len = %02x, want 0x40 (2-byte path hashes, 0 hops)", pkt[1])
	}

	payload := pkt[2:]

	// First byte is channel hash: SHA256(secret[:16])[0] — always 1 byte.
	if payload[0] != meshcore.ChannelHash(secret) {
		t.Fatalf("channel_hash = %02x, want %02x", payload[0], meshcore.ChannelHash(secret))
	}

	mac := payload[1:3]
	ciphertext := payload[3:]

	// Verify MAC: HMAC-SHA256(hmacKey, ciphertext)[:2]
	hmacKey := make([]byte, 32)
	copy(hmacKey, secret[:16])
	h := hmac.New(sha256.New, hmacKey)
	h.Write(ciphertext)
	wantMAC := h.Sum(nil)[:2]
	if mac[0] != wantMAC[0] || mac[1] != wantMAC[1] {
		t.Fatalf("mac = %x, want %x", mac, wantMAC)
	}

	// Decrypt and verify plaintext.
	block, _ := aes.NewCipher(secret[:16])
	plaintext := make([]byte, len(ciphertext))
	for i := 0; i < len(ciphertext); i += 16 {
		block.Decrypt(plaintext[i:i+16], ciphertext[i:i+16])
	}

	gotTS := binary.LittleEndian.Uint32(plaintext[0:4])
	if gotTS != uint32(ts.Unix()) {
		t.Fatalf("timestamp = %d, want %d", gotTS, ts.Unix())
	}
	if plaintext[4] != 0 {
		t.Fatalf("txt_type = %d, want 0", plaintext[4])
	}
	want := "Alice: hello"
	got := string(plaintext[5 : 5+len(want)])
	if got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
}

func TestGroupTextPacketPathHashSize1(t *testing.T) {
	secret := meshcore.DeriveChannelSecret("general")
	ts := time.Unix(1700000000, 0)

	pkt, err := GroupTextPacket(secret, "Alice", "hello", ts, WithPathHashSize(1))
	if err != nil {
		t.Fatal(err)
	}
	// path_len = 0x00: path hash size=1 (bits 7-6 = 00), hop_count=0
	if pkt[1] != 0x00 {
		t.Fatalf("path_len = %02x, want 0x00 (1-byte path hashes, 0 hops)", pkt[1])
	}
	// Payload channel hash is still 1 byte regardless of path hash size.
	wantHash := sha256.Sum256(secret[:16])
	if pkt[2] != wantHash[0] {
		t.Fatalf("channel_hash = %02x, want %02x", pkt[2], wantHash[0])
	}
}
