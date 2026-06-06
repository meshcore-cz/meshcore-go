package transport

import (
	"context"
	"net/url"
	"testing"
)

type stubConn struct{ PacketConn }

func TestRegistryDial(t *testing.T) {
	r := NewRegistry()
	var gotPath string
	r.Register("serial", DialerFunc(func(ctx context.Context, u *url.URL) (PacketConn, error) {
		gotPath = u.Path
		return stubConn{}, nil
	}))

	if _, err := r.Dial(context.Background(), "serial:///dev/ttyACM0"); err != nil {
		t.Fatalf("Dial: %v", err)
	}
	if gotPath != "/dev/ttyACM0" {
		t.Errorf("dialer saw path %q, want /dev/ttyACM0", gotPath)
	}

	if _, err := r.Dial(context.Background(), "ble://x"); err == nil {
		t.Error("expected error for unregistered scheme")
	}

	if schemes := r.Schemes(); len(schemes) != 1 || schemes[0] != "serial" {
		t.Errorf("Schemes() = %v, want [serial]", schemes)
	}
}
