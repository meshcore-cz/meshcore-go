// Package ble implements MeshCore companion BLE transport over Nordic UART
// Service characteristics. BLE carries raw companion packets: unlike USB
// serial, packets are not wrapped in the serial V3 '<'/'>' framing.
package ble

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/meshcore-cz/meshcore-go/transport"
	tinyble "tinygo.org/x/bluetooth"
)

const (
	// MeshCore companion BLE firmware uses Nordic UART Service UUIDs.
	ServiceUUID = "6E400001-B5A3-F393-E0A9-E50E24DCCA9E"
	RXUUID      = "6E400002-B5A3-F393-E0A9-E50E24DCCA9E"
	TXUUID      = "6E400003-B5A3-F393-E0A9-E50E24DCCA9E"

	defaultScanTimeout = 12 * time.Second
	maxPacketLen       = 8192
)

var (
	serviceUUID = mustUUID(ServiceUUID)
	rxUUID      = mustUUID(RXUUID)
	txUUID      = mustUUID(TXUUID)
)

// Option configures a BLE connection.
type Option func(*Conn)

// WithAdapter supplies a non-default adapter, mainly useful for tests or
// systems with multiple Bluetooth controllers.
func WithAdapter(adapter *tinyble.Adapter) Option {
	return func(c *Conn) {
		if adapter != nil {
			c.adapter = adapter
		}
	}
}

// WithScanTimeout bounds address/name discovery before connecting.
func WithScanTimeout(timeout time.Duration) Option {
	return func(c *Conn) {
		if timeout > 0 {
			c.scanTimeout = timeout
		}
	}
}

// Conn is a BLE transport.PacketConn for MeshCore companion radios.
type Conn struct {
	target      string
	scanTimeout time.Duration
	adapter     *tinyble.Adapter

	mu     sync.Mutex
	device tinyble.Device
	rx     tinyble.DeviceCharacteristic
	tx     tinyble.DeviceCharacteristic
	opened bool

	packets chan []byte
	done    chan struct{}
	once    sync.Once
}

// New returns an unopened BLE connection. target may be a platform Bluetooth
// address/UUID, advertised local name, or empty to connect to the first
// advertising MeshCore/NUS device.
func New(target string, opts ...Option) *Conn {
	c := &Conn{
		target:      strings.TrimSpace(target),
		scanTimeout: defaultScanTimeout,
		adapter:     tinyble.DefaultAdapter,
		packets:     make(chan []byte, 32),
		done:        make(chan struct{}),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

var _ transport.PacketConn = (*Conn)(nil)

// Open enables BLE, connects, discovers RX/TX characteristics and subscribes to
// TX notifications.
func (c *Conn) Open(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.opened {
		return nil
	}
	if err := ensureAdapterEnabled(c.adapter); err != nil {
		return fmt.Errorf("ble: enable adapter: %w", err)
	}

	result, err := c.findDevice(ctx)
	if err != nil {
		return err
	}
	device, err := c.adapter.Connect(result.Address, tinyble.ConnectionParams{})
	if err != nil {
		return fmt.Errorf("ble: connect %s: %w", describeScanResult(result), err)
	}
	c.device = device

	services, err := device.DiscoverServices([]tinyble.UUID{serviceUUID})
	if err != nil {
		_ = device.Disconnect()
		return fmt.Errorf("ble: discover MeshCore service: %w", err)
	}
	if len(services) == 0 {
		_ = device.Disconnect()
		return fmt.Errorf("ble: MeshCore service %s not found", ServiceUUID)
	}

	chars, err := services[0].DiscoverCharacteristics([]tinyble.UUID{rxUUID, txUUID})
	if err != nil {
		_ = device.Disconnect()
		return fmt.Errorf("ble: discover RX/TX characteristics: %w", err)
	}
	for _, ch := range chars {
		switch ch.UUID() {
		case rxUUID:
			c.rx = ch
		case txUUID:
			c.tx = ch
		}
	}
	if c.rx.UUID() != rxUUID {
		_ = device.Disconnect()
		return fmt.Errorf("ble: RX characteristic %s not found", RXUUID)
	}
	if c.tx.UUID() != txUUID {
		_ = device.Disconnect()
		return fmt.Errorf("ble: TX characteristic %s not found", TXUUID)
	}

	if err := c.tx.EnableNotifications(c.onNotify); err != nil {
		_ = device.Disconnect()
		return fmt.Errorf("ble: enable TX notifications: %w", err)
	}
	c.opened = true
	return nil
}

// ReadPacket returns the next packet delivered by TX notification.
func (c *Conn) ReadPacket(ctx context.Context) ([]byte, error) {
	select {
	case pkt := <-c.packets:
		return pkt, nil
	case <-c.done:
		return nil, fmt.Errorf("ble: connection closed")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// WritePacket writes one raw companion packet to the RX characteristic.
func (c *Conn) WritePacket(ctx context.Context, packet []byte) error {
	if len(packet) == 0 {
		return nil
	}
	if len(packet) > maxPacketLen {
		return fmt.Errorf("ble: packet too large (%d bytes)", len(packet))
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.opened {
		return fmt.Errorf("ble: connection not open")
	}
	done := make(chan error, 1)
	p := append([]byte(nil), packet...)
	go func() {
		_, err := c.rx.Write(p)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("ble: write RX: %w", err)
		}
		return nil
	case <-c.done:
		return fmt.Errorf("ble: connection closed")
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close disconnects the BLE device.
func (c *Conn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.once.Do(func() { close(c.done) })
	if !c.opened {
		return nil
	}
	_ = c.tx.EnableNotifications(nil)
	err := c.device.Disconnect()
	c.opened = false
	return err
}

// String returns the endpoint URI.
func (c *Conn) String() string {
	if c.target == "" {
		return "ble://"
	}
	return "ble://" + c.target
}

func (c *Conn) onNotify(buf []byte) {
	if len(buf) == 0 || len(buf) > maxPacketLen {
		return
	}
	packet := append([]byte(nil), buf...)
	select {
	case c.packets <- packet:
	case <-c.done:
	default:
		// Drop oldest to keep notifications non-blocking.
		select {
		case <-c.packets:
		default:
		}
		select {
		case c.packets <- packet:
		default:
		}
	}
}

func (c *Conn) findDevice(ctx context.Context) (tinyble.ScanResult, error) {
	scanCtx := ctx
	cancel := func() {}
	if _, ok := scanCtx.Deadline(); !ok {
		scanCtx, cancel = context.WithTimeout(ctx, c.scanTimeout)
	}
	defer cancel()

	found := make(chan tinyble.ScanResult, 1)
	errc := make(chan error, 1)
	target := strings.ToLower(strings.TrimSpace(c.target))

	go func() {
		err := c.adapter.Scan(func(adapter *tinyble.Adapter, result tinyble.ScanResult) {
			if !matchesTarget(result, target) {
				return
			}
			select {
			case found <- result:
				_ = adapter.StopScan()
			default:
			}
		})
		if err != nil {
			errc <- err
		}
	}()

	select {
	case result := <-found:
		return result, nil
	case err := <-errc:
		return tinyble.ScanResult{}, fmt.Errorf("ble: scan: %w", err)
	case <-scanCtx.Done():
		_ = c.adapter.StopScan()
		if c.target == "" {
			return tinyble.ScanResult{}, fmt.Errorf("ble: no MeshCore BLE device found: %w", scanCtx.Err())
		}
		return tinyble.ScanResult{}, fmt.Errorf("ble: device %q not found: %w", c.target, scanCtx.Err())
	}
}

func matchesTarget(result tinyble.ScanResult, target string) bool {
	if target == "" {
		return result.AdvertisementPayload.HasServiceUUID(serviceUUID) || looksMeshCoreName(result.LocalName())
	}
	addr := strings.ToLower(result.Address.String())
	name := strings.ToLower(result.LocalName())
	return addr == target || name == target || strings.Contains(name, target)
}

func describeScanResult(result tinyble.ScanResult) string {
	if name := result.LocalName(); name != "" {
		return fmt.Sprintf("%s (%s)", name, result.Address.String())
	}
	return result.Address.String()
}

func looksMeshCoreName(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	return strings.HasPrefix(n, "meshcore") ||
		strings.HasPrefix(n, "whisper") ||
		strings.HasPrefix(n, "wiscore") ||
		strings.HasPrefix(n, "ht-") ||
		strings.HasPrefix(n, "lowmesh_mc_") ||
		strings.HasPrefix(n, "nrf52")
}

func mustUUID(s string) tinyble.UUID {
	u, err := tinyble.ParseUUID(s)
	if err != nil {
		panic(err)
	}
	return u
}

// Dialer dials ble:// URIs for the transport registry.
type Dialer struct{}

// NewDialer returns a BLE Dialer.
func NewDialer() *Dialer { return &Dialer{} }

// Dial implements transport.Dialer. The URI host/path/opaque is used as a
// platform Bluetooth address/UUID or advertised name. Empty ble:// scans for the
// first MeshCore BLE device.
func (Dialer) Dial(ctx context.Context, uri *url.URL) (transport.PacketConn, error) {
	target := uri.Host
	if target == "" {
		target = strings.TrimPrefix(uri.Path, "/")
	}
	if target == "" {
		target = uri.Opaque
	}
	timeout := defaultScanTimeout
	if raw := uri.Query().Get("scan_timeout"); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			timeout = d
		}
	}
	return New(target, WithScanTimeout(timeout)), nil
}
