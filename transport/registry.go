package transport

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"sync"
)

// Dialer turns a parsed endpoint URI into an unopened PacketConn.
type Dialer interface {
	Dial(ctx context.Context, uri *url.URL) (PacketConn, error)
}

// DialerFunc adapts a plain function to the Dialer interface.
type DialerFunc func(ctx context.Context, uri *url.URL) (PacketConn, error)

// Dial implements Dialer.
func (f DialerFunc) Dial(ctx context.Context, uri *url.URL) (PacketConn, error) {
	return f(ctx, uri)
}

// Registry maps URI schemes to dialers, making URI-based dialing extensible.
// Third-party integrations can register their own schemes without modifying
// the core SDK.
type Registry struct {
	mu      sync.RWMutex
	dialers map[string]Dialer
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{dialers: make(map[string]Dialer)}
}

// Register associates a scheme (without the "://") with a dialer.
func (r *Registry) Register(scheme string, dialer Dialer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.dialers[scheme] = dialer
}

// Schemes returns the registered schemes in sorted order.
func (r *Registry) Schemes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.dialers))
	for s := range r.dialers {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// Dial parses the URI, looks up the matching dialer and returns an unopened
// PacketConn. The caller is responsible for calling Open.
func (r *Registry) Dial(ctx context.Context, uri string) (PacketConn, error) {
	u, err := ParseURI(uri)
	if err != nil {
		return nil, err
	}

	r.mu.RLock()
	dialer, ok := r.dialers[u.Scheme]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("transport: no dialer registered for scheme %q", u.Scheme)
	}

	return dialer.Dial(ctx, u)
}
