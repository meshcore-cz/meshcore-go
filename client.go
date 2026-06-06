// Package meshcore is a transport-independent Go SDK for communicating with
// MeshCore-compatible companion radios. Applications use the high-level Client
// regardless of how a radio is reached (serial, BLE and, later, TCP/WebSocket).
package meshcore

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/meshcore-dev/meshcore-go/internal/dispatcher"
	"github.com/meshcore-dev/meshcore-go/internal/queue"
	"github.com/meshcore-dev/meshcore-go/protocol"
	"github.com/meshcore-dev/meshcore-go/protocol/companion"
	"github.com/meshcore-dev/meshcore-go/transport"
)

// DefaultTimeout bounds how long a single request waits for its response.
const DefaultTimeout = 10 * time.Second

// Client is the high-level entry point for talking to a companion radio. It
// owns request sequencing, response matching and the asynchronous event
// stream so applications never manage command timing manually.
type Client struct {
	conn  transport.PacketConn
	proto protocol.Protocol

	queue  *queue.Queue
	events *dispatcher.Dispatcher[Event]

	timeout time.Duration

	mu      sync.RWMutex
	session protocol.SessionInfo

	cancel    context.CancelFunc
	closeOnce sync.Once
	closed    chan struct{}
}

// Option configures a Client.
type Option func(*Client)

// WithProtocol overrides the protocol driver (default: companion.New()).
func WithProtocol(p protocol.Protocol) Option {
	return func(c *Client) { c.proto = p }
}

// WithTimeout sets the per-request timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) { c.timeout = d }
}

// New builds a Client over an already-constructed transport. The caller is
// responsible for calling Connect.
func New(conn transport.PacketConn, opts ...Option) *Client {
	c := &Client{
		conn:    conn,
		proto:   companion.New(),
		queue:   queue.New(),
		events:  dispatcher.New[Event](64),
		timeout: DefaultTimeout,
		closed:  make(chan struct{}),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Connect opens the transport, performs the protocol handshake and starts the
// background read loop.
func (c *Client) Connect(ctx context.Context) error {
	if err := c.conn.Open(ctx); err != nil {
		return err
	}

	session, err := c.proto.Initialize(ctx, c.conn)
	if err != nil {
		_ = c.conn.Close()
		return err
	}
	c.mu.Lock()
	c.session = session
	c.mu.Unlock()

	// The read loop runs under a context tied to the client lifetime.
	loopCtx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	go c.readLoop(loopCtx)
	return nil
}

// readLoop deframes inbound packets, routing responses to the request queue
// and push notifications to the event stream.
func (c *Client) readLoop(ctx context.Context) {
	for {
		pkt, err := c.conn.ReadPacket(ctx)
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				c.events.Emit(Disconnected{Err: err})
			}
			c.events.Close()
			return
		}
		msg, err := c.proto.Decode(pkt)
		if err != nil {
			continue
		}
		if msg.Async() {
			c.events.Emit(translate(msg))
			continue
		}
		c.queue.Deliver(msg)
	}
}

// request sends a command and waits for the matching response, bounded by the
// per-request timeout and the supplied context.
func (c *Client) request(ctx context.Context, cmd protocol.Command) (protocol.Message, error) {
	raw, err := c.proto.Encode(cmd)
	if err != nil {
		return nil, err
	}

	tctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	v, err := c.queue.Do(tctx, func() error {
		return c.conn.WritePacket(tctx, raw)
	})
	if err != nil {
		return nil, err
	}

	msg := v.(protocol.Message)
	if e, ok := msg.(companion.Err); ok {
		return msg, &DeviceError{Code: e.Code}
	}
	return msg, nil
}

// Events returns the asynchronous event stream. The channel is closed when the
// connection ends.
func (c *Client) Events() <-chan Event {
	return c.events.Events()
}

// Close shuts down the read loop and the transport.
func (c *Client) Close() error {
	var err error
	c.closeOnce.Do(func() {
		if c.cancel != nil {
			c.cancel()
		}
		close(c.closed)
		err = c.conn.Close()
	})
	return err
}

// Capabilities returns the features negotiated during the handshake.
func (c *Client) Capabilities() Capabilities {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.session.Capabilities
}

// requireCapability returns ErrUnsupportedCapability unless cap is advertised.
func (c *Client) requireCapability(cap Capability) error {
	if !c.Capabilities().Has(cap) {
		return protocol.ErrUnsupportedCapability
	}
	return nil
}

// DeviceError reports a RESP_CODE_ERR returned by the device.
type DeviceError struct {
	Code byte
}

func (e *DeviceError) Error() string {
	return "meshcore: device returned error code " + itoa(int(e.Code))
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [4]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(b[pos:])
}
