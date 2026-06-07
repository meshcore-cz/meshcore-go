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

// StatsResponse is RESP_CODE_STATS, returned for local CMD_GET_STATS requests.
type StatsResponse struct {
	Type    byte
	Core    *StatsCore
	Radio   *StatsRadio
	Packets *StatsPackets
}

func (StatsResponse) Async() bool { return false }

// StatsCore carries local core counters.
type StatsCore struct {
	BatteryMV  uint16
	UptimeSecs uint32
	ErrorFlags uint16
	QueueLen   byte
}

// StatsRadio carries local radio measurements and airtime counters.
type StatsRadio struct {
	NoiseFloor int16
	LastRSSI   int
	LastSNR    float64
	TxAirSecs  uint32
	RxAirSecs  uint32
}

// StatsPackets carries local packet counters.
type StatsPackets struct {
	Received   uint32
	Sent       uint32
	FloodTx    uint32
	DirectTx   uint32
	FloodRx    uint32
	DirectRx   uint32
	RecvErrors uint32
}

// Advert is a PUSH_CODE_ADVERT/NEW_ADVERT push notification. NEW_ADVERT uses
// the full contact frame and may populate the optional fields below.
type Advert struct {
	PublicKey  string
	Name       string
	Type       byte
	HasPath    bool
	OutPathEnc byte
	OutPath    []byte
	Latitude   float64
	Longitude  float64
	LastAdvert time.Time
	LastMod    uint32
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
	OutPathEnc byte   // encoded path metadata; 0xff = flood / unknown
	OutPath    []byte // up to 64 raw path bytes
	Name       string
	LastAdvert time.Time
	Latitude   float64
	Longitude  float64
	LastMod    time.Time
}

func (Contact) Async() bool { return false }

// EndOfContacts is RESP_CODE_END_OF_CONTACTS, terminating a contact list.
type EndOfContacts struct {
	MostRecentLastMod uint32
}

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

// ControlData is a PUSH_CODE_CONTROL_DATA push carrying a binary control-plane
// reply the SDK does not decode into a more specific type. SNR/RSSI describe the
// link the reply arrived on. (Firmware-derived layout, not hardware-verified.)
type ControlData struct {
	SNR         float64
	RSSI        int
	PathLen     byte
	PayloadType byte
	Payload     []byte
}

func (ControlData) Async() bool { return true }

// NodeDiscoverResp is a PUSH_CODE_CONTROL_DATA push answering a node-discovery
// request: one record per node that heard the request and replied.
//
// SNRDown is how the local radio heard the reply; SNRUp is how the remote node
// heard our request. (Firmware-derived layout, not hardware-verified.)
type NodeDiscoverResp struct {
	Tag       uint32
	NodeType  byte    // 1=chat, 2=repeater, 3=room, 4=sensor
	PublicKey string  // hex; 8-byte prefix or full 32-byte key
	SNRUp     float64 // dB, remote-heard
	SNRDown   float64 // dB, locally-heard
	RSSI      int     // dBm
	PathLen   byte
}

func (NodeDiscoverResp) Async() bool { return true }

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
