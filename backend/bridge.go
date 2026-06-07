package backend

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"runtime"
	"strings"
	"time"

	"github.com/creack/pty"
	meshcore "github.com/meshcore-cz/meshcore-go"
	"github.com/meshcore-cz/meshcore-go/transport"
	"github.com/meshcore-cz/meshcore-go/transport/serial"
	"golang.org/x/term"
)

type BridgeConfig struct {
	Enabled bool
	Type    string
	Listen  string
	Name    string
}

type BridgeStatus struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Listen string `json:"listen,omitempty"`
	Path   string `json:"path,omitempty"`
	Active bool   `json:"active"`
	Error  string `json:"error,omitempty"`
	Note   string `json:"note,omitempty"`
}

type BridgeOption func(*DeviceSession)

func WithBridges(bridges []BridgeConfig) BridgeOption {
	return func(s *DeviceSession) {
		s.bridges = append([]BridgeConfig(nil), bridges...)
	}
}

type bridgeRuntime struct {
	status BridgeStatus
}

func (s *DeviceSession) startBridges() {
	for _, cfg := range s.bridges {
		if !cfg.Enabled {
			continue
		}
		rt := &bridgeRuntime{status: BridgeStatus{
			Name:   cfg.Name,
			Type:   strings.ToLower(cfg.Type),
			Listen: cfg.Listen,
		}}
		if rt.status.Name == "" {
			rt.status.Name = rt.status.Type
		}
		s.addBridgeRuntime(rt)
		switch rt.status.Type {
		case "tcp":
			go s.serveTCPBridge(rt)
		case "pty":
			go s.servePTYBridge(rt)
		default:
			s.setBridgeError(rt, fmt.Sprintf("unknown bridge type %q", cfg.Type))
		}
	}
}

func (s *DeviceSession) serveTCPBridge(rt *bridgeRuntime) {
	addr := rt.status.Listen
	if addr == "" {
		addr = "127.0.0.1:4403"
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		s.setBridgeError(rt, err.Error())
		return
	}
	defer ln.Close()
	rt.status.Listen = ln.Addr().String()
	s.updateBridgeStatus(rt, rt.status)

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-s.stopped:
				return
			default:
				s.setBridgeError(rt, err.Error())
				return
			}
		}
		s.runBridgeConn(rt, conn, true)
	}
}

func (s *DeviceSession) servePTYBridge(rt *bridgeRuntime) {
	ptmx, tty, err := pty.Open()
	if err != nil {
		s.setBridgeError(rt, err.Error())
		return
	}
	defer ptmx.Close()
	defer tty.Close()
	go func() {
		<-s.stopped
		_ = ptmx.Close()
		_ = tty.Close()
	}()
	if _, err := term.MakeRaw(int(tty.Fd())); err != nil {
		s.setBridgeError(rt, "pty raw mode: "+err.Error())
	}
	rt.status.Path = tty.Name()
	if runtime.GOOS == "darwin" {
		rt.status.Note = "macOS Chrome/Web Serial does not list PTY bridges; use this path with native serial clients, or use a TCP bridge with clients that support it"
	}
	s.updateBridgeStatus(rt, rt.status)

	br := bufio.NewReader(ptmx)
	for {
		select {
		case <-s.stopped:
			return
		default:
		}
		first, err := serial.ReadHostFrameResync(br)
		if err != nil {
			select {
			case <-s.stopped:
				return
			default:
				s.setBridgeError(rt, err.Error())
				return
			}
		}
		s.runBridgeConnWithReader(rt, noCloseReadWriter{ptmx}, br, first)
	}
}

func (s *DeviceSession) runBridgeConn(rt *bridgeRuntime, rw io.ReadWriter, closeAfter ...bool) {
	s.runBridgeConnWithReader(rt, rw, bufio.NewReader(rw), nil, closeAfter...)
}

func (s *DeviceSession) runBridgeConnWithReader(rt *bridgeRuntime, rw io.ReadWriter, br *bufio.Reader, first []byte, closeAfter ...bool) {
	if len(closeAfter) > 0 && closeAfter[0] {
		if c, ok := rw.(io.Closer); ok {
			defer c.Close()
		}
	}
	if err := s.enterBridge(rt); err != nil {
		s.setBridgeError(rt, err.Error())
		return
	}
	defer s.leaveBridge(rt)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn, err := meshcore.DefaultRegistry().Dial(ctx, s.uri)
	if err != nil {
		s.setBridgeError(rt, err.Error())
		return
	}
	if err := conn.Open(ctx); err != nil {
		s.setBridgeError(rt, err.Error())
		return
	}
	defer conn.Close()

	errc := make(chan error, 2)
	go func() { errc <- bridgeHostToDevice(ctx, br, conn, first) }()
	go func() { errc <- bridgeDeviceToHost(ctx, rw, conn) }()
	err = <-errc
	cancel()
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, context.Canceled) {
		s.setBridgeError(rt, err.Error())
	}
}

func bridgeHostToDevice(ctx context.Context, br *bufio.Reader, conn transport.PacketConn, first []byte) error {
	if len(first) > 0 {
		if err := conn.WritePacket(ctx, first); err != nil {
			return err
		}
	}
	for {
		packet, err := serial.ReadHostFrameResync(br)
		if err != nil {
			return err
		}
		if err := conn.WritePacket(ctx, packet); err != nil {
			return err
		}
	}
}

func bridgeDeviceToHost(ctx context.Context, w io.Writer, conn transport.PacketConn) error {
	for {
		packet, err := conn.ReadPacket(ctx)
		if err != nil {
			return err
		}
		if err := serial.WriteDeviceFrame(w, packet); err != nil {
			return err
		}
	}
}

func (s *DeviceSession) enterBridge(rt *bridgeRuntime) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == stateBridge {
		return fmt.Errorf("another bridge is already active")
	}
	if s.client != nil {
		_ = s.client.Close()
		s.client = nil
	}
	s.state = stateBridge
	s.lastError = ""
	rt.status.Active = true
	s.bridgeStatuses[rt.status.Name] = rt.status
	return nil
}

func (s *DeviceSession) leaveBridge(rt *bridgeRuntime) {
	s.mu.Lock()
	if s.state == stateBridge {
		s.state = stateDegraded
		s.lastError = "bridge disconnected; reconnecting SDK client"
		s.lastErrorAt = time.Now()
	}
	rt.status.Active = false
	s.bridgeStatuses[rt.status.Name] = rt.status
	s.mu.Unlock()
	go s.tryReconnect()
}

func (s *DeviceSession) addBridgeRuntime(rt *bridgeRuntime) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.bridgeStatuses == nil {
		s.bridgeStatuses = map[string]BridgeStatus{}
	}
	s.bridgeStatuses[rt.status.Name] = rt.status
}

func (s *DeviceSession) updateBridgeStatus(rt *bridgeRuntime, status BridgeStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rt.status = status
	if s.bridgeStatuses == nil {
		s.bridgeStatuses = map[string]BridgeStatus{}
	}
	s.bridgeStatuses[rt.status.Name] = rt.status
}

func (s *DeviceSession) setBridgeError(rt *bridgeRuntime, msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rt.status.Error = msg
	if s.bridgeStatuses == nil {
		s.bridgeStatuses = map[string]BridgeStatus{}
	}
	s.bridgeStatuses[rt.status.Name] = rt.status
}

func (s *DeviceSession) bridgeStatusLocked() []BridgeStatus {
	out := make([]BridgeStatus, 0, len(s.bridgeStatuses))
	for _, st := range s.bridgeStatuses {
		out = append(out, st)
	}
	return out
}

type noCloseReadWriter struct {
	io.ReadWriter
}
