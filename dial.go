package meshcore

import (
	"context"

	"github.com/meshcore-cz/meshcore-go/transport"
	"github.com/meshcore-cz/meshcore-go/transport/ble"
	"github.com/meshcore-cz/meshcore-go/transport/serial"
)

// DialOption configures Dial.
type DialOption func(*dialConfig)

type dialConfig struct {
	registry   *transport.Registry
	clientOpts []Option
}

// WithTransportRegistry supplies a custom transport registry, allowing
// applications to register additional schemes.
func WithTransportRegistry(r *transport.Registry) DialOption {
	return func(c *dialConfig) { c.registry = r }
}

// WithClientOptions forwards Client options through Dial.
func WithClientOptions(opts ...Option) DialOption {
	return func(c *dialConfig) { c.clientOpts = append(c.clientOpts, opts...) }
}

// DefaultRegistry returns a registry with the built-in transports registered.
func DefaultRegistry() *transport.Registry {
	r := transport.NewRegistry()
	r.Register("serial", serial.NewDialer())
	r.Register("ble", ble.NewDialer())
	// tcp and ws dialers register here as later phases land.
	return r
}

// Dial connects to a companion radio at the given endpoint URI and performs the
// protocol handshake.
//
//	client, err := meshcore.Dial(ctx, "serial:///dev/ttyACM0")
func Dial(ctx context.Context, uri string, opts ...DialOption) (*Client, error) {
	cfg := dialConfig{registry: DefaultRegistry()}
	for _, opt := range opts {
		opt(&cfg)
	}

	conn, err := cfg.registry.Dial(ctx, uri)
	if err != nil {
		return nil, err
	}

	client := New(conn, cfg.clientOpts...)
	if err := client.Connect(ctx); err != nil {
		return nil, err
	}
	return client, nil
}
