package protocol

import "sync"

// ExtensionDecoder lets a firmware extension claim packets the base protocol
// does not recognise. It is consulted before a packet degrades to a
// RawMessage. (Used from Phase 6 onward; the type is defined here so the base
// protocol can wire it in without churn.)
type ExtensionDecoder interface {
	// DecodeExtension attempts to decode an otherwise-unknown packet. The
	// boolean reports whether the packet was claimed.
	DecodeExtension(packet []byte) (Message, bool)
}

// ExtensionRegistry holds the extension decoders registered for a client.
type ExtensionRegistry struct {
	mu       sync.RWMutex
	decoders []ExtensionDecoder
}

// Register adds an extension decoder.
func (r *ExtensionRegistry) Register(d ExtensionDecoder) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.decoders = append(r.decoders, d)
}

// Decode offers the packet to each registered decoder in turn.
func (r *ExtensionRegistry) Decode(packet []byte) (Message, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, d := range r.decoders {
		if msg, ok := d.DecodeExtension(packet); ok {
			return msg, true
		}
	}
	return nil, false
}
