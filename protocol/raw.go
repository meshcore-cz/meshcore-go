package protocol

// RawMessage is returned for packets the active protocol does not recognise.
// It lets applications inspect unknown responses and notifications instead of
// failing outright, which keeps the client tolerant of modified firmware.
type RawMessage struct {
	// Type is the leading packet/opcode byte.
	Type byte
	// Payload is the remainder of the packet after the type byte.
	Payload []byte
	// Push reports whether the packet was classified as a push notification.
	Push bool
}

// Async implements Message.
func (m RawMessage) Async() bool { return m.Push }
