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

	meshcore "github.com/meshcore-cz/meshcore-go"
	"github.com/meshcore-cz/meshcore-go/protocol"
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

	mu              sync.RWMutex
	state           string
	lastSeen        time.Time
	lastError       string
	bridgeStatuses  map[string]BridgeStatus
	contactSyncing  bool
	contactCount    int
	contactSyncedAt time.Time
	contactError    string
	contactSyncMu   sync.Mutex
	radioMu         sync.Mutex // serialises radio IPC, contact sync, and keepalive probes

	stopOnce sync.Once
	stopped  chan struct{}
}

const (
	stateReady    = "ready"
	stateDegraded = "degraded"
	stateBridge   = "bridge"

	keepAliveInterval = 30 * time.Second
	keepAliveTimeout  = 8 * time.Second

	initialContactSyncTimeout = 90 * time.Second
)

// NewServer dials uri and prepares a local backend server.
func NewServer(ctx context.Context, uri string, opts ...meshcore.DialOption) (*Server, error) {
	return NewServerWithBridges(ctx, uri, nil, opts...)
}

// NewServerWithBridges dials uri and prepares a backend with configured bridge
// listeners.
func NewServerWithBridges(ctx context.Context, uri string, bridges []BridgeConfig, opts ...meshcore.DialOption) (*Server, error) {
	store, err := OpenSQLiteStore("")
	if err != nil {
		return nil, fmt.Errorf("opening backend store: %w", err)
	}
	s := &Server{
		uri:      uri,
		socket:   SocketPath(),
		opts:     append([]meshcore.DialOption(nil), opts...),
		bridges:  append([]BridgeConfig(nil), bridges...),
		store:    store,
		state:    stateReady,
		lastSeen: time.Now(),
		stopped:  make(chan struct{}),
	}
	client, err := s.dialClient(ctx)
	if err != nil {
		store.Close()
		return nil, fmt.Errorf("connecting to %s: %w", uri, err)
	}
	s.client = client
	return s, nil
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
	s.scheduleInitialContactSync()
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
		s.radioMu.Lock()
		defer s.radioMu.Unlock()
		s.watch(conn, req.ID)
		return
	}
	if req.Method == "watch_raw" {
		if !s.healthy() {
			_ = enc.Encode(response{ID: req.ID, OK: false, Error: "backend radio unavailable: " + s.lastErr()})
			return
		}
		s.radioMu.Lock()
		defer s.radioMu.Unlock()
		s.watchRaw(conn, req.ID)
		return
	}

	var result any
	var err error
	if req.Method == "status" || req.Method == "stop" {
		result, err = s.dispatch(connContext(), req.Method, req.Params)
	} else {
		s.radioMu.Lock()
		defer s.radioMu.Unlock()
		result, err = s.dispatch(connContext(), req.Method, req.Params)
	}
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
		if err != nil {
			return nil, err
		}
		return client.SendTextToContact(ctx, ct, p.Text)
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
			if err != nil {
				return nil, err
			}
			return client.TraceContact(ctx, ct)
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
		ct, err := s.replicaContact(ctx, p.Query)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(os.Stderr, "mc backend: radio send op=has_connection cmd=0x1c repeater=%q source=local_replica\n", p.Query)
		active, err := client.RepeaterHasConnectionContact(ctx, ct)
		if err != nil {
			fmt.Fprintf(os.Stderr, "mc backend: radio done op=has_connection repeater=%q error=%v\n", p.Query, err)
		} else {
			fmt.Fprintf(os.Stderr, "mc backend: radio done op=has_connection repeater=%q active=%t\n", p.Query, active)
		}
		if err != nil {
			return nil, err
		}
		return repeaterHasConnectionResult{Active: active}, nil
	case "repeater_login":
		var p repeaterLoginParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		fmt.Fprintf(os.Stderr, "mc backend: radio send op=send_login cmd=0x1a repeater=%q password_len=%d\n", p.Repeater, len(p.Password))
		session, err := s.repeaterLogin(ctx, client, p.Repeater, p.Password)
		if err != nil {
			fmt.Fprintf(os.Stderr, "mc backend: radio done op=send_login repeater=%q error=%v\n", p.Repeater, err)
			return nil, err
		}
		fmt.Fprintf(os.Stderr, "mc backend: radio done op=send_login repeater=%q expires_at=%s\n", p.Repeater, session.ExpiresAt.Format(time.RFC3339))
		return session, nil
	case "repeater_status":
		var p queryParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		statusStart := time.Now()
		fmt.Fprintf(os.Stderr, "mc backend: repeater status start repeater=%q at=%s\n", p.Query, statusStart.Format(time.RFC3339))
		sessionStart := time.Now()
		if err := s.ensureRepeaterSession(ctx, client, p.Query, ""); err != nil {
			fmt.Fprintf(os.Stderr, "mc backend: repeater status ensure_session failed repeater=%q elapsed=%s error=%v\n", p.Query, time.Since(sessionStart).Round(time.Millisecond), err)
			return nil, err
		}
		fmt.Fprintf(os.Stderr, "mc backend: repeater status ensure_session ok repeater=%q elapsed=%s\n", p.Query, time.Since(sessionStart).Round(time.Millisecond))
		lookupStart := time.Now()
		ct, err := s.replicaContact(ctx, p.Query)
		fmt.Fprintf(os.Stderr, "mc backend: repeater status contact_lookup repeater=%q source=local_replica elapsed=%s error=%v\n", p.Query, time.Since(lookupStart).Round(time.Millisecond), err)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(os.Stderr, "mc backend: radio send op=send_status_req cmd=0x1b repeater=%q at=%s\n", p.Query, time.Now().Format(time.RFC3339))
		statusCallStart := time.Now()
		resp, err := client.RepeaterStatusContact(ctx, ct)
		fmt.Fprintf(os.Stderr, "mc backend: repeater status radio elapsed=%s\n", time.Since(statusCallStart).Round(time.Millisecond))
		if err != nil {
			fmt.Fprintf(os.Stderr, "mc backend: repeater status done repeater=%q elapsed=%s error=%v\n", p.Query, time.Since(statusStart).Round(time.Millisecond), err)
			return nil, err
		}
		fmt.Fprintf(os.Stderr, "mc backend: repeater status done repeater=%q elapsed=%s stats=%t bytes=%d\n", p.Query, time.Since(statusStart).Round(time.Millisecond), resp.Stats != nil, len(resp.Text))
		return resp, nil
	case "repeater_neighbours":
		var p queryParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		if err := s.ensureRepeaterSession(ctx, client, p.Query, ""); err != nil {
			return nil, err
		}
		ct, err := s.replicaContact(ctx, p.Query)
		if err != nil {
			return nil, err
		}
		return client.RepeaterNeighboursContact(ctx, ct)
	case "repeater_exec":
		var p repeaterExecParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		if err := s.ensureRepeaterSession(ctx, client, p.Repeater, ""); err != nil {
			return nil, err
		}
		ct, err := s.replicaContact(ctx, p.Repeater)
		if err != nil {
			return nil, err
		}
		return client.RepeaterExecContact(ctx, ct, p.Command)
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
	return s.replicaContacts(ctx)
}

// contact returns a contact from the backend's local replica. Contacts are
// synced from the radio explicitly via contacts --refresh, not fetched per request.
func (s *Server) contact(ctx context.Context, client *meshcore.Client, query string) (meshcore.Contact, error) {
	_ = client
	if s.store == nil {
		return meshcore.Contact{}, fmt.Errorf("backend contact replica is unavailable")
	}
	entry, err := s.store.Contact(ctx, s.uri, query)
	if err != nil {
		if IsNotFound(err) {
			return meshcore.Contact{}, fmt.Errorf("no local contact matching %q; run `mc contacts --refresh` to sync from radio", query)
		}
		return meshcore.Contact{}, err
	}
	return entry.Contact, nil
}

func (s *Server) refreshContactsNow(ctx context.Context, client *meshcore.Client) ([]meshcore.Contact, error) {
	return s.syncContacts(ctx, client)
}

func (s *Server) replicaContacts(ctx context.Context) ([]meshcore.Contact, error) {
	if s.store == nil {
		return nil, fmt.Errorf("backend contact replica is unavailable")
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

func (s *Server) scheduleInitialContactSync() {
	go func() {
		s.radioMu.Lock()
		defer s.radioMu.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), initialContactSyncTimeout)
		defer cancel()
		client := s.clientSnapshot()
		if client == nil || !s.healthy() {
			return
		}
		contacts, err := s.syncContacts(ctx, client)
		if err != nil {
			fmt.Fprintf(os.Stderr, "mc backend: contact sync failed: %v\n", err)
			return
		}
		fmt.Fprintf(os.Stderr, "mc backend: contacts replicated: %d\n", len(contacts))
	}()
}

func (s *Server) syncContacts(ctx context.Context, client *meshcore.Client) ([]meshcore.Contact, error) {
	if s.store == nil {
		return nil, fmt.Errorf("backend contact replica is unavailable")
	}
	if client == nil {
		err := fmt.Errorf("backend radio unavailable: no active client")
		s.recordContactSyncError(err)
		return nil, err
	}
	s.contactSyncMu.Lock()
	defer s.contactSyncMu.Unlock()
	s.mu.Lock()
	s.contactSyncing = true
	s.contactError = ""
	s.mu.Unlock()
	defer s.finishContactSync()

	contacts, err := client.Contacts(ctx)
	if err != nil {
		s.recordContactSyncError(err)
		return nil, err
	}
	if err := s.store.UpsertContacts(ctx, s.uri, contacts); err != nil {
		s.recordContactSyncError(err)
		return nil, err
	}
	s.mu.Lock()
	s.contactCount = len(contacts)
	s.contactSyncedAt = time.Now()
	s.contactError = ""
	s.mu.Unlock()
	return contacts, nil
}

func (s *Server) recordContactSyncError(err error) {
	if err == nil {
		return
	}
	s.mu.Lock()
	s.contactError = err.Error()
	s.mu.Unlock()
}

func (s *Server) finishContactSync() {
	s.mu.Lock()
	s.contactSyncing = false
	s.mu.Unlock()
}

func (s *Server) repeaterLogin(ctx context.Context, client *meshcore.Client, repeater, password string) (meshcore.RepeaterSession, error) {
	ct, err := s.replicaContact(ctx, repeater)
	if err != nil {
		return meshcore.RepeaterSession{}, err
	}
	session, err := client.RepeaterLoginContact(ctx, ct, password)
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

func (s *Server) replicaContact(ctx context.Context, query string) (meshcore.Contact, error) {
	return s.contact(ctx, nil, query)
}

func (s *Server) ensureRepeaterSession(ctx context.Context, client *meshcore.Client, repeater, password string) error {
	if s.store != nil {
		session, err := s.store.RepeaterSession(ctx, s.uri, repeater)
		if err == nil && session.Active() {
			return nil
		}
	}
	if sess, ok := loadConfigRepeaterSession(repeater); ok {
		fmt.Fprintf(os.Stderr, "mc backend: repeater session from config repeater=%q expires_at=%s\n", sess.Repeater, sess.ExpiresAt.Format(time.RFC3339))
		if s.store != nil {
			_ = s.store.UpsertRepeaterSession(ctx, s.uri, sess)
		}
		return nil
	}

	ct, contactErr := s.replicaContact(ctx, repeater)
	if contactErr != nil {
		if password == "" {
			return nil
		}
		_, err := s.repeaterLogin(ctx, client, repeater, password)
		return err
	}
	active, err := client.RepeaterHasConnectionContact(ctx, ct)
	if err == nil && active {
		if s.store != nil {
			_ = s.store.UpsertRepeaterSession(ctx, s.uri, meshcore.RepeaterSession{
				Repeater:   ct.Name,
				PublicKey:  ct.PublicKey,
				LoggedInAt: time.Now(),
				ExpiresAt:  time.Now().Add(30 * time.Minute),
			})
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
	ticker := time.NewTicker(keepAliveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopped:
			return
		case <-ticker.C:
			if s.stateSnapshot() == stateBridge {
				continue
			}
			if !s.radioMu.TryLock() {
				continue
			}
			if !s.healthy() {
				s.tryReconnect()
				s.radioMu.Unlock()
				continue
			}
			client := s.clientSnapshot()
			if client == nil {
				s.markDegraded(fmt.Errorf("no active client"))
				s.radioMu.Unlock()
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), s.keepAliveTimeout())
			_, err := client.DeviceTime(ctx)
			cancel()
			if err != nil {
				s.markDegraded(err)
				s.radioMu.Unlock()
				continue
			}
			s.markReady()
			s.radioMu.Unlock()
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
		Contacts: contactStatus{
			Syncing:  s.contactSyncing,
			Count:    s.contactCount,
			SyncedAt: s.contactSyncedAt,
			Error:    s.contactError,
		},
	}
}

func (s *Server) markReady() {
	s.mu.Lock()
	s.state = stateReady
	s.lastSeen = time.Now()
	s.lastError = ""
	s.mu.Unlock()
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
	client, err := s.dialClient(ctx)
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

func (s *Server) dialClient(ctx context.Context) (*meshcore.Client, error) {
	opts := append([]meshcore.DialOption(nil), s.opts...)
	opts = append(opts, meshcore.WithClientOptions(meshcore.WithEventHook(s.observeEvent)))
	return meshcore.Dial(ctx, s.uri, opts...)
}

func (s *Server) markReconnectFailed(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = stateDegraded
	if err != nil {
		s.lastError = err.Error()
	}
}

func (s *Server) observeEvent(ev meshcore.Event) {
	adv, ok := ev.(meshcore.AdvertisementReceived)
	if !ok || s.store == nil || adv.Contact.PublicKey == "" {
		return
	}
	go s.upsertObservedContact(adv.Contact)
}

func (s *Server) upsertObservedContact(contact meshcore.Contact) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	contact.LastAdvert = time.Now()
	if contact.Type == "" {
		contact.Type = meshcore.ContactUnknown
	}
	if existing, err := s.store.Contact(ctx, s.uri, contact.PublicKey); err == nil {
		if contact.Name == "" {
			contact.Name = existing.Contact.Name
		}
		if contact.Type == meshcore.ContactUnknown && existing.Contact.Type != "" {
			contact.Type = existing.Contact.Type
		}
		contact.HasPath = existing.Contact.HasPath
		contact.Latitude = existing.Contact.Latitude
		contact.Longitude = existing.Contact.Longitude
	}
	if err := s.store.UpsertContact(ctx, s.uri, contact); err != nil {
		s.recordContactSyncError(err)
		return
	}
	if contacts, err := s.replicaContacts(ctx); err == nil {
		s.mu.Lock()
		s.contactCount = len(contacts)
		s.mu.Unlock()
	}
}

func (s *Server) keepAliveTimeout() time.Duration {
	if strings.HasPrefix(s.uri, "ble://") {
		return 20 * time.Second
	}
	return keepAliveTimeout
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
