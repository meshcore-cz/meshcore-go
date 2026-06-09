package meshpkt

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"testing"
	"time"
)

// ── envelope ─────────────────────────────────────────────────────────────────

func TestEncodeDecodePacket_Flood(t *testing.T) {
	want := Packet{
		Route:        RouteFlood,
		Type:         PayloadGrpTxt,
		Version:      0,
		PathHashSize: 2,
		Payload:      []byte{0x01, 0x02, 0x03},
	}
	raw, err := EncodePacket(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodePacket(raw)
	if err != nil {
		t.Fatal(err)
	}
	checkPacket(t, got, want)
}

func TestEncodeDecodePacket_WithPath(t *testing.T) {
	// 3 hops × 2-byte hashes
	want := Packet{
		Route:        RouteFlood,
		Type:         PayloadTxtMsg,
		Version:      1,
		PathHashSize: 2,
		Path:         []byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF},
		Payload:      []byte{0x42},
	}
	raw, err := EncodePacket(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodePacket(raw)
	if err != nil {
		t.Fatal(err)
	}
	checkPacket(t, got, want)
	if got.HopCount() != 3 {
		t.Fatalf("HopCount = %d, want 3", got.HopCount())
	}
	hops := got.Hops()
	if len(hops) != 3 {
		t.Fatalf("len(Hops) = %d, want 3", len(hops))
	}
	if !bytes.Equal(hops[0], []byte{0xAA, 0xBB}) {
		t.Fatalf("hops[0] = %x, want aabb", hops[0])
	}
}

func TestEncodeDecodePacket_TransportCodes(t *testing.T) {
	want := Packet{
		Route:          RouteTransportFlood,
		Type:           PayloadAdvert,
		Version:        2,
		TransportCodes: [2]uint16{0x1234, 0x5678},
		PathHashSize:   1,
		Payload:        []byte{0xFF},
	}
	raw, err := EncodePacket(want)
	if err != nil {
		t.Fatal(err)
	}
	// header(1) + transport_codes(4) + path_len(1) + payload(1) = 7
	if len(raw) != 7 {
		t.Fatalf("encoded length = %d, want 7", len(raw))
	}
	got, err := DecodePacket(raw)
	if err != nil {
		t.Fatal(err)
	}
	checkPacket(t, got, want)
	if got.TransportCodes != want.TransportCodes {
		t.Fatalf("TransportCodes = %v, want %v", got.TransportCodes, want.TransportCodes)
	}
}

func TestDecodePacket_TooShort(t *testing.T) {
	if _, err := DecodePacket([]byte{0x15}); err == nil {
		t.Fatal("expected error for 1-byte input")
	}
}

func checkPacket(t *testing.T, got, want Packet) {
	t.Helper()
	if got.Route != want.Route {
		t.Errorf("Route = %v, want %v", got.Route, want.Route)
	}
	if got.Type != want.Type {
		t.Errorf("Type = %v, want %v", got.Type, want.Type)
	}
	if got.Version != want.Version {
		t.Errorf("Version = %d, want %d", got.Version, want.Version)
	}
	if got.PathHashSize != want.PathHashSize {
		t.Errorf("PathHashSize = %d, want %d", got.PathHashSize, want.PathHashSize)
	}
	if !bytes.Equal(got.Path, want.Path) {
		t.Errorf("Path = %x, want %x", got.Path, want.Path)
	}
	if !bytes.Equal(got.Payload, want.Payload) {
		t.Errorf("Payload = %x, want %x", got.Payload, want.Payload)
	}
}

// ── channel helpers ───────────────────────────────────────────────────────────

func TestDeriveChannelSecret(t *testing.T) {
	secret := DeriveChannelSecret("#test")
	if len(secret) != ChannelSecretLen {
		t.Fatalf("secret length = %d, want %d", len(secret), ChannelSecretLen)
	}
	if !bytes.Equal(secret, DeriveChannelSecret("#test")) {
		t.Fatal("DeriveChannelSecret is not deterministic")
	}
}

func TestChannelHash(t *testing.T) {
	secret := DeriveChannelSecret("general")
	h := ChannelHash(secret)
	want := sha256.Sum256(secret[:16])
	if h != want[0] {
		t.Fatalf("ChannelHash = %02x, want %02x", h, want[0])
	}
}

// ── GRP_TXT round-trip ────────────────────────────────────────────────────────

func TestGroupTextPacket_RoundTrip(t *testing.T) {
	secret := DeriveChannelSecret("general")
	ts := time.Unix(1700000000, 0)

	raw, err := GroupTextPacket(secret, "Alice", "hello meshcore", ts)
	if err != nil {
		t.Fatal(err)
	}
	pkt, err := DecodePacket(raw)
	if err != nil {
		t.Fatal(err)
	}
	if pkt.Type != PayloadGrpTxt {
		t.Fatalf("Type = %v, want GRP_TXT", pkt.Type)
	}
	if pkt.Route != RouteFlood {
		t.Fatalf("Route = %v, want flood", pkt.Route)
	}
	if pkt.PathHashSize != 2 {
		t.Fatalf("PathHashSize = %d, want 2 (default)", pkt.PathHashSize)
	}

	gt, err := DecodeGroupTextPayload(secret, pkt.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if gt.Sender != "Alice" {
		t.Errorf("Sender = %q, want %q", gt.Sender, "Alice")
	}
	if gt.Text != "hello meshcore" {
		t.Errorf("Text = %q, want %q", gt.Text, "hello meshcore")
	}
	if gt.Timestamp.Unix() != ts.Unix() {
		t.Errorf("Timestamp = %v, want %v", gt.Timestamp, ts)
	}
	if gt.TxtType != 0 {
		t.Errorf("TxtType = %d, want 0", gt.TxtType)
	}
}

func TestGroupTextPacket_WrongSecret(t *testing.T) {
	secret := DeriveChannelSecret("#right")
	wrong := DeriveChannelSecret("#wrong")
	raw, err := GroupTextPacket(secret, "Bob", "test", time.Unix(1700000000, 0))
	if err != nil {
		t.Fatal(err)
	}
	pkt, _ := DecodePacket(raw)
	if _, err := DecodeGroupTextPayload(wrong, pkt.Payload); err == nil {
		t.Fatal("expected error decoding with wrong secret")
	}
}

func TestGroupTextPacket_PathHashSize1(t *testing.T) {
	secret := DeriveChannelSecret("general")
	raw, err := GroupTextPacket(secret, "Alice", "hello", time.Unix(1700000000, 0), WithPathHashSize(1))
	if err != nil {
		t.Fatal(err)
	}
	pkt, err := DecodePacket(raw)
	if err != nil {
		t.Fatal(err)
	}
	if pkt.PathHashSize != 1 {
		t.Fatalf("PathHashSize = %d, want 1", pkt.PathHashSize)
	}
	// path_len byte: bits7-6=00 (1-byte hashes), bits5-0=0 (0 hops) → 0x00
	if raw[1] != 0x00 {
		t.Fatalf("path_len = %02x, want 0x00", raw[1])
	}
	// channel_hash in payload is always 1 byte
	if pkt.Payload[0] != ChannelHash(secret) {
		t.Fatalf("channel_hash = %02x, want %02x", pkt.Payload[0], ChannelHash(secret))
	}
}

// ── TXT_MSG round-trip ────────────────────────────────────────────────────────

func TestDirectTextPacket_RoundTrip(t *testing.T) {
	shared, _ := hex.DecodeString("0102030405060708090a0b0c0d0e0f10")
	ts := time.Unix(1700000000, 0)
	destHash := byte(0xAB)
	srcHash := byte(0xCD)

	raw, err := DirectTextPacket(shared, destHash, srcHash, "direct hello", ts, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	pkt, err := DecodePacket(raw)
	if err != nil {
		t.Fatal(err)
	}
	if pkt.Type != PayloadTxtMsg {
		t.Fatalf("Type = %v, want TXT_MSG", pkt.Type)
	}

	dt, err := DecodeDirectTextPayload(shared, pkt.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if dt.DestHash != destHash {
		t.Errorf("DestHash = %02x, want %02x", dt.DestHash, destHash)
	}
	if dt.SrcHash != srcHash {
		t.Errorf("SrcHash = %02x, want %02x", dt.SrcHash, srcHash)
	}
	if dt.Text != "direct hello" {
		t.Errorf("Text = %q, want %q", dt.Text, "direct hello")
	}
	if dt.Timestamp.Unix() != ts.Unix() {
		t.Errorf("Timestamp = %d, want %d", dt.Timestamp.Unix(), ts.Unix())
	}
}

func TestDirectTextPacket_WrongSecret(t *testing.T) {
	shared, _ := hex.DecodeString("0102030405060708090a0b0c0d0e0f10")
	wrong, _ := hex.DecodeString("ffffffffffffffffffffffffffffffff")

	raw, _ := DirectTextPacket(shared, 0xAB, 0xCD, "test", time.Unix(1700000000, 0), 0, 0)
	pkt, _ := DecodePacket(raw)
	if _, err := DecodeDirectTextPayload(wrong, pkt.Payload); err == nil {
		t.Fatal("expected error decoding with wrong shared secret")
	}
}

// ── ADVERT decode ─────────────────────────────────────────────────────────────

func TestDecodeAdvertPayload(t *testing.T) {
	payload := make([]byte, 100)
	for i := 0; i < 32; i++ {
		payload[i] = 0x42 // pubkey
	}
	binary.LittleEndian.PutUint32(payload[32:], 1700000000)
	for i := 36; i < 100; i++ {
		payload[i] = 0x55 // signature
	}
	// Appdata: flags=0 (no GPS), then name
	payload = append(payload, 0x00)
	payload = append(payload, "TestNode"...)

	adv, err := DecodeAdvertPayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(adv.PublicKey) != 32 || adv.PublicKey[0] != 0x42 {
		t.Errorf("PublicKey[0] = %02x, want 0x42", adv.PublicKey[0])
	}
	if adv.Timestamp.Unix() != 1700000000 {
		t.Errorf("Timestamp = %v", adv.Timestamp)
	}
	if adv.Name != "TestNode" {
		t.Errorf("Name = %q, want TestNode", adv.Name)
	}
	if adv.HasGPS {
		t.Error("HasGPS should be false when flags=0")
	}
}

func TestDecodeAdvertPayload_TooShort(t *testing.T) {
	if _, err := DecodeAdvertPayload(make([]byte, 50)); err == nil {
		t.Fatal("expected error for payload shorter than 100 bytes")
	}
}
