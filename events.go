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

// MessageReceived is emitted when a direct text message arrives.
type MessageReceived struct {
	From      Contact
	Text      string
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

// translate maps a decoded protocol push message to a typed SDK event.
func translate(msg protocol.Message) Event {
	switch m := msg.(type) {
	case companion.Advert:
		return AdvertisementReceived{Contact: Contact{Name: m.Name, PublicKey: m.PublicKey}}
	case companion.SendConfirmed:
		return MessageAcknowledged{Code: hex32(m.Code), RTT: m.RoundTrip}
	case protocol.RawMessage:
		return RawEvent{Type: m.Type, Payload: m.Payload}
	default:
		return RawEvent{Type: 0, Payload: nil}
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
