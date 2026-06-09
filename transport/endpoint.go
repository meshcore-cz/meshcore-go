package transport

import (
	"fmt"
	"net/url"
	"strings"
)

// ParseURI parses an endpoint URI such as "serial:///dev/ttyACM0" or
// "ble://C4:20:12:34:56:78" and validates that it carries a scheme.
func ParseURI(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("transport: invalid URI %q: %w", raw, err)
	}
	if u.Scheme == "" {
		return nil, fmt.Errorf("transport: URI %q has no scheme", raw)
	}
	return u, nil
}

// Scheme returns the URI scheme — everything before the first ':' — or "" when
// the URI carries no scheme. It is a lightweight alternative to ParseURI for
// callers that only need the transport kind (e.g. "serial", "ble", "tcp").
func Scheme(uri string) string {
	if i := strings.IndexByte(uri, ':'); i >= 0 {
		return uri[:i]
	}
	return ""
}

// Address extracts the transport-specific address from a parsed URI.
//
// For path-style schemes (serial:///dev/ttyACM0) it returns the path; for
// authority-style schemes (ble://C4:20:..., tcp://host:port) it returns the
// host (including port when present).
func Address(u *url.URL) string {
	if u.Host != "" {
		return u.Host
	}
	return strings.TrimPrefix(u.Path, "")
}
