package meshcore

import (
	"encoding/hex"
	"fmt"
	"time"

	"github.com/meshcore-cz/meshcore-go/protocol"
	"github.com/meshcore-cz/meshcore-go/protocol/companion"
)

// Event is the common interface for asynchronous notifications emitted by a
// companion radio independently of any request.
type Event interface {
	isMeshCoreEvent()
}

// MessageReceived is emitted when a text message arrives (direct or channel).
type MessageReceived struct {
	From      Contact
	Channel   string // non-empty for channel messages
	Text      string
	TxtType   byte
	Timestamp time.Time
}

// MessageAcknowledged is emitted when a sent message is confirmed.
type MessageAcknowledged struct {
	Code string
	RTT  time.Duration
}

// MessagesWaiting is emitted when the device signals that messages are buffered
// and ready to be drained (companion MSG_WAITING). The consumer responsible for
// draining the inbox should call DrainMessages in response.
type MessagesWaiting struct{}

// AdvertisementReceived is emitted when a contact advertisement is heard.
type AdvertisementReceived struct {
	Contact Contact
}

// TelemetryReceived is emitted when telemetry data arrives.
type TelemetryReceived struct {
	From Contact
	Data Telemetry
}

// Disconnected is emitted once when the connection ends.
type Disconnected struct {
	Err error
}

// RepeaterLoginSucceeded is emitted when a remote repeater accepts a login.
type RepeaterLoginSucceeded struct {
	PublicKeyPrefix string // hex-encoded 6-byte key prefix
	Permissions     byte
	Tag             int32 // server timestamp on newer firmware
}

// RepeaterLoginFailed is emitted when a remote repeater rejects or times out a
// login.
type RepeaterLoginFailed struct {
	PublicKeyPrefix string // hex-encoded 6-byte key prefix
}

// RepeaterStatusReceived is emitted when a repeater returns a status response.
type RepeaterStatusReceived struct {
	PublicKeyPrefix string
	Stats           *RepeaterStats
	Text            string
}

// RawEvent carries a push notification the SDK did not decode into a typed
// event, so applications can still inspect unknown firmware behaviour.
type RawEvent struct {
	Type    byte
	Payload []byte
}

// RFPacketReceived is emitted for companion PACKET_LOG_DATA / RF log frames
// (0x88). Bytes contains only the raw over-the-air MeshCore packet, excluding
// the companion frame code and signal metadata.
type RFPacketReceived struct {
	Timestamp time.Time `json:"timestamp"`
	SNR       float64   `json:"snr"`
	RSSI      int       `json:"rssi"`
	Bytes     []byte    `json:"bytes"`
}

// RF packet directions for RFPacket.Direction.
const (
	RFRx = "rx" // received over the air (carries SNR/RSSI)
	RFTx = "tx" // transmitted by us (carries the send Priority)
)

// RFPacket is one over-the-air MeshCore packet seen by the local radio. It forms
// the SDK's unified RF log: received packets (Direction RFRx, from the companion
// 0x88 RF log, with the radio's measured SNR/RSSI) and packets we transmit
// (Direction RFTx, from SendMeshPacket, with the send Priority). Bytes holds the
// raw over-the-air packet. Subscribe via Client.SubscribeRFPackets.
//
// Only host-originated raw transmissions (SendMeshPacket) appear as RFTx; the
// firmware builds and encrypts higher-level sends itself and does not report
// their on-air bytes.
type RFPacket struct {
	Timestamp time.Time `json:"timestamp"`
	Direction string    `json:"direction"` // RFRx | RFTx
	SNR       float64   `json:"snr,omitempty"`
	RSSI      int       `json:"rssi,omitempty"`
	Priority  byte      `json:"priority,omitempty"`
	Bytes     []byte    `json:"bytes"`
}

// RawPacket is an inbound companion-protocol packet observed before the client
// routes it as a response or asynchronous event.
type RawPacket struct {
	Timestamp   time.Time `json:"timestamp"`
	Direction   string    `json:"direction"`
	Bytes       []byte    `json:"bytes"`
	Type        byte      `json:"type"`
	Async       bool      `json:"async"`
	DecodedType string    `json:"decoded_type,omitempty"`
	DecodeError string    `json:"decode_error,omitempty"`
}

func (MessageReceived) isMeshCoreEvent()        {}
func (MessageAcknowledged) isMeshCoreEvent()    {}
func (MessagesWaiting) isMeshCoreEvent()        {}
func (AdvertisementReceived) isMeshCoreEvent()  {}
func (TelemetryReceived) isMeshCoreEvent()      {}
func (Disconnected) isMeshCoreEvent()           {}
func (RepeaterLoginSucceeded) isMeshCoreEvent() {}
func (RepeaterLoginFailed) isMeshCoreEvent()    {}
func (RepeaterStatusReceived) isMeshCoreEvent() {}
func (RawEvent) isMeshCoreEvent()               {}
func (RFPacketReceived) isMeshCoreEvent()       {}

// Telemetry is a placeholder for decoded telemetry payloads (Phase 5).
type Telemetry struct {
	Fields map[string]any
}

// translate maps a decoded protocol push message to a typed SDK event. It
// returns nil for pushes that carry no user-facing event of their own.
func translate(msg protocol.Message) Event {
	switch m := msg.(type) {
	case companion.Advert:
		return AdvertisementReceived{Contact: contactFromAdvert(m)}
	case companion.SendConfirmed:
		return MessageAcknowledged{Code: hex32(m.Code), RTT: m.RoundTrip}
	case companion.MsgWaiting:
		// Signal that the inbox should be drained; the actual messages are
		// surfaced as MessageReceived by whoever drains via DrainMessages.
		return MessagesWaiting{}
	case companion.LoginSuccess:
		prefix := ""
		if len(m.PublicKeyPrefix) > 0 {
			prefix = hex.EncodeToString(m.PublicKeyPrefix)
		}
		return RepeaterLoginSucceeded{PublicKeyPrefix: prefix, Permissions: m.Permissions, Tag: m.Tag}
	case companion.LoginFail:
		prefix := ""
		if len(m.PublicKeyPrefix) > 0 {
			prefix = hex.EncodeToString(m.PublicKeyPrefix)
		}
		return RepeaterLoginFailed{PublicKeyPrefix: prefix}
	case companion.StatusResponse:
		prefix := ""
		if len(m.PublicKeyPrefix) > 0 {
			prefix = hex.EncodeToString(m.PublicKeyPrefix)
		}
		var stats *RepeaterStats
		if m.Stats != nil {
			s := *m.Stats
			stats = &s
		}
		return RepeaterStatusReceived{PublicKeyPrefix: prefix, Stats: stats, Text: m.Text}
	case protocol.RawMessage:
		if m.Type == 0x88 && m.Push {
			ev, err := decodeRFPacketLogPayload(m.Payload, time.Now())
			if err == nil {
				return ev
			}
		}
		return RawEvent{Type: m.Type, Payload: m.Payload}
	default:
		return nil
	}
}

// DecodeRFPacketReceived decodes a full companion PACKET_LOG_DATA frame:
// byte 0 = 0x88, byte 1 = int8(SNR*4), byte 2 = int8(RSSI), bytes 3.. = raw
// over-the-air MeshCore packet. This matches MeshCore companion
// PUSH_CODE_LOG_RX_DATA as implemented by logRxRaw().
func DecodeRFPacketReceived(frame []byte, timestamp time.Time) (RFPacketReceived, error) {
	if len(frame) == 0 {
		return RFPacketReceived{}, fmt.Errorf("rf packet log: truncated frame: missing packet type")
	}
	if frame[0] != 0x88 {
		return RFPacketReceived{}, fmt.Errorf("rf packet log: unexpected packet type 0x%02x", frame[0])
	}
	return decodeRFPacketLogPayload(frame[1:], timestamp)
}

func decodeRFPacketLogPayload(payload []byte, timestamp time.Time) (RFPacketReceived, error) {
	if len(payload) < 2 {
		return RFPacketReceived{}, fmt.Errorf("rf packet log: truncated 0x88 frame: need SNR, RSSI and RF bytes, got %d payload byte(s)", len(payload))
	}
	return RFPacketReceived{
		Timestamp: timestamp,
		SNR:       float64(int8(payload[0])) / 4.0,
		RSSI:      int(int8(payload[1])),
		Bytes:     cloneBytes(payload[2:]),
	}, nil
}

// messageEvent maps a drained Message to a MessageReceived event.
func messageEvent(m Message) Event {
	return MessageReceived{
		From:      Contact{Name: m.From},
		Text:      m.Text,
		TxtType:   m.TxtType,
		Timestamp: m.Timestamp,
		Channel:   m.Channel,
	}
}

func hex32(v uint32) string {
	const digits = "0123456789abcdef"
	var b [8]byte
	for i := 7; i >= 0; i-- {
		b[i] = digits[v&0xf]
		v >>= 4
	}
	return string(b[:])
}

func contactFromAdvert(m companion.Advert) Contact {
	ct := Contact{
		Name:       m.Name,
		PublicKey:  m.PublicKey,
		Type:       contactType(m.Type),
		HasPath:    m.HasPath,
		OutPathEnc: m.OutPathEnc,
		OutPath:    cloneBytes(m.OutPath),
		Latitude:   m.Latitude,
		Longitude:  m.Longitude,
		LastAdvert: m.LastAdvert,
		LastMod:    m.LastMod,
	}
	return ct
}
