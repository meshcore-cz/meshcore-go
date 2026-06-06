package companion

import "time"

// OK is RESP_CODE_OK, a bare positive acknowledgement.
type OK struct{}

func (OK) Async() bool { return false }

// Err is RESP_CODE_ERR. Code carries an optional device error code.
type Err struct {
	Code byte
}

func (Err) Async() bool { return false }

// SelfInfo is RESP_CODE_SELF_INFO, returned in reply to AppStart. It carries
// the device identity and radio configuration.
type SelfInfo struct {
	AdvType    byte
	TxPower    byte
	MaxTxPower byte
	PublicKey  string // hex-encoded 32-byte key
	AdvLat     float64
	AdvLon     float64
	RadioFreq  uint32 // kHz
	RadioBW    uint32 // kHz
	RadioSF    byte
	RadioCR    byte
	Name       string
}

func (SelfInfo) Async() bool { return false }

// DeviceInfo is RESP_CODE_DEVICE_INFO, returned in reply to DeviceQuery.
type DeviceInfo struct {
	FirmwareCode byte   // firmware/protocol version byte
	Model        string // hardware model, e.g. "Heltec V3"
	BuildDate    string // firmware build date, e.g. "19-Apr-2026"
	Version      string // firmware version, e.g. "v1.15.0-dee3e26"
}

func (DeviceInfo) Async() bool { return false }

// CurrentTime is RESP_CODE_CURR_TIME.
type CurrentTime struct {
	Time time.Time
}

func (CurrentTime) Async() bool { return false }

// BatteryVoltage is RESP_CODE_BATTERY_VOLTAGE.
type BatteryVoltage struct {
	Millivolts uint16
}

func (BatteryVoltage) Async() bool { return false }

// Advert is a PUSH_CODE_ADVERT/NEW_ADVERT push notification.
type Advert struct {
	PublicKey string
	Name      string
}

func (Advert) Async() bool { return true }

// SendConfirmed is a PUSH_CODE_SEND_CONFIRMED push (a message acknowledgement).
type SendConfirmed struct {
	// Code is the acknowledgement code echoed by the recipient.
	Code uint32
	// RoundTrip is the measured round-trip time, when reported.
	RoundTrip time.Duration
}

func (SendConfirmed) Async() bool { return true }

// MsgWaiting is a PUSH_CODE_MSG_WAITING push: the device has buffered messages
// the client can drain with SyncNextMessage.
type MsgWaiting struct{}

func (MsgWaiting) Async() bool { return true }
