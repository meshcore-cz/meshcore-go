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

	default:
		return nil, fmt.Errorf("companion: cannot encode command %T", cmd)
	}
}
