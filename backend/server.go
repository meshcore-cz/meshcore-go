package backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	meshcore "github.com/meshcore-dev/meshcore-go"
	"github.com/meshcore-dev/meshcore-go/protocol"
)

// Server owns one long-lived MeshCore client and exposes it over a local Unix
// socket.
type Server struct {
	uri      string
	socket   string
	opts     []meshcore.DialOption
	client   *meshcore.Client
	listener net.Listener

	mu        sync.RWMutex
	state     string
	lastSeen  time.Time
	lastError string

	stopOnce sync.Once
	stopped  chan struct{}
}

const (
	stateReady    = "ready"
	stateDegraded = "degraded"
)

// NewServer dials uri and prepares a local backend server.
func NewServer(ctx context.Context, uri string, opts ...meshcore.DialOption) (*Server, error) {
	client, err := meshcore.Dial(ctx, uri, opts...)
	if err != nil {
		return nil, fmt.Errorf("connecting to %s: %w", uri, err)
	}
	return &Server{
		uri:      uri,
		socket:   SocketPath(),
		opts:     append([]meshcore.DialOption(nil), opts...),
		client:   client,
		state:    stateReady,
		lastSeen: time.Now(),
		stopped:  make(chan struct{}),
	}, nil
}

// Serve listens until Stop is called or the listener fails.
func (s *Server) Serve() error {
	if err := os.MkdirAll(filepath.Dir(s.socket), 0o700); err != nil {
		return err
	}
	if err := os.Remove(s.socket); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	ln, err := net.Listen("unix", s.socket)
	if err != nil {
		return err
	}
	if err := os.Chmod(s.socket, 0o600); err != nil {
		ln.Close()
		return err
	}
	s.listener = ln
	go s.keepAlive()
	defer func() {
		ln.Close()
		os.Remove(s.socket)
		if client := s.clientSnapshot(); client != nil {
			client.Close()
		}
		close(s.stopped)
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-s.stopped:
				return nil
			default:
				if errors.Is(err, net.ErrClosed) {
					return nil
				}
				return err
			}
		}
		go s.handle(conn)
	}
}

// Stop closes the listener and the MeshCore client.
func (s *Server) Stop() {
	s.stopOnce.Do(func() {
		if s.listener != nil {
			s.listener.Close()
		}
	})
}

func (s *Server) handle(conn net.Conn) {
	defer conn.Close()

	var req request
	enc := json.NewEncoder(conn)
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		_ = enc.Encode(response{OK: false, Error: err.Error()})
		return
	}
	if req.Method == "watch" {
		if !s.healthy() {
			_ = enc.Encode(response{ID: req.ID, OK: false, Error: "backend radio unavailable: " + s.lastErr()})
			return
		}
		s.watch(conn, req.ID)
		return
	}
	if req.Method == "watch_raw" {
		if !s.healthy() {
			_ = enc.Encode(response{ID: req.ID, OK: false, Error: "backend radio unavailable: " + s.lastErr()})
			return
		}
		s.watchRaw(conn, req.ID)
		return
	}

	result, err := s.dispatch(connContext(), req.Method, req.Params)
	resp := response{ID: req.ID, OK: err == nil}
	if err != nil {
		resp.Error = err.Error()
	} else if result != nil {
		resp.Result, err = json.Marshal(result)
		if err != nil {
			resp.OK = false
			resp.Error = err.Error()
		}
	}
	_ = enc.Encode(resp)

	if req.Method == "stop" && err == nil {
		go s.Stop()
	}
}

func (s *Server) watch(conn net.Conn, id uint64) {
	enc := json.NewEncoder(conn)
	if err := enc.Encode(response{ID: id, OK: true}); err != nil {
		return
	}
	client := s.clientSnapshot()
	if client == nil {
		return
	}
	for ev := range client.Events() {
		out, ok := backendEvent(ev)
		if !ok {
			continue
		}
		if err := enc.Encode(out); err != nil {
			return
		}
	}
}

func (s *Server) watchRaw(conn net.Conn, id uint64) {
	enc := json.NewEncoder(conn)
	if err := enc.Encode(response{ID: id, OK: true}); err != nil {
		return
	}
	client := s.clientSnapshot()
	if client == nil {
		return
	}
	for pkt := range client.RawPackets() {
		if err := enc.Encode(pkt); err != nil {
			return
		}
	}
}

func (s *Server) dispatch(ctx context.Context, method string, params json.RawMessage) (any, error) {
	switch method {
	case "status":
		return s.status(), nil
	case "stop":
		return map[string]bool{"stopping": true}, nil
	}
	if !s.healthy() {
		return nil, fmt.Errorf("backend radio unavailable: %s", s.lastErr())
	}
	client := s.clientSnapshot()
	if client == nil {
		return nil, fmt.Errorf("backend radio unavailable: no active client")
	}

	switch method {
	case "device_info":
		return client.DeviceInfo(ctx)
	case "contacts":
		return client.Contacts(ctx)
	case "contact":
		var p queryParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		return client.Contact(ctx, p.Query)
	case "inbox":
		return client.SyncMessages(ctx)
	case "send_text":
		var p sendTextParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		return client.SendText(ctx, p.Recipient, p.Text)
	case "wait_ack":
		var receipt meshcore.Receipt
		if err := json.Unmarshal(params, &receipt); err != nil {
			return nil, err
		}
		return client.WaitForAcknowledgement(ctx, receipt)
	case "trace":
		var p queryParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		return client.Trace(ctx, p.Query)
	case "channels":
		return client.Channels(ctx)
	case "channel":
		var p queryParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		return client.Channel(ctx, p.Query)
	case "send_channel_text":
		var p channelSendParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		return client.SendChannelText(ctx, p.Channel, p.Text)
	case "raw_send":
		var p rawParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		msg, err := client.RawSend(ctx, p.Payload)
		if err != nil {
			return nil, err
		}
		return RawResultFromMessage(msg), nil
	case "repeater_login":
		var p repeaterLoginParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		return nil, client.RepeaterLogin(ctx, p.Repeater, p.Password)
	case "repeater_status":
		var p queryParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		return client.RepeaterStatus(ctx, p.Query)
	case "repeater_neighbours":
		var p queryParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		return client.RepeaterNeighbours(ctx, p.Query)
	case "repeater_exec":
		var p repeaterExecParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		return client.RepeaterExec(ctx, p.Repeater, p.Command)
	default:
		return nil, fmt.Errorf("unknown backend method %q", method)
	}
}

func (s *Server) keepAlive() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopped:
			return
		case <-ticker.C:
			if !s.healthy() {
				s.tryReconnect()
				continue
			}
			client := s.clientSnapshot()
			if client == nil {
				s.markDegraded(fmt.Errorf("no active client"))
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			_, err := client.DeviceTime(ctx)
			cancel()
			if err != nil {
				s.markDegraded(err)
				continue
			}
			s.markReady()
		}
	}
}

func (s *Server) status() statusResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	transport := s.uri
	if s.client != nil {
		transport = s.client.Transport()
	}
	return statusResult{
		Running:   true,
		Healthy:   s.state == stateReady,
		State:     s.state,
		URI:       s.uri,
		Transport: transport,
		PID:       os.Getpid(),
		LastSeen:  s.lastSeen,
		LastError: s.lastError,
	}
}

func (s *Server) markReady() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = stateReady
	s.lastSeen = time.Now()
	s.lastError = ""
}

func (s *Server) markDegraded(err error) {
	var old *meshcore.Client
	s.mu.Lock()
	s.state = stateDegraded
	if err != nil {
		s.lastError = err.Error()
	}
	old = s.client
	s.client = nil
	s.mu.Unlock()
	if old != nil {
		old.Close()
	}
}

func (s *Server) healthy() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state == stateReady
}

func (s *Server) lastErr() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.lastError == "" {
		return "radio is not responding"
	}
	return s.lastError
}

func (s *Server) clientSnapshot() *meshcore.Client {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.client
}

func (s *Server) tryReconnect() {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	client, err := meshcore.Dial(ctx, s.uri, s.opts...)
	cancel()
	if err != nil {
		s.markReconnectFailed(err)
		return
	}

	s.mu.Lock()
	old := s.client
	s.client = client
	s.state = stateReady
	s.lastSeen = time.Now()
	s.lastError = ""
	s.mu.Unlock()
	if old != nil {
		old.Close()
	}
}

func (s *Server) markReconnectFailed(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = stateDegraded
	if err != nil {
		s.lastError = err.Error()
	}
}

func connContext() context.Context {
	return context.Background()
}

func backendEvent(ev meshcore.Event) (Event, bool) {
	switch m := ev.(type) {
	case meshcore.MessageReceived:
		from := m.From.Name
		if m.Channel != "" {
			from = "#" + m.Channel
		}
		return Event{Type: "message", From: from, Channel: m.Channel, Text: m.Text, Timestamp: m.Timestamp}, true
	case meshcore.MessageAcknowledged:
		return Event{Type: "ack", Code: m.Code, RTTMillis: m.RTT.Milliseconds()}, true
	case meshcore.AdvertisementReceived:
		return Event{Type: "advert", Name: m.Contact.Name, PublicKey: m.Contact.PublicKey}, true
	case meshcore.Disconnected:
		msg := ""
		if m.Err != nil {
			msg = m.Err.Error()
		}
		return Event{Type: "disconnected", Error: msg}, true
	default:
		return Event{}, false
	}
}

// RawResultFromMessage converts a decoded SDK protocol message into the stable
// representation used by the backend IPC API.
func RawResultFromMessage(msg protocol.Message) RawResult {
	switch m := msg.(type) {
	case protocol.RawMessage:
		return RawResult{
			Type:    "raw",
			Code:    m.Type,
			Push:    m.Push,
			Payload: m.Payload,
		}
	default:
		return RawResult{
			Type:    fmt.Sprintf("%T", msg),
			Decoded: fmt.Sprintf("%+v", msg),
		}
	}
}
