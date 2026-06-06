package companion

import (
	"encoding/binary"
	"fmt"
	"time"

	"github.com/meshcore-dev/meshcore-go/protocol"
)

// appName is the identifier the client reports to the device during the
// APP_START handshake. It is padded/truncated to fit the reserved field.
const appName = "mcr"

// AppStart begins a companion session and elicits a SelfInfo response.
type AppStart struct {
	// Name identifies the connecting application (default "mcr").
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

	case SendChannelTextMessage:
		// [cmd][txt_type][channel_idx][timestamp(4 LE)][text]
		buf := make([]byte, 0, 7+len(c.Text))
		buf = append(buf, cmdSendChannelTxt, c.TxtType, c.Channel)
		buf = appendTimestamp(buf, c.Timestamp)
		buf = append(buf, []byte(c.Text)...)
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
