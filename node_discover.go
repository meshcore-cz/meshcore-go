package meshcore

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/meshcore-cz/meshcore-go/protocol/companion"
)

// Node-type filter bits for NodeDiscoverOptions.Filter (bit = 1<<node_type).
const (
	NodeFilterCompanion byte = 1 << 1 // 2
	NodeFilterRepeater  byte = 1 << 2 // 4
	NodeFilterRoom      byte = 1 << 3 // 8
	NodeFilterSensor    byte = 1 << 4 // 16
	NodeFilterAll       byte = 0xFF
)

// defaultDiscoverWindow is how long DiscoverNodes listens for replies when the
// caller does not set NodeDiscoverOptions.Timeout.
const defaultDiscoverWindow = 6 * time.Second

// DiscoveredNode is one reply to a node-discovery request: a node that heard
// the request and answered, with the round-trip signal quality.
type DiscoveredNode struct {
	PublicKey string      `json:"public_key"`
	Type      ContactType `json:"type"`
	SNRUp     float64     `json:"snr_up"`   // dB, how the remote heard our request
	SNRDown   float64     `json:"snr_down"` // dB, how we heard the reply
	RSSI      int         `json:"rssi"`     // dBm
	PathLen   int         `json:"path_len"`
	Tag       uint32      `json:"tag"`
}

// NodeDiscoverOptions tunes a node-discovery scan.
type NodeDiscoverOptions struct {
	// Filter is a node-type bitmask (NodeFilterRepeater, NodeFilterCompanion, …).
	// Zero defaults to NodeFilterAll.
	Filter byte
	// PrefixOnly requests 8-byte key prefixes instead of full public keys.
	PrefixOnly bool
	// Timeout bounds how long to listen for replies. Zero uses a default window.
	Timeout time.Duration
}

// DiscoverNodes broadcasts a node-discovery request and listens for replies
// until the timeout elapses. If onNode is non-nil it is called for each node as
// it arrives (so callers can render results live); every node is also returned.
//
// The wire format is firmware-derived (from the meshcore_py reference) and not
// yet hardware-verified.
func (c *Client) DiscoverNodes(ctx context.Context, opts NodeDiscoverOptions, onNode func(DiscoveredNode)) ([]DiscoveredNode, error) {
	filter := opts.Filter
	if filter == 0 {
		filter = NodeFilterAll
	}
	tag := rand.Uint32()
	if _, err := c.request(ctx, companion.SendNodeDiscoverReq{
		Filter:     filter,
		PrefixOnly: opts.PrefixOnly,
		Tag:        tag,
	}); err != nil {
		return nil, err
	}

	wait := opts.Timeout
	if wait <= 0 {
		wait = defaultDiscoverWindow
	}
	wctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()

	var nodes []DiscoveredNode
	seen := make(map[string]bool)
	for {
		select {
		case <-wctx.Done():
			return nodes, nil
		case <-c.closed:
			return nodes, fmt.Errorf("meshcore: connection closed")
		case r := <-c.discoveries:
			if r.Tag != tag {
				continue
			}
			if r.PublicKey != "" && seen[r.PublicKey] {
				continue
			}
			seen[r.PublicKey] = true
			n := DiscoveredNode{
				PublicKey: r.PublicKey,
				Type:      contactType(r.NodeType),
				SNRUp:     r.SNRUp,
				SNRDown:   r.SNRDown,
				RSSI:      r.RSSI,
				PathLen:   int(r.PathLen),
				Tag:       r.Tag,
			}
			nodes = append(nodes, n)
			if onNode != nil {
				onNode(n)
			}
		}
	}
}
