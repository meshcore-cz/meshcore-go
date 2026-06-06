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

// ContactsStart is RESP_CODE_CONTACTS_START, sent before a run of Contact
// records. Count is the number of records that follow.
type ContactsStart struct {
	Count uint32
}

func (ContactsStart) Async() bool { return false }

// Contact is RESP_CODE_CONTACT, one entry of the contact list.
type Contact struct {
	PublicKey  string // hex-encoded 32-byte key
	Type       byte
	Flags      byte
	HasPath    bool
	Name       string
	LastAdvert time.Time
	Latitude   float64
	Longitude  float64
	LastMod    time.Time
}

func (Contact) Async() bool { return false }

// EndOfContacts is RESP_CODE_END_OF_CONTACTS, terminating a contact list.
type EndOfContacts struct{}

func (EndOfContacts) Async() bool { return false }

// ChannelInfo is RESP_CODE_CHANNEL_INFO, describing one channel slot.
type ChannelInfo struct {
	Index  byte
	Name   string
	Secret []byte // 16-byte pre-shared key
}

func (ChannelInfo) Async() bool { return false }

// NoMoreMessages is RESP_CODE_NO_MORE_MESSAGES, signalling the inbound buffer
// is drained.
type NoMoreMessages struct{}

func (NoMoreMessages) Async() bool { return false }

// Sent is RESP_CODE_SENT, returned after queuing an outbound message. The
// ExpectedAck code is later echoed by a SendConfirmed push. (Firmware-derived
// layout, not hardware-verified.)
type Sent struct {
	Result           byte
	ExpectedAck      uint32
	SuggestedTimeout time.Duration
}

func (Sent) Async() bool { return false }

// ContactMessage is RESP_CODE_CONTACT_MSG_RECV[_V3], a direct message drained
// from the device buffer. (Firmware-derived layout, not hardware-verified.)
type ContactMessage struct {
	SenderPrefix string // hex-encoded 6-byte sender public-key prefix
	PathLen      byte
	TxtType      byte
	Timestamp    time.Time
	SNR          float64
	Text         string
}

func (ContactMessage) Async() bool { return false }

// ChannelMessage is RESP_CODE_CHANNEL_MSG_RECV[_V3], a channel message drained
// from the device buffer. (Firmware-derived layout, not hardware-verified.)
type ChannelMessage struct {
	Channel   byte
	PathLen   byte
	TxtType   byte
	Timestamp time.Time
	SNR       float64
	Text      string
}

func (ChannelMessage) Async() bool { return false }

// TraceData is a PUSH_CODE_TRACE_DATA push carrying the result of a path trace.
// Verified against MeshCore v1.15 (tracing through a single repeater).
type TraceData struct {
	Tag   uint32
	Auth  uint32
	Flags byte
	Path  []byte    // intermediate node hashes
	SNRs  []float64 // per-link SNR in dB (typically len(Path)+1)
}

func (TraceData) Async() bool { return true }

// LoginSuccess is PUSH_CODE_LOGIN_SUCCESS: a repeater or room server accepted
// the login request.
type LoginSuccess struct {
	Permissions     byte
	PublicKeyPrefix []byte // first 6 bytes of the server's public key
	Tag             int32  // server timestamp on newer firmware
	NewPermissions  byte   // ACL permissions on v7+ firmware
}

func (LoginSuccess) Async() bool { return true }

// LoginFail is PUSH_CODE_LOGIN_FAIL: a repeater or room server rejected or
// timed out the login request.
type LoginFail struct {
	PublicKeyPrefix []byte // first 6 bytes of the server's public key
}

func (LoginFail) Async() bool { return true }

// StatusResponse is PUSH_CODE_STATUS_RESPONSE: binary repeater stats or text
// from a sensor.
type StatusResponse struct {
	PublicKeyPrefix []byte // first 6 bytes of the server's public key
	Stats           *RepeaterStats
	Text            string // plain-text sensor status
}

func (StatusResponse) Async() bool { return true }
