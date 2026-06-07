package meshcore

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/meshcore-cz/meshcore-go/protocol/companion"
	"github.com/meshcore-cz/meshcore-go/protocol/pathhash"
)

// TracePlan describes how a trace target is resolved before sending.
type TracePlan struct {
	Query      string
	Name       string // label used in results (contact name or original query)
	Source     string // explicit_path, contact_out_path, contact_direct_path, contact_key_fallback
	Contact    string // contact name when resolved via contact lookup
	Path       []byte
	HashSize   int
	Flags      byte
	HopCount   int
	OutPathEnc byte // contact out-path metadata; 0xff when not used
	PrefixHint int  // hex prefix byte length from query, when applicable
}

// PlanTraceContact builds a trace plan for a resolved contact.
func PlanTraceContact(ct Contact, hintBytes int) (TracePlan, error) {
	path, hashSize, flags, err := tracePathForContact(ct, hintBytes)
	if err != nil {
		return TracePlan{}, err
	}
	plan := TracePlan{
		Query:      ct.Name,
		Name:       ct.Name,
		Contact:    ct.Name,
		Path:       path,
		HashSize:   hashSize,
		Flags:      flags,
		HopCount:   hopCount(path, hashSize),
		PrefixHint: hintBytes,
	}
	if ct.HasPath && pathhash.HasStoredOutPath(ct.OutPathEnc, ct.OutPath) {
		plan.Source = "contact_out_path"
		plan.OutPathEnc = ct.OutPathEnc
	} else if ct.HasPath && pathhash.IsDirectOutPath(ct.OutPathEnc) {
		plan.Source = "contact_direct_path"
		plan.OutPathEnc = ct.OutPathEnc
	} else {
		plan.Source = "contact_key_fallback"
	}
	return plan, nil
}

// Trace is the result of a path trace: the route to a target with per-link
// signal quality.
type Trace struct {
	Target       string
	Tag          uint32
	Path         []byte // raw path bytes (hop_count * hash_size)
	PathHashSize int    // bytes per hop (1, 2, 4, or 8)
	SNRs         []float64
	RoundTrip    time.Duration
}

// Trace traces a route through one or more nodes. The target may be:
//
//   - a hash path such as "25", "a1b2,c3d4", or "01020304,05060708" (uniform
//     hex width per hop: 1, 2, 4, or 8 bytes), or
//   - a contact name, using the contact's stored out-path when available or a
//     pubkey hash fallback for direct routes.
//
// Hex targets are always treated as explicit paths, even when they match a
// contact key prefix.
func (c *Client) Trace(ctx context.Context, target string) (Trace, error) {
	if err := c.requireCapability(CapabilityTracing); err != nil {
		return Trace{}, err
	}

	path, hashSize, flags, name, err := c.resolveTracePath(ctx, target)
	if err != nil {
		return Trace{}, err
	}
	return c.tracePath(ctx, path, hashSize, flags, name)
}

// TraceContact traces a route to an already-resolved contact.
func (c *Client) TraceContact(ctx context.Context, contact Contact) (Trace, error) {
	return c.TraceContactWithHint(ctx, contact, 0)
}

// TraceContactWithHint traces a route to a resolved contact. hintBytes is the
// byte length of a hex key prefix query, when applicable.
func (c *Client) TraceContactWithHint(ctx context.Context, contact Contact, hintBytes int) (Trace, error) {
	if err := c.requireCapability(CapabilityTracing); err != nil {
		return Trace{}, err
	}
	path, hashSize, flags, err := tracePathForContact(contact, hintBytes)
	if err != nil {
		return Trace{}, err
	}
	plan := TracePlan{
		Query:      contact.Name,
		Name:       contact.Name,
		Contact:    contact.Name,
		Path:       path,
		HashSize:   hashSize,
		Flags:      flags,
		HopCount:   hopCount(path, hashSize),
		PrefixHint: hintBytes,
	}
	if contact.HasPath && pathhash.HasStoredOutPath(contact.OutPathEnc, contact.OutPath) {
		plan.Source = "contact_out_path"
		plan.OutPathEnc = contact.OutPathEnc
	} else if contact.HasPath && pathhash.IsDirectOutPath(contact.OutPathEnc) {
		plan.Source = "contact_direct_path"
		plan.OutPathEnc = contact.OutPathEnc
	} else {
		plan.Source = "contact_key_fallback"
	}
	c.logTracePlan(plan)
	return c.tracePath(ctx, path, hashSize, flags, contact.Name)
}

func (c *Client) tracePath(ctx context.Context, path []byte, hashSize int, flags byte, name string) (Trace, error) {
	tag := rand.Uint32()
	started := time.Now()

	c.log.Debug("trace send",
		"target", name,
		"tag", fmt.Sprintf("%08x", tag),
		"flags", fmt.Sprintf("0x%02x", flags),
		"path_hex", hex.EncodeToString(path),
		"hop_count", hopCount(path, hashSize),
		"hops", formatTraceHops(path, hashSize),
		"hash_size", hashSize,
	)

	msg, err := c.request(ctx, companion.SendTracePath{Tag: tag, Flags: flags, Path: path})
	if err != nil {
		c.log.Debug("trace send failed", "target", name, "tag", fmt.Sprintf("%08x", tag), "error", err)
		return Trace{}, err
	}

	wait := c.timeout
	if sent, ok := msg.(companion.Sent); ok && sent.SuggestedTimeout > 0 {
		wait = sent.SuggestedTimeout + time.Second
	}
	c.log.Debug("trace waiting",
		"target", name,
		"tag", fmt.Sprintf("%08x", tag),
		"timeout", wait.Round(time.Millisecond),
	)

	wctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()

	for {
		select {
		case <-wctx.Done():
			c.log.Debug("trace timeout",
				"target", name,
				"tag", fmt.Sprintf("%08x", tag),
				"elapsed", time.Since(started).Round(time.Millisecond),
				"error", wctx.Err(),
			)
			return Trace{}, fmt.Errorf("trace to %s: no response: %w", name, wctx.Err())
		case <-c.closed:
			return Trace{}, fmt.Errorf("meshcore: connection closed")
		case td := <-c.traces:
			if td.Tag != tag {
				c.log.Debug("trace ignored",
					"expected_tag", fmt.Sprintf("%08x", tag),
					"got_tag", fmt.Sprintf("%08x", td.Tag),
				)
				continue
			}
			size := pathhash.HashSizeFromTraceFlags(td.Flags)
			if size <= 0 {
				size = hashSize
			}
			c.log.Debug("trace received",
				"target", name,
				"tag", fmt.Sprintf("%08x", td.Tag),
				"path_hex", hex.EncodeToString(td.Path),
				"hop_count", hopCount(td.Path, size),
				"hops", formatTraceHops(td.Path, size),
				"hash_size", size,
				"flags", fmt.Sprintf("0x%02x", td.Flags),
				"snr_count", len(td.SNRs),
				"elapsed", time.Since(started).Round(time.Millisecond),
			)
			return Trace{
				Target:       name,
				Tag:          td.Tag,
				Path:         td.Path,
				PathHashSize: size,
				SNRs:         td.SNRs,
				RoundTrip:    time.Since(started),
			}, nil
		}
	}
}

func (c *Client) resolveTracePath(ctx context.Context, target string) ([]byte, int, byte, string, error) {
	plan, err := c.planTrace(ctx, target)
	if err != nil {
		return nil, 0, 0, target, err
	}
	c.logTracePlan(plan)
	return plan.Path, plan.HashSize, plan.Flags, plan.Name, nil
}

func (c *Client) planTrace(ctx context.Context, target string) (TracePlan, error) {
	if pathhash.IsHexTraceTarget(target) {
		return explicitTracePlan(target)
	}
	ct, err := c.Contact(ctx, target)
	if err != nil {
		return TracePlan{}, fmt.Errorf("trace target %q: %w", target, err)
	}
	plan, err := PlanTraceContact(ct, 0)
	if err != nil {
		return TracePlan{}, err
	}
	plan.Query = target
	return plan, nil
}

func explicitTracePlan(target string) (TracePlan, error) {
	path, hashSize, err := pathhash.ParsePath(target)
	if err != nil {
		return TracePlan{}, err
	}
	flags, err := pathhash.TraceFlagsFromHashSize(hashSize)
	if err != nil {
		return TracePlan{}, err
	}
	return TracePlan{
		Query:    target,
		Name:     target,
		Source:   "explicit_path",
		Path:     path,
		HashSize: hashSize,
		Flags:    flags,
		HopCount: hopCount(path, hashSize),
	}, nil
}

func hopCount(path []byte, hashSize int) int {
	if hashSize <= 0 || len(path) == 0 {
		return 0
	}
	return len(path) / hashSize
}

func (c *Client) logTracePlan(plan TracePlan) {
	hops := formatTraceHops(plan.Path, plan.HashSize)
	args := []any{
		"query", plan.Query,
		"resolved_as", plan.Name,
		"source", plan.Source,
		"path_hex", hex.EncodeToString(plan.Path),
		"path_bytes", len(plan.Path),
		"hop_count", plan.HopCount,
		"hops", hops,
		"hash_size", plan.HashSize,
		"flags", fmt.Sprintf("0x%02x", plan.Flags),
	}
	if plan.Contact != "" {
		args = append(args, "contact", plan.Contact)
	}
	if plan.OutPathEnc != 0 && plan.OutPathEnc != pathhash.OutPathUnknown {
		args = append(args,
			"out_path_enc", fmt.Sprintf("0x%02x", plan.OutPathEnc),
			"out_path_hops", pathhash.HopCountFromPathMeta(plan.OutPathEnc),
			"out_path_hash_size", pathhash.HashSizeFromPathMeta(plan.OutPathEnc),
		)
	}
	if plan.PrefixHint > 0 {
		args = append(args, "prefix_hint_bytes", plan.PrefixHint)
	}
	c.log.Debug("trace plan", args...)
}

func formatTraceHops(path []byte, hashSize int) string {
	hops := pathhash.Split(path, hashSize)
	if len(hops) == 0 {
		return ""
	}
	parts := make([]string, len(hops))
	for i, hop := range hops {
		parts[i] = pathhash.FormatHop(hop)
	}
	return strings.Join(parts, ",")
}

func tracePathForContact(ct Contact, hintBytes int) ([]byte, int, byte, error) {
	if ct.HasPath && pathhash.HasStoredOutPath(ct.OutPathEnc, ct.OutPath) {
		path, hashSize, flags, err := pathhash.TracePathFromOutPath(ct.OutPathEnc, ct.OutPath)
		if err != nil {
			return nil, 0, 0, fmt.Errorf("trace contact %q: %w", ct.Name, err)
		}
		return path, hashSize, flags, nil
	}
	if ct.HasPath && pathhash.IsDirectOutPath(ct.OutPathEnc) && hintBytes <= 0 {
		hintBytes = pathhash.HashSizeFromPathMeta(ct.OutPathEnc)
	}
	return traceKeyFallback(ct, hintBytes)
}

func traceKeyFallback(ct Contact, hintBytes int) ([]byte, int, byte, error) {
	key, err := hex.DecodeString(ct.PublicKey)
	if err != nil || len(key) == 0 {
		return nil, 0, 0, fmt.Errorf("trace: contact %q has no usable key", ct.Name)
	}
	if hintBytes <= 0 {
		hintBytes = 2
	}
	hashSize := pathhash.NearestTraceHashSize(hintBytes)
	if hashSize > len(key) {
		hashSize = pathhash.NearestTraceHashSize(len(key))
	}
	flags, err := pathhash.TraceFlagsFromHashSize(hashSize)
	if err != nil {
		return nil, 0, 0, err
	}
	return append([]byte(nil), key[:hashSize]...), hashSize, flags, nil
}
