// Package tcp implements a MeshCore companion stream transport over TCP.
package tcp

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/url"
	"sync"

	"github.com/meshcore-cz/meshcore-go/transport"
	"github.com/meshcore-cz/meshcore-go/transport/internal/streamframe"
)

type Conn struct {
	uri  string
	addr string

	mu   sync.Mutex
	conn net.Conn
	r    *bufio.Reader
}

func New(uri, addr string) *Conn {
	return &Conn{uri: uri, addr: addr}
}

var _ transport.PacketConn = (*Conn)(nil)

func (c *Conn) Open(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		return nil
	}
	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", c.addr)
	if err != nil {
		return fmt.Errorf("tcp transport: %w", err)
	}
	c.conn = conn
	c.r = bufio.NewReader(conn)
	return nil
}

func (c *Conn) ReadPacket(ctx context.Context) ([]byte, error) {
	c.mu.Lock()
	conn := c.conn
	r := c.r
	c.mu.Unlock()
	if conn == nil || r == nil {
		return nil, fmt.Errorf("tcp transport: connection not open")
	}

	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.SetReadDeadline(timeNow())
		case <-done:
		}
	}()
	defer close(done)

	pkt, err := streamframe.Read(r, streamframe.ToHost)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return pkt, err
}

func (c *Conn) WritePacket(ctx context.Context, packet []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return fmt.Errorf("tcp transport: connection not open")
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = c.conn.SetWriteDeadline(timeNow())
		case <-done:
		}
	}()
	defer close(done)
	if err := streamframe.Write(c.conn, streamframe.ToDevice, packet); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	}
	return nil
}

func (c *Conn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	c.r = nil
	return err
}

func (c *Conn) String() string {
	return c.uri
}

type Dialer struct{}

func NewDialer() *Dialer { return &Dialer{} }

func (Dialer) Dial(ctx context.Context, uri *url.URL) (transport.PacketConn, error) {
	addr := uri.Host
	if addr == "" {
		addr = uri.Opaque
	}
	if addr == "" {
		return nil, fmt.Errorf("tcp transport: URI %q has no host:port address", uri)
	}
	return New(uri.String(), addr), nil
}
