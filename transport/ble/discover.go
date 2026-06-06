package ble

import (
	"context"
	"fmt"
	"time"

	"github.com/meshcore-cz/meshcore-go/transport"
	tinyble "tinygo.org/x/bluetooth"
)

// Discoverer enumerates BLE devices advertising the MeshCore/Nordic UART
// service or a known MeshCore companion name prefix.
type Discoverer struct {
	adapter *tinyble.Adapter
	timeout time.Duration
}

// NewDiscoverer returns a BLE Discoverer.
func NewDiscoverer() *Discoverer {
	return &Discoverer{
		adapter: tinyble.DefaultAdapter,
		timeout: defaultScanTimeout,
	}
}

var _ transport.Discoverer = (*Discoverer)(nil)

// Discover scans for candidate MeshCore BLE companion endpoints.
func (d *Discoverer) Discover(ctx context.Context) ([]transport.Endpoint, error) {
	adapter := d.adapter
	if adapter == nil {
		adapter = tinyble.DefaultAdapter
	}
	if err := ensureAdapterEnabled(adapter); err != nil {
		return nil, fmt.Errorf("ble: enable adapter: %w", err)
	}
	scanCtx := ctx
	cancel := func() {}
	if _, ok := scanCtx.Deadline(); !ok {
		scanCtx, cancel = context.WithTimeout(ctx, d.timeout)
	}
	defer cancel()

	found := make(chan transport.Endpoint, 32)
	errc := make(chan error, 1)
	go func() {
		err := adapter.Scan(func(adapter *tinyble.Adapter, result tinyble.ScanResult) {
			if !result.AdvertisementPayload.HasServiceUUID(serviceUUID) && !looksMeshCoreName(result.LocalName()) {
				return
			}
			addr := result.Address.String()
			meta := map[string]string{
				"rssi": fmt.Sprintf("%d", result.RSSI),
			}
			if result.AdvertisementPayload.HasServiceUUID(serviceUUID) {
				meta["service_uuid"] = ServiceUUID
			}
			ep := transport.Endpoint{
				URI:       "ble://" + addr,
				Transport: "ble",
				Name:      result.LocalName(),
				Address:   addr,
				Metadata:  meta,
			}
			select {
			case found <- ep:
			default:
			}
		})
		if err != nil {
			errc <- err
		}
	}()

	out := []transport.Endpoint{}
	seen := map[string]bool{}
	for {
		select {
		case ep := <-found:
			if !seen[ep.Address] {
				seen[ep.Address] = true
				out = append(out, ep)
			}
		case err := <-errc:
			return out, fmt.Errorf("ble: scan: %w", err)
		case <-scanCtx.Done():
			_ = adapter.StopScan()
			return out, nil
		}
	}
}
