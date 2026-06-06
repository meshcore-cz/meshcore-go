// Package transport defines the packet-oriented interfaces every meshcore-go
// connection is built on. A transport moves complete logical companion-protocol
// packets and hides its own low-level framing behind the PacketConn interface.
package transport

import "context"

// PacketConn is the minimal packet-oriented interface implemented by every
// transport. The SDK client works only with complete logical packets;
// transport-specific framing remains hidden behind each adapter.
type PacketConn interface {
	// Open establishes the underlying connection.
	Open(ctx context.Context) error
	// Close releases the underlying connection.
	Close() error

	// ReadPacket blocks until one complete logical packet is available,
	// the context is cancelled, or the connection fails.
	ReadPacket(ctx context.Context) ([]byte, error)
	// WritePacket transmits one complete logical packet.
	WritePacket(ctx context.Context, packet []byte) error

	// String returns a human-readable endpoint description, typically the URI.
	String() string
}

// Endpoint describes a discovered companion radio endpoint. Discovery and
// protocol verification are separate concerns: an Endpoint only records where
// a device might be reached, not that it is a verified MeshCore radio.
type Endpoint struct {
	URI       string            // e.g. serial:///dev/ttyACM0
	Transport string            // e.g. "serial", "ble"
	Name      string            // best-effort human label
	Address   string            // raw transport address
	Metadata  map[string]string // transport-specific extras
}

// Discoverer enumerates endpoints for a single transport. Custom discovery
// mechanisms can be plugged in by implementing this interface.
type Discoverer interface {
	Discover(ctx context.Context) ([]Endpoint, error)
}
