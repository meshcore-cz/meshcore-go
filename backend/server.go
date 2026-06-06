package backend

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
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
	bridges  []BridgeConfig
	client   *meshcore.Client
	listener net.Listener
	store    Store

	mu             sync.RWMutex
	state          string
	lastSeen       time.Time
	lastError      string
	bridgeStatuses map[string]BridgeStatus

	stopOnce sync.Once
	stopped  chan struct{}
}

const (
	stateReady    = "ready"
	stateDegraded = "degraded"
	stateBridge   = "bridge"
)

// NewServer dials uri and prepares a local backend server.
func NewServer(ctx context.Context, uri string, opts ...meshcore.DialOption) (*Server, error) {
	return NewServerWithBridges(ctx, uri, nil, opts...)
}

// NewServerWithBridges dials uri and prepares a backend with configured bridge
// listeners.
func NewServerWithBridges(ctx context.Context, uri string, bridges []BridgeConfig, opts ...meshcore.DialOption) (*Server, error) {
	client, err := meshcore.Dial(ctx, uri, opts...)
	if err != nil {
		return nil, fmt.Errorf("connecting to %s: %w", uri, err)
	}
	store, err := OpenSQLiteStore("")
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("opening backend store: %w", err)
	}
	return &Server{
		uri:      uri,
		socket:   SocketPath(),
		opts:     append([]meshcore.DialOption(nil), opts...),
		bridges:  append([]BridgeConfig(nil), bridges...),
		client:   client,
		store:    store,
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
	s.startBridges()
	go s.refreshContacts(context.Background())
	defer func() {
		ln.Close()
		os.Remove(s.socket)
		if client := s.clientSnapshot(); client != nil {
			client.Close()
		}
		if s.store != nil {
			s.store.Close()
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
	case "contacts":
		var p contactsParams
		if len(params) > 0 {
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, err
			}
		}
		client := s.clientSnapshot()
		return s.contacts(ctx, client, p)
	case "contact":
		var p queryParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		client := s.clientSnapshot()
		return s.contact(ctx, client, p.Query)
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
	case "inbox":
		return client.SyncMessages(ctx)
	case "send_text":
		var p sendTextParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		ct, err := s.contact(ctx, client, p.Recipient)
		if err == nil {
			return client.SendTextToContact(ctx, ct, p.Text)
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
		if _, ok := parseBackendHashPath(p.Query); !ok {
			ct, err := s.contact(ctx, client, p.Query)
			if err == nil {
				return client.TraceContact(ctx, ct)
			}
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
	case "repeater_has_connection":
		var p queryParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		active, err := client.RepeaterHasConnection(ctx, p.Query)
		if err != nil {
			return nil, err
		}
		return repeaterHasConnectionResult{Active: active}, nil
	case "repeater_login":
		var p repeaterLoginParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		return s.repeaterLogin(ctx, client, p.Repeater, p.Password)
	case "repeater_status":
		var p queryParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		if err := s.ensureRepeaterSession(ctx, client, p.Query, ""); err != nil {
			return nil, err
		}
		return client.RepeaterStatus(ctx, p.Query)
	case "repeater_neighbours":
		var p queryParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		if err := s.ensureRepeaterSession(ctx, client, p.Query, ""); err != nil {
			return nil, err
		}
		return client.RepeaterNeighbours(ctx, p.Query)
	case "repeater_exec":
		var p repeaterExecParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		if err := s.ensureRepeaterSession(ctx, client, p.Repeater, ""); err != nil {
			return nil, err
		}
		return client.RepeaterExec(ctx, p.Repeater, p.Command)
	default:
		return nil, fmt.Errorf("unknown backend method %q", method)
	}
}

func (s *Server) contacts(ctx context.Context, client *meshcore.Client, opts contactsParams) ([]meshcore.Contact, error) {
	if opts.Refresh {
		if client == nil || !s.healthy() {
			return nil, fmt.Errorf("backend radio unavailable: %s", s.lastErr())
		}
		return s.refreshContactsNow(ctx, client)
	}

	cached, err := s.cachedContacts(ctx)
	if err == nil && len(cached) > 0 {
		if client != nil && s.healthy() && !opts.Cached {
			go s.refreshContacts(context.Background())
		}
		return cached, nil
	}

	if opts.Cached {
		return cached, err
	}
	if client == nil || !s.healthy() {
		if err != nil {
			return nil, err
		}
		return cached, nil
	}
	return s.refreshContactsNow(ctx, client)
}

func (s *Server) contact(ctx context.Context, client *meshcore.Client, query string) (meshcore.Contact, error) {
	if s.store != nil {
		entry, err := s.store.Contact(ctx, s.uri, query)
		if err == nil {
			if client != nil && s.healthy() {
				go s.refreshContacts(context.Background())
			}
			return entry.Contact, nil
		}
		if err != nil && !IsNotFound(err) {
			return meshcore.Contact{}, err
		}
	}
	if client != nil && s.healthy() {
		ct, err := client.Contact(ctx, query)
		if err == nil {
			return ct, nil
		}
	}
	if s.store == nil {
		return meshcore.Contact{}, fmt.Errorf("backend contact cache is unavailable")
	}
	entry, err := s.store.Contact(ctx, s.uri, query)
	if err != nil {
		if IsNotFound(err) {
			return meshcore.Contact{}, fmt.Errorf("no cached contact matching %q", query)
		}
		return meshcore.Contact{}, err
	}
	return entry.Contact, nil
}

func (s *Server) refreshContactsNow(ctx context.Context, client *meshcore.Client) ([]meshcore.Contact, error) {
	contacts, err := client.Contacts(ctx)
	if err != nil {
		return nil, err
	}
	if s.store != nil {
		_ = s.store.UpsertContacts(ctx, s.uri, contacts)
	}
	return contacts, nil
}

func (s *Server) cachedContacts(ctx context.Context) ([]meshcore.Contact, error) {
	if s.store == nil {
		return nil, fmt.Errorf("backend contact cache is unavailable")
	}
	entries, err := s.store.Contacts(ctx, s.uri)
	if err != nil {
		return nil, err
	}
	contacts := make([]meshcore.Contact, len(entries))
	for i, entry := range entries {
		contacts[i] = entry.Contact
	}
	return contacts, nil
}

func (s *Server) repeaterLogin(ctx context.Context, client *meshcore.Client, repeater, password string) (meshcore.RepeaterSession, error) {
	session, err := client.RepeaterLogin(ctx, repeater, password)
	if err != nil {
		if s.store != nil {
			_ = s.store.ClearRepeaterSession(ctx, s.uri, repeater)
		}
		return meshcore.RepeaterSession{}, err
	}
	if s.store != nil {
		_ = s.store.UpsertRepeaterSession(ctx, s.uri, session)
	}
	return session, nil
}

func (s *Server) ensureRepeaterSession(ctx context.Context, client *meshcore.Client, repeater, password string) error {
	if s.store != nil {
		session, err := s.store.RepeaterSession(ctx, s.uri, repeater)
		if err == nil && session.Active() {
			return nil
		}
	}

	active, err := client.RepeaterHasConnection(ctx, repeater)
	if err == nil && active {
		if s.store != nil {
			ct, contactErr := s.contact(ctx, client, repeater)
			if contactErr == nil {
				_ = s.store.UpsertRepeaterSession(ctx, s.uri, meshcore.RepeaterSession{
					Repeater:   ct.Name,
					PublicKey:  ct.PublicKey,
					LoggedInAt: time.Now(),
					ExpiresAt:  time.Now().Add(30 * time.Minute),
				})
			}
		}
		return nil
	}

	if password == "" {
		return nil
	}
	_, err = s.repeaterLogin(ctx, client, repeater, password)
	return err
}

func (s *Server) keepAlive() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopped:
			return
		case <-ticker.C:
			if s.stateSnapshot() == stateBridge {
				continue
			}
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
		Bridges:   s.bridgeStatusLocked(),
	}
}

func (s *Server) markReady() {
	s.mu.Lock()
	s.state = stateReady
	s.lastSeen = time.Now()
	s.lastError = ""
	s.mu.Unlock()
	go s.refreshContacts(context.Background())
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

func (s *Server) stateSnapshot() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

func (s *Server) lastErr() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.lastError == "" {
		if s.state == stateBridge {
			return "bridge is active"
		}
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
	if s.stateSnapshot() == stateBridge {
		return
	}
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
	go s.refreshContacts(context.Background())
}

func (s *Server) markReconnectFailed(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = stateDegraded
	if err != nil {
		s.lastError = err.Error()
	}
}

func (s *Server) refreshContacts(ctx context.Context) {
	if s.store == nil {
		return
	}
	client := s.clientSnapshot()
	if client == nil || !s.healthy() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	contacts, err := client.Contacts(ctx)
	if err != nil {
		return
	}
	_ = s.store.UpsertContacts(ctx, s.uri, contacts)
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

func parseBackendHashPath(s string) ([]byte, bool) {
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
