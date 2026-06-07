package companion

import (
	"encoding/binary"
	"fmt"
	"time"

	"github.com/meshcore-cz/meshcore-go/protocol"
)

// appName is the identifier the client reports to the device during the
// APP_START handshake. It is padded/truncated to fit the reserved field.
const appName = "mc"

// AppStart begins a companion session and elicits a SelfInfo response.
type AppStart struct {
	// Name identifies the connecting application (default "mc").
	Name string
}

// DeviceQuery requests firmware/build details (RESP_CODE_DEVICE_INFO).
type DeviceQuery struct{}

// GetDeviceTime requests the device's current clock (RESP_CODE_CURR_TIME).
type GetDeviceTime struct{}

// SetDeviceTime sets the device clock to the given time.
type SetDeviceTime struct {
	Time time.Time
}

// GetBatteryVoltage requests the battery voltage (RESP_CODE_BATTERY_VOLTAGE).
type GetBatteryVoltage struct{}

// GetStats requests one local statistics block (RESP_CODE_STATS).
type GetStats struct {
	Type byte
}

// SendSelfAdvert broadcasts the device's own advertisement.
type SendSelfAdvert struct {
	// Flood requests a flood advertisement rather than a zero-hop one.
	Flood bool
}

// Reboot asks the device to restart.
type Reboot struct{}

// GetContacts requests the full contact list (CONTACTS_START, CONTACT…,
// END_OF_CONTACTS).
type GetContacts struct{}

// GetChannel requests information about a channel slot by index.
type GetChannel struct {
	Index byte
}

// SyncNextMessage drains the next buffered inbound message, if any.
type SyncNextMessage struct{}

// SendTextMessage sends a direct text message to a contact.
//
// Wire layout is firmware-derived and not yet hardware-verified — sending
// transmits over LoRa to a live mesh, so it was not test-fired during
// development. Cross-check against firmware before relying on it.
type SendTextMessage struct {
	DestPublicKey []byte // recipient public key (at least the 6-byte prefix)
	Text          string
	Timestamp     time.Time // defaults to now when zero
	TxtType       byte      // 0 = plain text
	Attempt       byte
}

// SendLogin logs in to a remote repeater/sensor.
type SendLogin struct {
	PublicKey []byte
	Password  string
}

// SendStatusReq requests status from a remote repeater or sensor.
type SendStatusReq struct {
	PublicKey []byte
}

// HasConnection asks whether the radio still has an active session to a repeater.
type HasConnection struct {
	PublicKey []byte
}

// Logout logs out from a remote repeater/sensor.
type Logout struct {
	PublicKey []byte
}

// SendChannelTextMessage sends a text message to a channel slot.
//
// Wire layout is firmware-derived and not yet hardware-verified (see
// SendTextMessage).
type SendChannelTextMessage struct {
	Channel   byte
	Text      string
	Timestamp time.Time
	TxtType   byte
}

// SendTracePath initiates a path trace. The device replies immediately with a
// Sent response and later emits a TraceData push tagged with Tag.
//
// Path lists repeater hashes to trace through. All hops must share the same
// hash width (1, 2, 4, or 8 bytes). Flags lower two bits select the width
// (hash_size = 1 << bits). An empty path floods the trace.
type SendTracePath struct {
	Tag   uint32
	Auth  uint32
	Flags byte
	Path  []byte // optional repeater hashes to trace through; empty = flood
}

// SendNodeDiscoverReq broadcasts a node-discovery control packet. The device
// replies immediately with OK and then emits a ControlData push per node that
// answers, each tagged with Tag.
//
// Filter is a node-type bitmask (bit = 1<<node_type): repeater=4, companion=2,
// room=8, sensor=16; 0xFF requests all types. Wire format derived from the
// meshcore_py reference (send_node_discover_req); not yet hardware-verified.
type SendNodeDiscoverReq struct {
	Filter     byte
	PrefixOnly bool // request 8-byte key prefixes instead of full public keys
	Tag        uint32
}

// encode serialises a command to its wire payload (without transport framing).
func encode(cmd protocol.Command) ([]byte, error) {
	switch c := cmd.(type) {
	case AppStart:
		name := c.Name
		if name == "" {
			name = appName
		}
		// [cmd][app_target=3][6 reserved bytes][name...]
		buf := make([]byte, 0, 8+len(name))
		buf = append(buf, cmdAppStart, 3)
		buf = append(buf, make([]byte, 6)...)
		buf = append(buf, []byte(name)...)
		return buf, nil

	case DeviceQuery:
		// [cmd][app_ver]
		return []byte{cmdDeviceQuery, 3}, nil

	case GetDeviceTime:
		return []byte{cmdGetDeviceTime}, nil

	case SetDeviceTime:
		buf := make([]byte, 5)
		buf[0] = cmdSetDeviceTime
		binary.LittleEndian.PutUint32(buf[1:], uint32(c.Time.Unix()))
		return buf, nil

	case GetBatteryVoltage:
		return []byte{cmdGetBattery}, nil

	case GetStats:
		return []byte{cmdGetStats, c.Type}, nil

	case SendSelfAdvert:
		typ := byte(0)
		if c.Flood {
			typ = 1
		}
		return []byte{cmdSendSelfAdvert, typ}, nil

	case Reboot:
		return []byte{cmdReboot}, nil

	case GetContacts:
		return []byte{cmdGetContacts}, nil

	case GetChannel:
		return []byte{cmdGetChannel, c.Index}, nil

	case SyncNextMessage:
		return []byte{cmdSyncNextMessage}, nil

	case SendTextMessage:
		if len(c.DestPublicKey) < 6 {
			return nil, fmt.Errorf("companion: recipient key too short (%d bytes)", len(c.DestPublicKey))
		}
		// [cmd][txt_type][attempt][timestamp(4 LE)][dest prefix(6)][text]
		buf := make([]byte, 0, 13+len(c.Text))
		buf = append(buf, cmdSendTxtMsg, c.TxtType, c.Attempt)
		buf = appendTimestamp(buf, c.Timestamp)
		buf = append(buf, c.DestPublicKey[:6]...)
		buf = append(buf, []byte(c.Text)...)
		return buf, nil

	case SendLogin:
		if len(c.PublicKey) < 32 {
			return nil, fmt.Errorf("companion: repeater key too short (%d bytes)", len(c.PublicKey))
		}
		// [cmd][dest public key(32)][password]
		buf := make([]byte, 0, 33+len(c.Password))
		buf = append(buf, cmdSendLogin)
		buf = append(buf, c.PublicKey[:32]...)
		buf = append(buf, []byte(c.Password)...)
		return buf, nil

	case SendStatusReq:
		if len(c.PublicKey) < 32 {
			return nil, fmt.Errorf("companion: repeater key too short (%d bytes)", len(c.PublicKey))
		}
		// [cmd][dest public key(32)]
		buf := make([]byte, 0, 33)
		buf = append(buf, cmdSendStatusReq)
		buf = append(buf, c.PublicKey[:32]...)
		return buf, nil

	case HasConnection:
		if len(c.PublicKey) < 32 {
			return nil, fmt.Errorf("companion: repeater key too short (%d bytes)", len(c.PublicKey))
		}
		// [cmd][dest public key(32)]
		buf := make([]byte, 0, 33)
		buf = append(buf, cmdHasConnection)
		buf = append(buf, c.PublicKey[:32]...)
		return buf, nil

	case Logout:
		if len(c.PublicKey) < 32 {
			return nil, fmt.Errorf("companion: repeater key too short (%d bytes)", len(c.PublicKey))
		}
		buf := make([]byte, 0, 33)
		buf = append(buf, cmdLogout)
		buf = append(buf, c.PublicKey[:32]...)
		return buf, nil

	case SendChannelTextMessage:
		// [cmd][txt_type][channel_idx][timestamp(4 LE)][text]
		buf := make([]byte, 0, 7+len(c.Text))
		buf = append(buf, cmdSendChannelTxt, c.TxtType, c.Channel)
		buf = appendTimestamp(buf, c.Timestamp)
		buf = append(buf, []byte(c.Text)...)
		return buf, nil

	case SendTracePath:
		// [cmd][tag(4 LE)][auth(4 LE)][flags][path…]
		buf := make([]byte, 0, 11+len(c.Path))
		buf = append(buf, cmdSendTracePath)
		buf = binary.LittleEndian.AppendUint32(buf, c.Tag)
		buf = binary.LittleEndian.AppendUint32(buf, c.Auth)
		buf = append(buf, c.Flags)
		buf = append(buf, c.Path...)
		// Firmware rejects frames with len <= 10 when path is empty.
		if len(buf) <= 10 {
			buf = append(buf, 0x00)
		}
		return buf, nil

	case SendNodeDiscoverReq:
		// [cmd][control_type | prefix flag][filter][tag(4 LE)]
		ctrl := controlNodeDiscoverReq
		if c.PrefixOnly {
			ctrl |= 0x01
		}
		buf := make([]byte, 0, 7)
		buf = append(buf, cmdSendControlData, ctrl, c.Filter)
		buf = binary.LittleEndian.AppendUint32(buf, c.Tag)
		return buf, nil

	default:
		return nil, fmt.Errorf("companion: cannot encode command %T", cmd)
	}
}

// appendTimestamp appends a little-endian uint32 Unix time, defaulting to now.
func appendTimestamp(buf []byte, t time.Time) []byte {
	if t.IsZero() {
		t = time.Now()
	}
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], uint32(t.Unix()))
	return append(buf, b[:]...)
}
