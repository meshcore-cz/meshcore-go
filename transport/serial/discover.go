package serial

import (
	"context"
	"strings"

	"go.bug.st/serial/enumerator"

	"github.com/meshcore-cz/meshcore-go/transport"
)

// Discoverer enumerates USB serial ports as candidate companion endpoints.
//
// A discovered port is only a candidate: discovery and protocol verification
// are separate, so a port is not assumed to be a MeshCore device until a
// handshake succeeds.
type Discoverer struct{}

// NewDiscoverer returns a serial Discoverer.
func NewDiscoverer() *Discoverer { return &Discoverer{} }

var _ transport.Discoverer = (*Discoverer)(nil)

// Discover lists serial ports, preferring USB CDC-ACM devices.
func (Discoverer) Discover(ctx context.Context) ([]transport.Endpoint, error) {
	ports, err := enumerator.GetDetailedPortsList()
	if err != nil {
		return nil, err
	}

	var endpoints []transport.Endpoint
	for _, p := range ports {
		// Skip ports with no USB identity: they are rarely companion radios.
		if !p.IsUSB {
			continue
		}
		name := p.Product
		if name == "" {
			name = strings.TrimSpace(p.Manufacturer)
		}
		meta := map[string]string{}
		if p.VID != "" {
			meta["vid"] = p.VID
		}
		if p.PID != "" {
			meta["pid"] = p.PID
		}
		if p.SerialNumber != "" {
			meta["serial"] = p.SerialNumber
		}
		endpoints = append(endpoints, transport.Endpoint{
			URI:       "serial://" + p.Name,
			Transport: "serial",
			Name:      name,
			Address:   p.Name,
			Metadata:  meta,
		})
	}
	return endpoints, nil
}
