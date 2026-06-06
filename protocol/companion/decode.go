package companion

import (
	"encoding/binary"
	"encoding/hex"
	"strings"
	"time"

	"github.com/meshcore-cz/meshcore-go/protocol"
)

// decode parses a complete companion-protocol packet. Unknown or short packets
// degrade to a protocol.RawMessage so callers stay tolerant of firmware that
// adds fields or packet types.
func decode(packet []byte) (protocol.Message, error) {
	if len(packet) == 0 {
		return protocol.RawMessage{Type: 0, Payload: nil}, nil
	}

	code := packet[0]
	body := packet[1:]

	switch code {
	case respOK:
		return OK{}, nil

	case respErr:
		e := Err{}
		if len(body) > 0 {
			e.Code = body[0]
		}
		return e, nil

	case respSelfInfo:
		return decodeSelfInfo(body), nil

	case respDeviceInfo:
		return decodeDeviceInfo(body), nil

	case respCurrTime:
		if len(body) < 4 {
			break
		}
		secs := binary.LittleEndian.Uint32(body)
		return CurrentTime{Time: time.Unix(int64(secs), 0)}, nil

	case respBatteryVoltage:
		if len(body) < 2 {
			break
		}
		return BatteryVoltage{Millivolts: binary.LittleEndian.Uint16(body)}, nil

	case respContactsStart:
		var c ContactsStart
		if len(body) >= 4 {
			c.Count = binary.LittleEndian.Uint32(body)
		}
		return c, nil

	case respContact:
		return decodeContact(body), nil

	case respEndOfContacts:
		return EndOfContacts{}, nil

	case respChannelInfo:
		return decodeChannelInfo(body), nil

	case respNoMoreMessages:
		return NoMoreMessages{}, nil

	case respSent:
		return decodeSent(body), nil

	case respContactMsgRecv, respContactMsgRecvV3:
		return decodeContactMessage(body, code == respContactMsgRecvV3), nil

	case respChannelMsgRecv, respChannelMsgRecvV3:
		return decodeChannelMessage(body, code == respChannelMsgRecvV3), nil

	case pushAdvert, pushNewAdvert:
		return decodeAdvert(body), nil

	case pushSendConfirmed:
		return decodeSendConfirmed(body), nil

	case pushMsgWaiting:
		return MsgWaiting{}, nil

	case pushTraceData:
		return decodeTraceData(body), nil

	case pushControlData:
		return decodeControlData(body), nil

	case pushLoginSuccess:
		return decodeLoginSuccess(body), nil

	case pushLoginFail:
		return decodeLoginFail(body), nil

	case pushStatusResp:
		return decodeStatusResponse(body), nil
	}

	// Unknown packet: preserve it for inspection.
	return protocol.RawMessage{
		Type:    code,
		Payload: body,
		Push:    isPush(code),
	}, nil
}

// decodeSelfInfo parses RESP_CODE_SELF_INFO.
//
// Layout (offsets relative to the body, i.e. after the code byte), verified
// against MeshCore firmware v1.15:
//
//	[0]      adv_type
//	[1]      tx_power
//	[2]      max_tx_power
//	[3:35]   public_key (32 bytes)
//	[35:39]  adv_lat   (int32 LE, units of 1e-6 degrees)
//	[39:43]  adv_lon   (int32 LE, units of 1e-6 degrees)
//	[43:47]  reserved
//	[47:51]  radio_freq (uint32 LE)
//	[51:55]  radio_bw   (uint32 LE)
//	[55]     radio_sf
//	[56]     radio_cr
//	[57:]    name (UTF-8, NUL-trimmed)
func decodeSelfInfo(b []byte) SelfInfo {
	var s SelfInfo
	get := func(i int) byte {
		if i < len(b) {
			return b[i]
		}
		return 0
	}
	u32 := func(i int) uint32 {
		if i+4 <= len(b) {
			return binary.LittleEndian.Uint32(b[i : i+4])
		}
		return 0
	}

	s.AdvType = get(0)
	s.TxPower = get(1)
	s.MaxTxPower = get(2)
	if len(b) >= 35 {
		s.PublicKey = hex.EncodeToString(b[3:35])
	}
	s.AdvLat = float64(int32(u32(35))) / 1e6
	s.AdvLon = float64(int32(u32(39))) / 1e6
	// b[43:47] reserved
	s.RadioFreq = u32(47)
	s.RadioBW = u32(51)
	s.RadioSF = get(55)
	s.RadioCR = get(56)
	if len(b) > 57 {
		s.Name = trimString(b[57:])
	}
	return s
}

// decodeDeviceInfo parses RESP_CODE_DEVICE_INFO.
//
// Layout: a small binary header followed by NUL-padded strings (build date,
// hardware model and firmware version). Verified against MeshCore v1.15, which
// returns e.g. "19-Apr-2026", "Heltec V3", "v1.15.0-dee3e26". Parsing is
// tolerant of ordering and extra fields.
func decodeDeviceInfo(b []byte) DeviceInfo {
	var d DeviceInfo
	if len(b) > 0 {
		d.FirmwareCode = b[0]
	}
	for _, tok := range printableTokens(b) {
		switch {
		case looksLikeVersion(tok):
			if d.Version == "" {
				d.Version = tok
			}
		case looksLikeDate(tok):
			if d.BuildDate == "" {
				d.BuildDate = tok
			}
		default:
			if d.Model == "" {
				d.Model = tok
			}
		}
	}
	return d
}

func decodeAdvert(b []byte) Advert {
	var a Advert
	if len(b) >= 32 {
		a.PublicKey = hex.EncodeToString(b[:32])
		a.Name = trimString(b[32:])
	} else {
		a.Name = trimString(b)
	}
	return a
}

func decodeSendConfirmed(b []byte) SendConfirmed {
	var s SendConfirmed
	if len(b) >= 4 {
		s.Code = binary.LittleEndian.Uint32(b[:4])
	}
	if len(b) >= 8 {
		// round-trip reported in milliseconds
		s.RoundTrip = time.Duration(binary.LittleEndian.Uint32(b[4:8])) * time.Millisecond
	}
	return s
}

// decodeContact parses RESP_CODE_CONTACT.
//
// Layout (verified against MeshCore v1.15, body length 147):
//
//	[0:32]    public_key
//	[32]      type (1=chat, 2=repeater, 3=room, 4=sensor)
//	[33]      flags
//	[34]      out_path_len (int8; -1/0xff = no path)
//	[35:99]   out_path (64 bytes)
//	[99:131]  adv_name (32 bytes, NUL-terminated)
//	[131:135] last_advert (uint32 LE)
//	[135:139] adv_lat (int32 LE, 1e-6 deg)
//	[139:143] adv_lon (int32 LE, 1e-6 deg)
//	[143:147] lastmod (uint32 LE)
func decodeContact(b []byte) Contact {
	var c Contact
	u32 := func(i int) uint32 {
		if i+4 <= len(b) {
			return binary.LittleEndian.Uint32(b[i : i+4])
		}
		return 0
	}
	if len(b) >= 32 {
		c.PublicKey = hex.EncodeToString(b[:32])
	}
	if len(b) > 32 {
		c.Type = b[32]
	}
	if len(b) > 33 {
		c.Flags = b[33]
	}
	if len(b) > 34 {
		c.HasPath = int8(b[34]) >= 0
	}
	if len(b) >= 131 {
		c.Name = trimString(b[99:131])
	}
	if t := u32(131); t != 0 {
		c.LastAdvert = time.Unix(int64(t), 0)
	}
	c.Latitude = float64(int32(u32(135))) / 1e6
	c.Longitude = float64(int32(u32(139))) / 1e6
	if t := u32(143); t != 0 {
		c.LastMod = time.Unix(int64(t), 0)
	}
	return c
}

// decodeChannelInfo parses RESP_CODE_CHANNEL_INFO.
//
// Layout (verified against MeshCore v1.15, body length 49):
//
//	[0]      channel index
//	[1:33]   name (32 bytes, NUL-terminated)
//	[33:49]  secret/PSK (16 bytes)
func decodeChannelInfo(b []byte) ChannelInfo {
	var ci ChannelInfo
	if len(b) > 0 {
		ci.Index = b[0]
	}
	if len(b) >= 33 {
		ci.Name = trimString(b[1:33])
	}
	if len(b) >= 49 {
		ci.Secret = append([]byte(nil), b[33:49]...)
	}
	return ci
}

// decodeSent parses RESP_CODE_SENT. Firmware-derived layout:
//
//	[0]     result
//	[1:5]   expected ack code (uint32 LE)
//	[5:9]   suggested timeout (uint32 LE, ms)
func decodeSent(b []byte) Sent {
	var s Sent
	if len(b) > 0 {
		s.Result = b[0]
	}
	if len(b) >= 5 {
		s.ExpectedAck = binary.LittleEndian.Uint32(b[1:5])
	}
	if len(b) >= 9 {
		s.SuggestedTimeout = time.Duration(binary.LittleEndian.Uint32(b[5:9])) * time.Millisecond
	}
	return s
}

// decodeContactMessage parses RESP_CODE_CONTACT_MSG_RECV[_V3].
//
// Standard layout:
//
//	[0:6]   sender public-key prefix
//	[6]     path_len
//	[7]     txt_type
//	[8:12]  sender_timestamp (uint32 LE)
//	[12:16] signature (txt_type == 2 only)
//	[...]   text
//
// V3 layout (verified against MeshCore companion radio v1.15):
//
//	[0]     SNR (int8, 0.25 dB units)
//	[1:3]   reserved
//	[3:9]   sender public-key prefix
//	[9]     path_len
//	[10]    txt_type
//	[11:15] sender_timestamp (uint32 LE)
//	[15:19] signature (txt_type == 2 only)
//	[...]   text
func decodeContactMessage(b []byte, v3 bool) ContactMessage {
	var m ContactMessage
	off := 0
	if v3 {
		if len(b) > 0 {
			m.SNR = float64(int8(b[0])) / 4
		}
		off = 3
	}
	if len(b) >= off+6 {
		m.SenderPrefix = hex.EncodeToString(b[off : off+6])
	}
	if len(b) > off+6 {
		m.PathLen = b[off+6]
	}
	if len(b) > off+7 {
		m.TxtType = b[off+7]
	}
	if len(b) >= off+12 {
		m.Timestamp = time.Unix(int64(binary.LittleEndian.Uint32(b[off+8 : off+12])), 0)
	}
	textOff := off + 12
	if m.TxtType == 2 && len(b) >= textOff+4 {
		textOff += 4
	}
	if len(b) > textOff {
		m.Text = trimString(b[textOff:])
	}
	return m
}

// decodeChannelMessage parses RESP_CODE_CHANNEL_MSG_RECV[_V3].
//
// Standard layout:
//
//	[0]     channel index
//	[1]     path_len
//	[2]     txt_type
//	[3:7]   sender_timestamp (uint32 LE)
//	[7:]    text
//
// V3 layout:
//
//	[0]     SNR (int8, 0.25 dB units)
//	[1:3]   reserved
//	[3]     channel index
//	[4]     path_len
//	[5]     txt_type
//	[6:10]  sender_timestamp (uint32 LE)
//	[10:]   text
func decodeChannelMessage(b []byte, v3 bool) ChannelMessage {
	var m ChannelMessage
	off := 0
	if v3 {
		if len(b) > 0 {
			m.SNR = float64(int8(b[0])) / 4
		}
		off = 3
	}
	if len(b) > off {
		m.Channel = b[off]
	}
	if len(b) > off+1 {
		m.PathLen = b[off+1]
	}
	if len(b) > off+2 {
		m.TxtType = b[off+2]
	}
	if len(b) >= off+7 {
		m.Timestamp = time.Unix(int64(binary.LittleEndian.Uint32(b[off+3 : off+7])), 0)
	}
	if len(b) > off+7 {
		m.Text = trimString(b[off+7:])
	}
	return m
}

// decodeTraceData parses PUSH_CODE_TRACE_DATA.
//
// Layout (verified against MeshCore v1.15):
//
//	[0]      flags
//	[1]      path_len (number of intermediate hops)
//	[2]      reserved
//	[3:7]    tag (uint32 LE)
//	[7:11]   auth (uint32 LE)
//	[11:11+n]   node hashes
//	[11+n:]     per-link SNR (int8, 0.25 dB units); usually n+1 values for the
//	            round trip (each forward hop plus the return to the origin)
func decodeLoginSuccess(b []byte) LoginSuccess {
	var l LoginSuccess
	if len(b) > 0 {
		l.Permissions = b[0]
	}
	if len(b) >= 7 {
		l.PublicKeyPrefix = append([]byte(nil), b[1:7]...)
	}
	if len(b) >= 11 {
		l.Tag = int32(binary.LittleEndian.Uint32(b[7:11]))
	}
	if len(b) >= 12 {
		l.NewPermissions = b[11]
	}
	return l
}

func decodeLoginFail(b []byte) LoginFail {
	var l LoginFail
	if len(b) >= 7 {
		l.PublicKeyPrefix = append([]byte(nil), b[1:7]...)
	}
	return l
}

func decodeStatusResponse(b []byte) StatusResponse {
	var s StatusResponse
	if len(b) >= 7 {
		s.PublicKeyPrefix = append([]byte(nil), b[1:7]...)
	}
	if len(b) > 7 {
		s.Stats, s.Text = decodeStatusData(b[7:])
	}
	return s
}

// decodeControlData parses PUSH_CODE_CONTROL_DATA.
//
// Layout (firmware-derived from the meshcore_py reader, not hardware-verified):
//
//	[0]    SNR (int8, 0.25 dB units)
//	[1]    RSSI (int8, dBm)
//	[2]    path_len
//	[3:]   payload; payload[0] is a sub-type whose high nibble identifies it
//
// A payload sub-type with high nibble 0x90 is a node-discovery response.
func decodeControlData(b []byte) protocol.Message {
	if len(b) < 4 {
		return protocol.RawMessage{Type: pushControlData, Payload: append([]byte(nil), b...), Push: true}
	}
	snr := float64(int8(b[0])) / 4
	rssi := int(int8(b[1]))
	pathLen := b[2]
	payload := b[3:]
	if payload[0]&0xF0 == controlNodeDiscoverResp {
		return decodeNodeDiscoverResp(payload, snr, rssi, pathLen)
	}
	return ControlData{
		SNR:         snr,
		RSSI:        rssi,
		PathLen:     pathLen,
		PayloadType: payload[0],
		Payload:     append([]byte(nil), payload[1:]...),
	}
}

// decodeNodeDiscoverResp parses a node-discovery reply carried in CONTROL_DATA.
//
// Payload layout (firmware-derived):
//
//	[0]    sub-type: high nibble 0x90, low nibble node_type
//	[1]    SNR_in (int8, 0.25 dB units) — how the remote heard our request
//	[2:6]  tag (uint32 LE) matching the request
//	[6:]   public key (8-byte prefix or full 32-byte key)
func decodeNodeDiscoverResp(payload []byte, snr float64, rssi int, pathLen byte) NodeDiscoverResp {
	r := NodeDiscoverResp{SNRDown: snr, RSSI: rssi, PathLen: pathLen}
	r.NodeType = payload[0] & 0x0F
	if len(payload) >= 2 {
		r.SNRUp = float64(int8(payload[1])) / 4
	}
	if len(payload) >= 6 {
		r.Tag = binary.LittleEndian.Uint32(payload[2:6])
	}
	if len(payload) > 6 {
		key := payload[6:]
		if len(key) >= 32 {
			key = key[:32]
		} else if len(key) >= 8 {
			key = key[:8]
		}
		r.PublicKey = hex.EncodeToString(key)
	}
	return r
}

func decodeTraceData(b []byte) TraceData {
	var t TraceData
	if len(b) > 0 {
		t.Flags = b[0]
	}
	if len(b) < 11 {
		return t
	}
	n := int(b[1])
	t.Tag = binary.LittleEndian.Uint32(b[3:7])
	t.Auth = binary.LittleEndian.Uint32(b[7:11])

	end := 11 + n
	if end > len(b) {
		end = len(b)
	}
	t.Path = append([]byte(nil), b[11:end]...)
	for _, s := range b[end:] {
		t.SNRs = append(t.SNRs, float64(int8(s))/4)
	}
	return t
}

// trimString decodes bytes as UTF-8 up to the first NUL.
func trimString(b []byte) string {
	if i := indexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return strings.TrimRight(string(b), "\x00")
}

// splitStrings splits a byte slice on NUL and newline into non-empty strings.
func splitStrings(b []byte) []string {
	raw := strings.FieldsFunc(string(b), func(r rune) bool {
		return r == 0 || r == '\n' || r == '\r'
	})
	out := make([]string, 0, len(raw))
	for _, s := range raw {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// printableTokens extracts runs of printable ASCII (length >= 2) from b,
// skipping binary header bytes and NUL padding.
func printableTokens(b []byte) []string {
	var out []string
	var cur []byte
	flush := func() {
		if t := strings.TrimSpace(string(cur)); len(t) >= 2 {
			out = append(out, t)
		}
		cur = cur[:0]
	}
	for _, c := range b {
		if c >= 0x20 && c < 0x7f {
			cur = append(cur, c)
		} else {
			flush()
		}
	}
	flush()
	return out
}

// looksLikeVersion reports whether tok resembles a version string such as
// "v1.15.0-dee3e26" or "1.2.3".
func looksLikeVersion(tok string) bool {
	if len(tok) >= 2 && (tok[0] == 'v' || tok[0] == 'V') && isDigit(tok[1]) {
		return true
	}
	return isDigit(tok[0]) && strings.Contains(tok, ".")
}

// looksLikeDate reports whether tok resembles a date such as "19-Apr-2026".
func looksLikeDate(tok string) bool {
	return strings.Contains(tok, "-") && strings.ContainsFunc(tok, isLetterRune)
}

func isDigit(c byte) bool      { return c >= '0' && c <= '9' }
func isLetterRune(r rune) bool { return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') }

func indexByte(b []byte, c byte) int {
	for i := range b {
		if b[i] == c {
			return i
		}
	}
	return -1
}
