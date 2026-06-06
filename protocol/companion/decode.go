package companion

import (
	"encoding/binary"
	"encoding/hex"
	"strings"
	"time"

	"github.com/meshcore-dev/meshcore-go/protocol"
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

	case pushAdvert, pushNewAdvert:
		return decodeAdvert(body), nil

	case pushSendConfirmed:
		return decodeSendConfirmed(body), nil

	case pushMsgWaiting:
		return MsgWaiting{}, nil
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
