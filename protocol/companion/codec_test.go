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
	b.Write(le32(0)) // reserved
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

// Golden fixtures captured from real MeshCore v1.15 hardware (Heltec V3).

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestDecodeRealSelfInfoGolden(t *testing.T) {
	pkt := mustHex(t,
		"05 01 16 16 ef f0 1e f2 18 05 fb 30 9c 2d ba 60 73 ac 39 54 51 1a 2b 08 be 41 c9 bc 78 7c c1 2a 41 87 9a a7 00 00 00 00 00 00 00 00 00 00 00 00 38 44 0d 00 24 f4 00 00 07 05 45 46 46 30 31 45 46 32")
	msg, err := decode(pkt)
	if err != nil {
		t.Fatal(err)
	}
	si := msg.(SelfInfo)
	if si.Name != "EFF01EF2" {
		t.Errorf("name = %q, want EFF01EF2", si.Name)
	}
	if !strings.HasPrefix(si.PublicKey, "eff01ef21805fb30") {
		t.Errorf("public key = %q", si.PublicKey)
	}
	if si.RadioSF != 7 || si.RadioCR != 5 {
		t.Errorf("sf/cr = %d/%d, want 7/5", si.RadioSF, si.RadioCR)
	}
	if si.TxPower != 22 {
		t.Errorf("tx power = %d, want 22", si.TxPower)
	}
}

func TestEncodeMessagingCommands(t *testing.T) {
	if got, _ := encode(GetContacts{}); !bytes.Equal(got, []byte{cmdGetContacts}) {
		t.Errorf("GetContacts = %x", got)
	}
	if got, _ := encode(GetChannel{Index: 3}); !bytes.Equal(got, []byte{cmdGetChannel, 3}) {
		t.Errorf("GetChannel = %x", got)
	}
	if got, _ := encode(SyncNextMessage{}); !bytes.Equal(got, []byte{cmdSyncNextMessage}) {
		t.Errorf("SyncNextMessage = %x", got)
	}

	ts := time.Unix(1717675200, 0)
	got, err := encode(SendTextMessage{DestPublicKey: bytes.Repeat([]byte{0xaa}, 8), Text: "hi", Timestamp: ts})
	if err != nil {
		t.Fatal(err)
	}
	want := append([]byte{cmdSendTxtMsg, 0, 0}, le32(uint32(ts.Unix()))...)
	want = append(want, []byte{0xaa, 0xaa, 0xaa, 0xaa, 0xaa, 0xaa}...)
	want = append(want, []byte("hi")...)
	if !bytes.Equal(got, want) {
		t.Errorf("SendTextMessage = %x, want %x", got, want)
	}

	if _, err := encode(SendTextMessage{DestPublicKey: []byte{1, 2}, Text: "x"}); err == nil {
		t.Error("expected error for too-short recipient key")
	}

	gotc, _ := encode(SendChannelTextMessage{Channel: 0, Text: "hi", Timestamp: ts})
	wantc := append([]byte{cmdSendChannelTxt, 0, 0}, le32(uint32(ts.Unix()))...)
	wantc = append(wantc, []byte("hi")...)
	if !bytes.Equal(gotc, wantc) {
		t.Errorf("SendChannelTextMessage = %x, want %x", gotc, wantc)
	}
}

func TestEncodeSendTracePath(t *testing.T) {
	got, err := encode(SendTracePath{Tag: 0x11223344, Auth: 0x0a0b0c0d, Flags: 1})
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{cmdSendTracePath, 0x44, 0x33, 0x22, 0x11, 0x0d, 0x0c, 0x0b, 0x0a, 1}
	if !bytes.Equal(got, want) {
		t.Errorf("SendTracePath = %x, want %x", got, want)
	}
}

func TestDecodeTraceDataGolden(t *testing.T) {
	// Captured verbatim from MeshCore v1.15: trace through repeater 0x25.
	// flags, path_len=1, rsvd, tag, auth, hash 0x25, two SNR bytes (0x30, 0x2e).
	pkt := mustHex(t, "89 00 01 00 62 69 d9 49 00 00 00 00 25 30 2e")
	td := mustDecode(t, pkt).(TraceData)
	if td.Tag != 0x49d96962 {
		t.Errorf("tag = %08x, want 49d96962", td.Tag)
	}
	if len(td.Path) != 1 || td.Path[0] != 0x25 {
		t.Errorf("path = %x, want [25]", td.Path)
	}
	if len(td.SNRs) != 2 || td.SNRs[0] != 12 || td.SNRs[1] != 11.5 {
		t.Errorf("snrs = %v, want [12 11.5]", td.SNRs)
	}
	if !td.Async() {
		t.Error("trace data must be async")
	}
}

func TestDecodeRealContactGolden(t *testing.T) {
	// Captured verbatim from MeshCore v1.15: the "liba.meshcore.cz" repeater.
	pkt := mustHex(t,
		"03 bb ab 1a ad 30 e4 5c e4 83 65 98 3d f5 87 42 e2 8f bb a2 87 a8 12 e4 af 79 1d 2e 15 df a3 3b e6 02 00 ff 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 6c 69 62 61 2e 6d 65 73 68 63 6f 72 65 2e 63 7a 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 5f 0a 24 6a ba d1 fc 02 60 74 ba 00 6a 0a 24 6a")
	msg, err := decode(pkt)
	if err != nil {
		t.Fatal(err)
	}
	c := msg.(Contact)
	if c.Name != "liba.meshcore.cz" {
		t.Errorf("name = %q", c.Name)
	}
	if c.Type != 2 {
		t.Errorf("type = %d, want 2 (repeater)", c.Type)
	}
	if c.HasPath {
		t.Error("out_path_len 0xff should mean no path")
	}
	if !strings.HasPrefix(c.PublicKey, "bbab1aad30e4") {
		t.Errorf("public key = %q", c.PublicKey)
	}
	if c.LastAdvert.IsZero() || c.LastMod.IsZero() {
		t.Error("expected non-zero timestamps")
	}
}

func TestDecodeRealChannelInfoGolden(t *testing.T) {
	// Captured verbatim from MeshCore v1.15: the default "Public" channel.
	pkt := mustHex(t,
		"12 00 50 75 62 6c 69 63 00 51 53 25 25 a0 25 a1 b1 9c 35 f9 d8 bb ee ac 74 44 03 2d f3 14 f4 52 de b5 8b 33 87 e9 c5 cd ea 6a c9 e5 ed ba a1 15 cd 72")
	msg, err := decode(pkt)
	if err != nil {
		t.Fatal(err)
	}
	ci := msg.(ChannelInfo)
	if ci.Index != 0 || ci.Name != "Public" {
		t.Errorf("channel = %d/%q", ci.Index, ci.Name)
	}
	if hex.EncodeToString(ci.Secret) != "8b3387e9c5cdea6ac9e5edbaa115cd72" {
		t.Errorf("secret = %x", ci.Secret)
	}
}

func TestDecodeMessageFrames(t *testing.T) {
	// Sent response (firmware-derived layout).
	sent := mustHex(t, "06 00 78 56 34 12 e8 03 00 00")
	if m := mustDecode(t, sent).(Sent); m.ExpectedAck != 0x12345678 {
		t.Errorf("sent ack = %08x", m.ExpectedAck)
	}

	// NO_MORE_MESSAGES.
	if _, ok := mustDecode(t, []byte{respNoMoreMessages}).(NoMoreMessages); !ok {
		t.Error("expected NoMoreMessages")
	}

	// CONTACT_MSG_RECV: prefix(6) path txt ts(4) "hello".
	cm := mustHex(t, "07 aabbccddeeff 00 00 80 a1 24 6a 68 65 6c 6c 6f")
	m := mustDecode(t, cm).(ContactMessage)
	if m.Text != "hello" || m.SenderPrefix != "aabbccddeeff" {
		t.Errorf("contact msg = %+v", m)
	}
}

func mustDecode(t *testing.T, pkt []byte) protocol.Message {
	t.Helper()
	m, err := decode(pkt)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestDecodeRealDeviceInfoGolden(t *testing.T) {
	pkt := mustHex(t,
		"0d 0b af 28 00 00 00 00 31 39 2d 41 70 72 2d 32 30 32 36 00 48 65 6c 74 65 63 20 56 33 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 76 31 2e 31 35 2e 30 2d 64 65 65 33 65 32 36 00 00 00 00 00 00 00")
	msg, err := decode(pkt)
	if err != nil {
		t.Fatal(err)
	}
	d := msg.(DeviceInfo)
	if d.Model != "Heltec V3" {
		t.Errorf("model = %q, want Heltec V3", d.Model)
	}
	if d.BuildDate != "19-Apr-2026" {
		t.Errorf("build date = %q, want 19-Apr-2026", d.BuildDate)
	}
	if d.Version != "v1.15.0-dee3e26" {
		t.Errorf("version = %q, want v1.15.0-dee3e26", d.Version)
	}
}

func TestEncodeSendLogin(t *testing.T) {
	key := bytes.Repeat([]byte{0x25}, 32)
	got, err := encode(SendLogin{PublicKey: key, Password: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	want := append([]byte{cmdSendLogin}, key...)
	want = append(want, []byte("secret")...)
	if !bytes.Equal(got, want) {
		t.Fatalf("got %x, want %x", got, want)
	}
}

func TestEncodeHasConnection(t *testing.T) {
	key := bytes.Repeat([]byte{0x25}, 32)
	got, err := encode(HasConnection{PublicKey: key})
	if err != nil {
		t.Fatal(err)
	}
	want := append([]byte{cmdHasConnection}, key...)
	if !bytes.Equal(got, want) {
		t.Fatalf("got %x, want %x", got, want)
	}
}

func TestEncodeSendStatusReq(t *testing.T) {
	key := bytes.Repeat([]byte{0x25}, 32)
	got, err := encode(SendStatusReq{PublicKey: key})
	if err != nil {
		t.Fatal(err)
	}
	want := append([]byte{cmdSendStatusReq}, key...)
	if !bytes.Equal(got, want) {
		t.Fatalf("got %x, want %x", got, want)
	}
}

func TestDecodeContactMessageV3(t *testing.T) {
	prefix := []byte{0x25, 0x25, 0x25, 0xce, 0x52, 0x67}
	var body []byte
	body = append(body, 20, 0, 0) // SNR + reserved
	body = append(body, prefix...)
	body = append(body, 0xff, 1) // path_len, txt_type CLI
	body = append(body, le32(1717675200)...)
	body = append(body, []byte("12:34:56")...)
	pkt := append([]byte{respContactMsgRecvV3}, body...)
	msg, err := decode(pkt)
	if err != nil {
		t.Fatal(err)
	}
	cm, ok := msg.(ContactMessage)
	if !ok {
		t.Fatalf("decoded to %T, want ContactMessage", msg)
	}
	if cm.SenderPrefix != hex.EncodeToString(prefix) || cm.TxtType != 1 || cm.Text != "12:34:56" {
		t.Fatalf("decoded %#v", cm)
	}
	if cm.SNR != 5 {
		t.Fatalf("SNR = %v, want 5", cm.SNR)
	}
}

func TestDecodeChannelMessageV3(t *testing.T) {
	var body []byte
	body = append(body, 12, 0, 0) // SNR + reserved
	body = append(body, 2, 0xff, 0) // channel, path_len, txt_type plain
	body = append(body, le32(1717675200)...)
	body = append(body, []byte("hello")...)
	pkt := append([]byte{respChannelMsgRecvV3}, body...)
	msg, err := decode(pkt)
	if err != nil {
		t.Fatal(err)
	}
	ch, ok := msg.(ChannelMessage)
	if !ok {
		t.Fatalf("decoded to %T, want ChannelMessage", msg)
	}
	if ch.Channel != 2 || ch.Text != "hello" || ch.SNR != 3 {
		t.Fatalf("decoded %#v", ch)
	}
}

func TestDecodeStatusResponse(t *testing.T) {
	prefix := []byte{0x25, 0x25, 0x25, 0xce, 0x52, 0x67}
	stats := make([]byte, RepeaterStatsSize)
	binary.LittleEndian.PutUint16(stats[0:2], 3700)
	binary.LittleEndian.PutUint16(stats[2:4], 2)
	binary.LittleEndian.PutUint16(stats[4:6], 0xff8a) // -118
	binary.LittleEndian.PutUint16(stats[6:8], 0xffd6) // -42
	binary.LittleEndian.PutUint32(stats[8:12], 1200)
	binary.LittleEndian.PutUint32(stats[12:16], 800)
	binary.LittleEndian.PutUint32(stats[16:20], 3600)
	binary.LittleEndian.PutUint32(stats[20:24], 86400)
	binary.LittleEndian.PutUint32(stats[24:28], 50)
	binary.LittleEndian.PutUint32(stats[28:32], 30)
	binary.LittleEndian.PutUint32(stats[32:36], 600)
	binary.LittleEndian.PutUint32(stats[36:40], 400)
	binary.LittleEndian.PutUint16(stats[40:42], 1)
	binary.LittleEndian.PutUint16(stats[42:44], uint16(int16(34))) // 8.5 dB
	binary.LittleEndian.PutUint16(stats[44:46], 3)
	binary.LittleEndian.PutUint16(stats[46:48], 5)
	binary.LittleEndian.PutUint32(stats[48:52], 7200)
	binary.LittleEndian.PutUint32(stats[52:56], 2)

	pkt := append(append([]byte{pushStatusResp, 0x00}, prefix...), stats...)
	msg, err := decode(pkt)
	if err != nil {
		t.Fatal(err)
	}
	resp, ok := msg.(StatusResponse)
	if !ok {
		t.Fatalf("decoded to %T, want StatusResponse", msg)
	}
	want := RepeaterStats{
		BattMilliVolts:     3700,
		CurrTxQueueLen:     2,
		NoiseFloor:         -118,
		LastRSSI:           -42,
		NPacketsRecv:       1200,
		NPacketsSent:       800,
		TotalAirTimeSecs:   3600,
		TotalUpTimeSecs:    86400,
		NSentFlood:         50,
		NSentDirect:        30,
		NRecvFlood:         600,
		NRecvDirect:        400,
		ErrEvents:          1,
		LastSNR:            34,
		NDirectDups:        3,
		NFloodDups:         5,
		TotalRxAirTimeSecs: 7200,
		NRecvErrors:        2,
	}
	if !bytes.Equal(resp.PublicKeyPrefix, prefix) || resp.Stats == nil || *resp.Stats != want {
		t.Fatalf("decoded %#v, want stats %#v", resp, want)
	}
	if resp.Text != "" {
		t.Fatalf("decoded %#v, want empty text", resp)
	}

	textPkt := append(append([]byte{pushStatusResp, 0x00}, prefix...), []byte("sensor ok")...)
	msg, err = decode(textPkt)
	if err != nil {
		t.Fatal(err)
	}
	resp, ok = msg.(StatusResponse)
	if !ok {
		t.Fatalf("decoded to %T, want StatusResponse", msg)
	}
	if resp.Text != "sensor ok" {
		t.Fatalf("decoded %#v", resp)
	}
}

func TestDecodeLoginPush(t *testing.T) {
	prefix := []byte{0x25, 0x25, 0x25, 0xce, 0x52, 0x67}
	pkt := append([]byte{pushLoginSuccess, 0x01}, prefix...)
	msg, err := decode(pkt)
	if err != nil {
		t.Fatal(err)
	}
	ok, is := msg.(LoginSuccess)
	if !is {
		t.Fatalf("decoded to %T, want LoginSuccess", msg)
	}
	if ok.Permissions != 1 || !bytes.Equal(ok.PublicKeyPrefix, prefix) {
		t.Fatalf("decoded %#v", ok)
	}

	pkt = append([]byte{pushLoginFail, 0x00}, prefix...)
	msg, err = decode(pkt)
	if err != nil {
		t.Fatal(err)
	}
	fail, is := msg.(LoginFail)
	if !is {
		t.Fatalf("decoded to %T, want LoginFail", msg)
	}
	if !bytes.Equal(fail.PublicKeyPrefix, prefix) {
		t.Fatalf("decoded %#v", fail)
	}
}
