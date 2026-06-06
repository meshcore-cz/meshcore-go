// Package meshcore is a transport-independent Go SDK for communicating with
// MeshCore-compatible companion radios. Applications use the high-level Client
// regardless of how a radio is reached (serial, BLE and, later, TCP/WebSocket).
package meshcore

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/meshcore-cz/meshcore-go/internal/dispatcher"
	"github.com/meshcore-cz/meshcore-go/internal/queue"
	"github.com/meshcore-cz/meshcore-go/protocol"
	"github.com/meshcore-cz/meshcore-go/protocol/companion"
	"github.com/meshcore-cz/meshcore-go/transport"
)

// DefaultTimeout bounds how long a single request waits for its response.
const DefaultTimeout = 10 * time.Second

// DefaultHandshakeTimeout bounds the protocol handshake when the caller's
// context has no deadline of its own.
const DefaultHandshakeTimeout = 20 * time.Second

// Client is the high-level entry point for talking to a companion radio. It
// owns request sequencing, response matching and the asynchronous event
// stream so applications never manage command timing manually.
type Client struct {
	conn  transport.PacketConn
	proto protocol.Protocol

	queue  *queue.Queue
	events *dispatcher.Dispatcher[Event]
	raw    *dispatcher.Dispatcher[RawPacket]

	timeout time.Duration
	log     *slog.Logger

	acks        chan uint32                     // SendConfirmed ack codes, for WaitForAcknowledgement
	traces      chan companion.TraceData        // TraceData pushes, for Trace
	discoveries chan companion.NodeDiscoverResp // NodeDiscoverResp pushes, for Discover
	autoSync  bool                     // drain inbound messages on MSG_WAITING
	syncReq   chan struct{}            // signals the sync loop
	eventHook func(Event)              // optional observer called before Events emits

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

// WithLogger enables debug logging through the given slog.Logger. By default
// the client logs nothing.
func WithLogger(l *slog.Logger) Option {
	return func(c *Client) {
		if l != nil {
			c.log = l
		}
	}
}

// WithMessageSync makes the client automatically drain inbound messages when
// the device signals MSG_WAITING, emitting MessageReceived events. Leave it
// off for one-shot commands so messages are not consumed unexpectedly; enable
// it for long-running consumers such as `mc watch`.
func WithMessageSync() Option {
	return func(c *Client) { c.autoSync = true }
}

// WithEventHook observes typed events without consuming the Events channel.
// The hook must return quickly; it runs on the client's read/sync goroutines.
func WithEventHook(hook func(Event)) Option {
	return func(c *Client) { c.eventHook = hook }
}

// New builds a Client over an already-constructed transport. The caller is
// responsible for calling Connect.
func New(conn transport.PacketConn, opts ...Option) *Client {
	c := &Client{
		conn:    conn,
		proto:   companion.New(),
		queue:   queue.New(),
		events:  dispatcher.New[Event](64),
		raw:     dispatcher.New[RawPacket](256),
		timeout: DefaultTimeout,
		log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		acks:        make(chan uint32, 16),
		traces:      make(chan companion.TraceData, 8),
		discoveries: make(chan companion.NodeDiscoverResp, 32),
		syncReq:     make(chan struct{}, 1),
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
	c.log.Debug("opening transport", "endpoint", c.conn.String())
	if err := c.conn.Open(ctx); err != nil {
		c.log.Debug("open failed", "error", err)
		return err
	}

	// Bound the handshake if the caller did not supply a deadline, so a silent
	// or wrongly-framed device cannot hang Connect forever.
	hctx := ctx
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		hctx, cancel = context.WithTimeout(ctx, DefaultHandshakeTimeout)
		defer cancel()
	}

	c.log.Debug("starting handshake")
	session, err := c.proto.Initialize(hctx, c.conn)
	if err != nil {
		c.log.Debug("handshake failed", "error", err)
		_ = c.conn.Close()
		return err
	}
	c.log.Debug("handshake ok",
		"name", session.Name,
		"firmware", session.FirmwareName,
		"version", session.FirmwareVersion,
		"public_key", session.PublicKey)
	c.mu.Lock()
	c.session = session
	c.mu.Unlock()

	// The read loop runs under a context tied to the client lifetime.
	loopCtx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	go c.readLoop(loopCtx)
	if c.autoSync {
		go c.syncLoop(loopCtx)
	}
	return nil
}

// readLoop deframes inbound packets, routing responses to the request queue
// and push notifications to the event stream.
func (c *Client) readLoop(ctx context.Context) {
	for {
		pkt, err := c.conn.ReadPacket(ctx)
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				c.emitEvent(Disconnected{Err: err})
			}
			c.events.Close()
			c.raw.Close()
			return
		}
		msg, err := c.proto.Decode(pkt)
		if err != nil {
			c.raw.Emit(rawPacket(pkt, nil, err))
			c.log.Debug("decode error", "bytes", len(pkt), "error", err)
			continue
		}
		c.raw.Emit(rawPacket(pkt, msg, nil))
		if msg.Async() {
			c.log.Debug("event", "type", msgType(msg))
			c.handleAsync(msg)
			continue
		}
		if !c.queue.Deliver(msg) {
			c.log.Debug("unmatched response", "type", msgType(msg))
		}
	}
}

// handleAsync routes a push notification: acks feed WaitForAcknowledgement,
// MSG_WAITING triggers the optional sync loop, and everything becomes an event.
func (c *Client) handleAsync(msg protocol.Message) {
	switch m := msg.(type) {
	case companion.SendConfirmed:
		select {
		case c.acks <- m.Code:
		default:
		}
	case companion.TraceData:
		select {
		case c.traces <- m:
		default:
		}
	case companion.NodeDiscoverResp:
		select {
		case c.discoveries <- m:
		default:
		}
	case companion.MsgWaiting:
		if c.autoSync {
			select {
			case c.syncReq <- struct{}{}:
			default:
			}
		}
	}
	if ev := translate(msg); ev != nil {
		c.emitEvent(ev)
	}
}

// syncLoop drains buffered messages when signalled, emitting MessageReceived
// events. It runs only when message sync is enabled.
func (c *Client) syncLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.syncReq:
			msgs, err := c.SyncMessages(ctx)
			if err != nil {
				c.log.Debug("sync failed", "error", err)
				continue
			}
			for _, m := range msgs {
				c.emitEvent(messageEvent(m))
			}
		}
	}
}

func (c *Client) emitEvent(ev Event) {
	if c.eventHook != nil {
		c.eventHook(ev)
	}
	c.events.Emit(ev)
}

func hexPreview(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	n := min(16, len(b))
	s := hex.EncodeToString(b[:n])
	if len(b) > n {
		return s + "…"
	}
	return s
}

func msgType(msg protocol.Message) string {
	if raw, ok := msg.(protocol.RawMessage); ok {
		return fmt.Sprintf("raw(0x%02x)", raw.Type)
	}
	return fmt.Sprintf("%T", msg)
}

func rawPacket(pkt []byte, msg protocol.Message, decodeErr error) RawPacket {
	out := RawPacket{
		Timestamp: time.Now(),
		Direction: "in",
		Bytes:     append([]byte(nil), pkt...),
	}
	if len(pkt) > 0 {
		out.Type = pkt[0]
	}
	if msg != nil {
		out.Async = msg.Async()
		out.DecodedType = msgType(msg)
	}
	if decodeErr != nil {
		out.DecodeError = decodeErr.Error()
	}
	return out
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

	cmdName := fmt.Sprintf("%T", cmd)
	cmdHex := ""
	if len(raw) > 0 {
		cmdHex = fmt.Sprintf("0x%02x", raw[0])
	}
	c.log.Debug("radio send", "cmd", cmdHex, "op", cmdName, "bytes", len(raw), "hex", hexPreview(raw))
	v, err := c.queue.Do(tctx, func() error {
		return c.conn.WritePacket(tctx, raw)
	})
	if err != nil {
		c.log.Debug("radio recv", "cmd", cmdHex, "op", cmdName, "error", err)
		return nil, err
	}

	msg := v.(protocol.Message)
	if e, ok := msg.(companion.Err); ok {
		c.log.Debug("radio recv", "cmd", cmdHex, "op", cmdName, "response", "err", "err_code", e.Code)
		return msg, &DeviceError{Code: e.Code}
	}
	c.log.Debug("radio recv", "cmd", cmdHex, "op", cmdName, "response", msgType(msg))
	return msg, nil
}

// requestStream sends a pre-encoded command and feeds each response to collect
// until it reports the stream is complete. A device error frame aborts the
// stream.
func (c *Client) requestStream(ctx context.Context, raw []byte, collect func(protocol.Message) bool) error {
	tctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	c.log.Debug("request stream", "bytes", len(raw))
	var streamErr error
	err := c.queue.DoStream(tctx, func() error {
		return c.conn.WritePacket(tctx, raw)
	}, func(v any) bool {
		msg := v.(protocol.Message)
		if e, ok := msg.(companion.Err); ok {
			streamErr = &DeviceError{Code: e.Code}
			return true
		}
		return collect(msg)
	})
	if err != nil {
		return err
	}
	return streamErr
}

// RawSend transmits payload directly to the device without any companion
// encoding and returns the decoded response. It is intended for protocol
// exploration and debugging; the caller is responsible for framing the bytes
// correctly for the active protocol.
//
// The returned message is whatever the protocol's decoder produces: a known
// typed response or a protocol.RawMessage for unrecognised opcodes.
func (c *Client) RawSend(ctx context.Context, payload []byte) (protocol.Message, error) {
	tctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	c.log.Debug("raw send", "bytes", len(payload))
	v, err := c.queue.Do(tctx, func() error {
		return c.conn.WritePacket(tctx, payload)
	})
	if err != nil {
		return nil, err
	}
	return v.(protocol.Message), nil
}

// Events returns the asynchronous event stream. The channel is closed when the
// connection ends.
func (c *Client) Events() <-chan Event {
	return c.events.Events()
}

// RawPackets returns inbound packets observed by the read loop before response
// matching or event translation.
func (c *Client) RawPackets() <-chan RawPacket {
	return c.raw.Events()
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
