package meshcore

import (
	"context"

	"github.com/meshcore-dev/meshcore-go/transport"
	"github.com/meshcore-dev/meshcore-go/transport/ble"
	"github.com/meshcore-dev/meshcore-go/transport/serial"
)

// Endpoint is re-exported from the transport package for convenience.
type Endpoint = transport.Endpoint

// DiscoverOption enables a discovery provider.
type DiscoverOption func(*discoverConfig)

type discoverConfig struct {
	providers []transport.Discoverer
}

// WithSerialDiscovery enables USB serial discovery.
func WithSerialDiscovery() DiscoverOption {
	return func(c *discoverConfig) {
		c.providers = append(c.providers, serial.NewDiscoverer())
	}
}

// WithBLEDiscovery enables BLE discovery.
func WithBLEDiscovery() DiscoverOption {
	return func(c *discoverConfig) {
		c.providers = append(c.providers, ble.NewDiscoverer())
	}
}

// WithDiscoverer registers a custom discovery provider.
func WithDiscoverer(d transport.Discoverer) DiscoverOption {
	return func(c *discoverConfig) { c.providers = append(c.providers, d) }
}

// Discover runs the enabled discovery providers and returns the combined set of
// candidate endpoints. Discovery does not verify that an endpoint is a MeshCore
// device; verification happens at handshake time.
func Discover(ctx context.Context, opts ...DiscoverOption) ([]Endpoint, error) {
	var cfg discoverConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	if len(cfg.providers) == 0 {
		cfg.providers = append(cfg.providers, serial.NewDiscoverer())
	}

	var endpoints []Endpoint
	for _, p := range cfg.providers {
		eps, err := p.Discover(ctx)
		if err != nil {
			return endpoints, err
		}
		endpoints = append(endpoints, eps...)
	}
	return endpoints, nil
}
