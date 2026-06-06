package meshcore

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/meshcore-dev/meshcore-go/protocol/companion"
)

// Trace is the result of a path trace: the route to a target with per-link
// signal quality.
type Trace struct {
	Target    string
	Tag       uint32
	Path      []byte    // intermediate node hashes traversed
	SNRs      []float64 // per-link SNR in dB (round trip)
	RoundTrip time.Duration
}

// Trace traces a route through one or more nodes. The target may be:
//
//   - a hash path such as "25" or "25,a1" (hex byte per hop), or
//   - a contact name / key prefix, whose node hash (first key byte) is used.
//
// Tracing transmits a path-trace packet over the mesh and waits for the
// device's TraceData push. A single-hash target only reaches a direct
// neighbour; distant nodes require the full multi-hop path (e.g. "25,45").
func (c *Client) Trace(ctx context.Context, target string) (Trace, error) {
	if err := c.requireCapability(CapabilityTracing); err != nil {
		return Trace{}, err
	}

	path, name, err := c.resolveTracePath(ctx, target)
	if err != nil {
		return Trace{}, err
	}
	return c.tracePath(ctx, path, name)
}

// TraceContact traces a route to an already-resolved contact.
func (c *Client) TraceContact(ctx context.Context, contact Contact) (Trace, error) {
	if err := c.requireCapability(CapabilityTracing); err != nil {
		return Trace{}, err
	}
	key, err := hex.DecodeString(contact.PublicKey)
	if err != nil || len(key) == 0 {
		return Trace{}, fmt.Errorf("trace: contact %q has no usable key", contact.Name)
	}
	return c.tracePath(ctx, []byte{key[0]}, contact.Name)
}

func (c *Client) tracePath(ctx context.Context, path []byte, name string) (Trace, error) {
	tag := rand.Uint32()
	started := time.Now()

	msg, err := c.request(ctx, companion.SendTracePath{Tag: tag, Path: path})
	if err != nil {
		return Trace{}, err
	}

	// The immediate reply suggests how long to wait for the result.
	wait := c.timeout
	if sent, ok := msg.(companion.Sent); ok && sent.SuggestedTimeout > 0 {
		wait = sent.SuggestedTimeout + time.Second
	}
	wctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()

	for {
		select {
		case <-wctx.Done():
			return Trace{}, fmt.Errorf("trace to %s: no response: %w", name, wctx.Err())
		case <-c.closed:
			return Trace{}, fmt.Errorf("meshcore: connection closed")
		case td := <-c.traces:
			if td.Tag != tag {
				continue
			}
			return Trace{
				Target:    name,
				Tag:       td.Tag,
				Path:      td.Path,
				SNRs:      td.SNRs,
				RoundTrip: time.Since(started),
			}, nil
		}
	}
}

// resolveTracePath turns a target into a hash path plus a display name.
func (c *Client) resolveTracePath(ctx context.Context, target string) ([]byte, string, error) {
	if path, ok := parseHashPath(target); ok {
		return path, target, nil
	}
	ct, err := c.Contact(ctx, target)
	if err != nil {
		return nil, target, fmt.Errorf("trace target %q: %w", target, err)
	}
	key, err := hex.DecodeString(ct.PublicKey)
	if err != nil || len(key) == 0 {
		return nil, ct.Name, fmt.Errorf("trace: contact %q has no usable key", ct.Name)
	}
	return []byte{key[0]}, ct.Name, nil
}

// parseHashPath parses a comma- or space-separated list of hex bytes such as
// "25" or "25,a1" into a hash path. It reports false if any token is not a
// two-digit hex byte.
func parseHashPath(s string) ([]byte, bool) {
	fields := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' })
	if len(fields) == 0 {
		return nil, false
	}
	path := make([]byte, 0, len(fields))
	for _, f := range fields {
		if len(f) != 2 {
			return nil, false
		}
		b, err := hex.DecodeString(f)
		if err != nil {
			return nil, false
		}
		path = append(path, b[0])
	}
	return path, true
}
