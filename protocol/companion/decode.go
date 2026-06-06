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
// Provisional layout (offsets relative to the body, i.e. after the code byte):
//
//	[0]      adv_type
//	[1]      tx_power
//	[2]      max_tx_power
//	[3:35]   public_key (32 bytes)
//	[35:39]  adv_lat   (int32 LE, units of 1e-6 degrees)
//	[39:43]  adv_lon   (int32 LE, units of 1e-6 degrees)
//	[43:47]  radio_freq (uint32 LE, kHz)
//	[47:51]  radio_bw   (uint32 LE, kHz)
//	[51]     radio_sf
//	[52]     radio_cr
//	[53:]    name (UTF-8, NUL-trimmed)
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
	s.RadioFreq = u32(43)
	s.RadioBW = u32(47)
	s.RadioSF = get(51)
	s.RadioCR = get(52)
	if len(b) > 53 {
		s.Name = trimString(b[53:])
	}
	return s
}

// decodeDeviceInfo parses RESP_CODE_DEVICE_INFO.
//
// Provisional layout: a small fixed header followed by NUL/newline-separated
// strings (model/firmware name and build). Parsing is best-effort.
func decodeDeviceInfo(b []byte) DeviceInfo {
	var d DeviceInfo
	if len(b) > 0 {
		d.FirmwareVersion = b[0]
	}
	if len(b) >= 3 {
		d.MaxContacts = binary.LittleEndian.Uint16(b[1:3])
	}
	if len(b) >= 4 {
		d.MaxChannels = b[3]
	}
	// Remaining bytes: printable strings separated by NUL or newline.
	if len(b) > 4 {
		fields := splitStrings(b[4:])
		if len(fields) > 0 {
			d.FirmwareName = fields[0]
		}
		if len(fields) > 1 {
			d.FirmwareBuild = fields[1]
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

func indexByte(b []byte, c byte) int {
	for i := range b {
		if b[i] == c {
			return i
		}
	}
	return -1
}
