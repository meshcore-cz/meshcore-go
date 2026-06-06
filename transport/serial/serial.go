// Package serial implements the USB CDC-ACM transport for MeshCore companion
// radios using the companion "serial V3" framing.
package serial

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net/url"
	"sync"

	bugserial "go.bug.st/serial"

	"github.com/meshcore-cz/meshcore-go/transport"
)

// DefaultBaud is the default serial line speed for companion radios.
const DefaultBaud = 115200

// ErrBusy is returned by Open when the serial port is already in use by another
// program. Callers can test for it with errors.Is.
var ErrBusy = errors.New("port is busy (already in use by another program)")

// Option configures a serial connection.
type Option func(*Conn)

// WithBaud sets the line speed (default 115200).
func WithBaud(baud int) Option {
	return func(c *Conn) { c.baud = baud }
}

// Conn is a serial transport.PacketConn. A background reader continuously
// deframes inbound packets so reads honour context cancellation and Close
// promptly unblocks pending reads.
type Conn struct {
	path string
	baud int

	mu   sync.Mutex
	port bugserial.Port

	packets chan []byte
	readErr chan error
	done    chan struct{}
	once    sync.Once
}

// New returns an unopened serial connection for the given device path.
func New(path string, opts ...Option) *Conn {
	c := &Conn{
		path:    path,
		baud:    DefaultBaud,
		packets: make(chan []byte, 16),
		readErr: make(chan error, 1),
		done:    make(chan struct{}),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

var _ transport.PacketConn = (*Conn)(nil)

// Open opens the serial port and starts the background reader.
func (c *Conn) Open(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.port != nil {
		return nil
	}

	port, err := bugserial.Open(c.path, &bugserial.Mode{
		BaudRate: c.baud,
		DataBits: 8,
		Parity:   bugserial.NoParity,
		StopBits: bugserial.OneStopBit,
	})
	if err != nil {
		if isBusy(err) {
			return fmt.Errorf("serial %s: %w", c.path, ErrBusy)
		}
		return fmt.Errorf("serial: open %s: %w", c.path, err)
	}
	c.port = port

	go c.readLoop(bufio.NewReader(port))
	return nil
}

func (c *Conn) readLoop(r *bufio.Reader) {
	for {
		frame, err := readFrame(r)
		if err != nil {
			select {
			case c.readErr <- err:
			default:
			}
			return
		}
		select {
		case c.packets <- frame:
		case <-c.done:
			return
		}
	}
}

// ReadPacket returns the next deframed packet.
func (c *Conn) ReadPacket(ctx context.Context) ([]byte, error) {
	select {
	case p := <-c.packets:
		return p, nil
	case err := <-c.readErr:
		return nil, err
	case <-c.done:
		return nil, fmt.Errorf("serial: connection closed")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// WritePacket frames and writes a packet to the device.
func (c *Conn) WritePacket(ctx context.Context, packet []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.port == nil {
		return fmt.Errorf("serial: connection not open")
	}
	return writeFrame(c.port, packet)
}

// Close closes the serial port and stops the reader.
func (c *Conn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.once.Do(func() { close(c.done) })
	if c.port == nil {
		return nil
	}
	err := c.port.Close()
	c.port = nil
	return err
}

// String returns the endpoint URI.
func (c *Conn) String() string {
	return "serial://" + c.path
}

// isBusy reports whether err is the library's "port busy" condition.
func isBusy(err error) bool {
	var pe *bugserial.PortError
	if errors.As(err, &pe) {
		return pe.Code() == bugserial.PortBusy
	}
	return false
}

// Dialer dials serial:// URIs for the transport registry.
type Dialer struct{}

// NewDialer returns a serial Dialer.
func NewDialer() *Dialer { return &Dialer{} }

// Dial implements transport.Dialer. It honours an optional ?baud= query.
func (Dialer) Dial(ctx context.Context, uri *url.URL) (transport.PacketConn, error) {
	path := uri.Path
	if path == "" {
		path = uri.Opaque
	}
	if path == "" {
		return nil, fmt.Errorf("serial: URI %q has no device path", uri)
	}

	opts := []Option{}
	if b := uri.Query().Get("baud"); b != "" {
		var baud int
		if _, err := fmt.Sscanf(b, "%d", &baud); err == nil && baud > 0 {
			opts = append(opts, WithBaud(baud))
		}
	}
	return New(path, opts...), nil
}
