package meshcore

import (
	"time"

	"github.com/meshcore-dev/meshcore-go/protocol"
	"github.com/meshcore-dev/meshcore-go/protocol/companion"
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

// RawEvent carries a push notification the SDK did not decode into a typed
// event, so applications can still inspect unknown firmware behaviour.
type RawEvent struct {
	Type    byte
	Payload []byte
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

func (MessageReceived) isMeshCoreEvent()       {}
func (MessageAcknowledged) isMeshCoreEvent()   {}
func (AdvertisementReceived) isMeshCoreEvent() {}
func (TelemetryReceived) isMeshCoreEvent()     {}
func (Disconnected) isMeshCoreEvent()          {}
func (RawEvent) isMeshCoreEvent()              {}

// Telemetry is a placeholder for decoded telemetry payloads (Phase 5).
type Telemetry struct {
	Fields map[string]any
}

// translate maps a decoded protocol push message to a typed SDK event. It
// returns nil for pushes that carry no user-facing event of their own.
func translate(msg protocol.Message) Event {
	switch m := msg.(type) {
	case companion.Advert:
		return AdvertisementReceived{Contact: Contact{Name: m.Name, PublicKey: m.PublicKey}}
	case companion.SendConfirmed:
		return MessageAcknowledged{Code: hex32(m.Code), RTT: m.RoundTrip}
	case companion.MsgWaiting:
		// Surfaced via the sync loop as MessageReceived, not on its own.
		return nil
	case protocol.RawMessage:
		return RawEvent{Type: m.Type, Payload: m.Payload}
	default:
		return nil
	}
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
