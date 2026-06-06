package companion

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/meshcore-dev/meshcore-go/protocol"
)

func TestEncodeCommands(t *testing.T) {
	tests := []struct {
		name string
		cmd  protocol.Command
		want []byte
	}{
		{"get-time", GetDeviceTime{}, []byte{cmdGetDeviceTime}},
		{"battery", GetBatteryVoltage{}, []byte{cmdGetBattery}},
		{"reboot", Reboot{}, []byte{cmdReboot}},
		{"advert-flood", SendSelfAdvert{Flood: true}, []byte{cmdSendSelfAdvert, 1}},
		{"advert-zero", SendSelfAdvert{}, []byte{cmdSendSelfAdvert, 0}},
	}
	for _, tt := range tests {
		got, err := encode(tt.cmd)
		if err != nil {
			t.Errorf("%s: encode error %v", tt.name, err)
			continue
		}
		if !bytes.Equal(got, tt.want) {
			t.Errorf("%s: got %x, want %x", tt.name, got, tt.want)
		}
	}
}

func TestEncodeAppStart(t *testing.T) {
	got, err := encode(AppStart{Name: "mcr"})
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != cmdAppStart || got[1] != 3 {
		t.Fatalf("header = %x, want 0103", got[:2])
	}
	if !bytes.HasSuffix(got, []byte("mcr")) {
		t.Errorf("payload %x should end with app name", got)
	}
	if len(got) != 8+len("mcr") {
		t.Errorf("len = %d, want %d", len(got), 8+len("mcr"))
	}
}

func TestEncodeSetDeviceTime(t *testing.T) {
	ts := time.Unix(1717675200, 0)
	got, err := encode(SetDeviceTime{Time: ts})
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != cmdSetDeviceTime {
		t.Fatalf("code = %#x", got[0])
	}
	if secs := binary.LittleEndian.Uint32(got[1:]); secs != uint32(ts.Unix()) {
		t.Errorf("encoded time = %d, want %d", secs, ts.Unix())
	}
}

func TestDecodeSimpleResponses(t *testing.T) {
	if msg, _ := decode([]byte{respOK}); msg != (OK{}) {
		t.Errorf("respOK decoded to %T", msg)
	}

	msg, _ := decode([]byte{respErr, 7})
	if e, ok := msg.(Err); !ok || e.Code != 7 {
		t.Errorf("respErr decoded to %#v", msg)
	}

	tpkt := append([]byte{respCurrTime}, le32(1717675200)...)
	msg, _ = decode(tpkt)
	if ct, ok := msg.(CurrentTime); !ok || ct.Time.Unix() != 1717675200 {
		t.Errorf("respCurrTime decoded to %#v", msg)
	}

	bpkt := append([]byte{respBatteryVoltage}, le16(3700)...)
	msg, _ = decode(bpkt)
	if b, ok := msg.(BatteryVoltage); !ok || b.Millivolts != 3700 {
		t.Errorf("battery decoded to %#v", msg)
	}
}

func TestDecodeSelfInfoRoundTrip(t *testing.T) {
	key := bytes.Repeat([]byte{0xab}, 32)
	pkt := buildSelfInfo(t, selfInfoFixture{
		advType: 1, tx: 22, maxTx: 30,
		key:  key,
		freq: 868000, bw: 250, sf: 11, cr: 5,
		name: "MeshCore-desk",
	})

	msg, err := decode(pkt)
	if err != nil {
		t.Fatal(err)
	}
	si, ok := msg.(SelfInfo)
	if !ok {
		t.Fatalf("decoded to %T", msg)
	}
	if si.Name != "MeshCore-desk" {
		t.Errorf("name = %q", si.Name)
	}
	if si.PublicKey != hex.EncodeToString(key) {
		t.Errorf("key = %q", si.PublicKey)
	}
	if si.RadioFreq != 868000 || si.RadioSF != 11 || si.RadioCR != 5 {
		t.Errorf("radio params = %+v", si)
	}
	if si.Async() {
		t.Error("self info must not be async")
	}
}

func TestDecodeUnknownDegradesToRaw(t *testing.T) {
	// Unknown push (high bit set).
	msg, _ := decode([]byte{0xF0, 0x01, 0x02})
	raw, ok := msg.(protocol.RawMessage)
	if !ok {
		t.Fatalf("decoded to %T, want RawMessage", msg)
	}
	if raw.Type != 0xF0 || !raw.Push || !raw.Async() {
		t.Errorf("raw = %+v", raw)
	}

	// Unknown response (high bit clear).
	msg, _ = decode([]byte{0x7E})
	raw = msg.(protocol.RawMessage)
	if raw.Push {
		t.Error("unknown response should not be marked push")
	}
}

func TestDecodeAdvertIsAsync(t *testing.T) {
	key := bytes.Repeat([]byte{0x11}, 32)
	pkt := append([]byte{pushAdvert}, key...)
	pkt = append(pkt, []byte("alice")...)
	msg, _ := decode(pkt)
	a, ok := msg.(Advert)
	if !ok || !a.Async() {
		t.Fatalf("advert decoded to %#v", msg)
	}
	if a.Name != "alice" {
		t.Errorf("name = %q", a.Name)
	}
}

// --- fixture helpers ---

type selfInfoFixture struct {
	advType, tx, maxTx byte
	key                []byte
	freq, bw           uint32
	sf, cr             byte
	name               string
}

func buildSelfInfo(t *testing.T, f selfInfoFixture) []byte {
	t.Helper()
	if len(f.key) != 32 {
		t.Fatalf("key must be 32 bytes, got %d", len(f.key))
	}
	var b bytes.Buffer
	b.WriteByte(respSelfInfo)
	b.WriteByte(f.advType)
	b.WriteByte(f.tx)
	b.WriteByte(f.maxTx)
	b.Write(f.key)
	b.Write(le32(0)) // lat
	b.Write(le32(0)) // lon
	b.Write(le32(f.freq))
	b.Write(le32(f.bw))
	b.WriteByte(f.sf)
	b.WriteByte(f.cr)
	b.WriteString(f.name)
	return b.Bytes()
}

func le16(v uint16) []byte {
	b := make([]byte, 2)
	binary.LittleEndian.PutUint16(b, v)
	return b
}

func le32(v uint32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	return b
}

func TestSplitStrings(t *testing.T) {
	got := splitStrings([]byte("MeshCore\x00v1.2.3\x00"))
	if strings.Join(got, "|") != "MeshCore|v1.2.3" {
		t.Errorf("got %v", got)
	}
}
