// Package testutil provides deterministic helpers for testing the SDK without
// physical hardware.
package testutil

import (
	"context"
	"errors"
	"sync"

	"github.com/meshcore-cz/meshcore-go/transport"
)

// ErrClosed is returned by a closed FakeTransport.
var ErrClosed = errors.New("testutil: transport closed")

// FakeTransport is an in-memory transport.PacketConn. Tests push packets the
// client should read onto ReadPackets and inspect what the client wrote on
// WrittenPackets.
type FakeTransport struct {
	ReadPackets    chan []byte
	WrittenPackets chan []byte

	mu     sync.Mutex
	closed chan struct{}
	once   sync.Once
}

// NewFakeTransport returns a FakeTransport with buffered channels.
func NewFakeTransport(buffer int) *FakeTransport {
	if buffer < 1 {
		buffer = 1
	}
	return &FakeTransport{
		ReadPackets:    make(chan []byte, buffer),
		WrittenPackets: make(chan []byte, buffer),
		closed:         make(chan struct{}),
	}
}

// Open implements transport.PacketConn.
func (f *FakeTransport) Open(ctx context.Context) error { return nil }

// Close implements transport.PacketConn.
func (f *FakeTransport) Close() error {
	f.once.Do(func() { close(f.closed) })
	return nil
}

// ReadPacket implements transport.PacketConn.
func (f *FakeTransport) ReadPacket(ctx context.Context) ([]byte, error) {
	select {
	case p := <-f.ReadPackets:
		return p, nil
	case <-f.closed:
		return nil, ErrClosed
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// WritePacket implements transport.PacketConn.
func (f *FakeTransport) WritePacket(ctx context.Context, packet []byte) error {
	cp := make([]byte, len(packet))
	copy(cp, packet)
	select {
	case f.WrittenPackets <- cp:
		return nil
	case <-f.closed:
		return ErrClosed
	case <-ctx.Done():
		return ctx.Err()
	}
}

// String implements transport.PacketConn.
func (f *FakeTransport) String() string { return "fake://memory" }

var _ transport.PacketConn = (*FakeTransport)(nil)
