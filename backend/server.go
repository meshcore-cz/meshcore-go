package backend

import (
	"context"
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
	"github.com/meshcore-cz/meshcore-go/protocol/pathhash"
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

	mu                  sync.RWMutex
	state               string
	lastSeen            time.Time
	lastError           string
	bridgeStatuses      map[string]BridgeStatus
	contactSyncing      bool
	contactSyncRecv     int
	contactSyncTotal    int
	contactCount        int
	contactSyncedAt     time.Time
	contactError        string
	contactSyncMu       sync.Mutex
	refreshMu           sync.Mutex
	refreshCancel       context.CancelFunc
	channelSyncing      bool
	channelCount        int
	channelSyncedAt     time.Time
	channelError        string
	channelSyncMu       sync.Mutex
	radioMu             sync.Mutex // serialises live radio I/O
	radioActive         bool
	radioMethod         string
	radioSince          time.Time
	radioLastAt         time.Time
	radioLastMethod     string
	radioLastDurationMs int64
	deviceInfo          meshcore.DeviceInfo
	deviceInfoOK        bool
	deviceStats         meshcore.LocalStats
	deviceStatsOK       bool
	deviceStatsAt       time.Time

	startedAt   time.Time
	lastErrorAt time.Time
	logRequests bool
	stopOnce    sync.Once
	stopped     chan struct{}

	ipcClients        int32
	radioQueuePending int32
	reconnects        int32
	requestsOK        int64
	requestsFailed    int64
}

const (
	stateReady    = "ready"
	stateDegraded = "degraded"
	stateBridge   = "bridge"

	keepAliveInterval   = 30 * time.Second
	statsPollTimeout    = 15 * time.Second
	statsPollTimeoutBLE = 30 * time.Second

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
	s.startedAt = time.Now()
	s.refreshStartupCaches(context.Background(), client)
	return s, nil
}

// SetLogRequests enables logging of IPC requests and responses to the backend log.
func (s *Server) SetLogRequests(v bool) {
	s.logRequests = v
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
	go s.pollStats()
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
	s.trackIPCClient(1)
	defer s.trackIPCClient(-1)

	var req request
	enc := json.NewEncoder(conn)
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		if s.logRequests {
			Logf("ipc request decode error: %v", err)
		}
		_ = enc.Encode(response{OK: false, Error: err.Error()})
		s.trackRequestFailed()
		return
	}
	start := time.Now()
	if s.logRequests {
		logIPCRequest(req.ID, req.Method, req.Params)
	}
	streaming := req.Method == "watch" || req.Method == "watch_raw" || req.Method == "discover"
	if req.Method == "watch" {
		if !s.healthy() {
			_ = enc.Encode(response{ID: req.ID, OK: false, Error: "backend radio unavailable: " + s.lastErr()})
			s.trackRequestFailed()
			return
		}
		s.trackRequestOK()
		s.watch(conn, req.ID)
		return
	}
	if req.Method == "watch_raw" {
		if !s.healthy() {
			_ = enc.Encode(response{ID: req.ID, OK: false, Error: "backend radio unavailable: " + s.lastErr()})
			s.trackRequestFailed()
			return
		}
		s.trackRequestOK()
		s.watchRaw(conn, req.ID)
		return
	}
	if req.Method == "discover" {
		if !s.healthy() {
			_ = enc.Encode(response{ID: req.ID, OK: false, Error: "backend radio unavailable: " + s.lastErr()})
			s.trackRequestFailed()
			return
		}
		s.lockRadio("discover")
		defer s.unlockRadio()
		s.trackRequestOK()
		s.discover(conn, req.ID, req.Params)
		return
	}

	var result any
	var err error
	if s.methodUsesRadio(req.Method, req.Params) {
		s.interruptContactRefresh()
		s.lockRadio(req.Method)
		defer s.unlockRadio()
	}
	result, err = s.dispatch(connContext(), req.Method, req.Params)
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

	if resp.OK {
		s.trackRequestOK()
	} else {
		s.trackRequestFailed()
	}

	if s.logRequests && !streaming {
		logIPCResponse(req.ID, req.Method, err, time.Since(start))
	}

	if req.Method == "stop" && err == nil {
		go s.Stop()
	} else if err == nil && s.methodUsesRadio(req.Method, req.Params) && req.Method != "contacts" && req.Method != "channels" {
		s.scheduleContactRefreshAfterInteractive()
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
	events, cancel := client.SubscribeEvents(64)
	defer cancel()
	for ev := range events {
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
	raw, cancel := client.SubscribeRawPackets(256)
	defer cancel()
	for pkt := range raw {
		if err := enc.Encode(pkt); err != nil {
			return
		}
	}
}

func (s *Server) discover(conn net.Conn, id uint64, params json.RawMessage) {
	enc := json.NewEncoder(conn)
	if err := enc.Encode(response{ID: id, OK: true}); err != nil {
		return
	}
	client := s.clientSnapshot()
	if client == nil {
		return
	}

	var p discoverParams
	if len(params) > 0 {
		if err := json.Unmarshal(params, &p); err != nil {
			return
		}
	}
	opts := meshcore.NodeDiscoverOptions{
		Filter:     p.Filter,
		PrefixOnly: p.PrefixOnly,
	}
	if p.TimeoutMs > 0 {
		opts.Timeout = time.Duration(p.TimeoutMs) * time.Millisecond
	}

	Logf("radio send op=discover filter=0x%02x prefix_only=%t", p.Filter, p.PrefixOnly)
	nodes, err := client.DiscoverNodes(context.Background(), opts, func(n meshcore.DiscoveredNode) {
		_ = enc.Encode(n)
	})
	if err != nil {
		Logf("radio done op=discover error=%v", err)
		return
	}
	Logf("radio done op=discover nodes=%d", len(nodes))
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
	case "channels":
		var p channelsParams
		if len(params) > 0 {
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, err
			}
		}
		client := s.clientSnapshot()
		return s.channels(ctx, client, p)
	case "channel":
		var p queryParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		return s.channel(ctx, p.Query)
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
		if info, ok := s.deviceInfoSnapshot(); ok {
			return info, nil
		}
		info, err := client.DeviceInfo(ctx)
		if err != nil {
			return nil, err
		}
		s.storeDeviceInfo(info)
		return info, nil
	case "stats":
		var p statsParams
		if len(params) > 0 {
			_ = json.Unmarshal(params, &p)
		}
		if !p.Refresh {
			if stats, ok := s.deviceStatsSnapshot(); ok {
				return stats, nil
			}
		}
		stats, err := client.Stats(ctx)
		if err != nil {
			return nil, err
		}
		s.storeDeviceStats(stats)
		return stats, nil
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
		if pathhash.IsHexTraceTarget(p.Query) {
			return client.Trace(ctx, p.Query)
		}
		ct, err := s.contact(ctx, client, p.Query)
		if err != nil {
			return nil, err
		}
		return client.TraceContactWithHint(ctx, ct, 0)
	case "send_channel_text":
		var p channelSendParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		return client.SendChannelText(ctx, p.Channel, p.Text)
	case "advert":
		var p advertParams
		if len(params) > 0 {
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, err
			}
		}
		mode := "zero-hop"
		if p.Flood {
			mode = "flood"
		}
		Logf("radio send op=advert cmd=0x07 mode=%s", mode)
		if err := client.Advertise(ctx, p.Flood); err != nil {
			Logf("radio done op=advert mode=%s error=%v", mode, err)
			return nil, err
		}
		Logf("radio done op=advert mode=%s", mode)
		return map[string]bool{"sent": true}, nil
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
		Logf("radio send op=has_connection cmd=0x1c repeater=%q source=local_replica", p.Query)
		active, err := client.RepeaterHasConnectionContact(ctx, ct)
		if err != nil {
			Logf("radio done op=has_connection repeater=%q error=%v", p.Query, err)
		} else {
			Logf("radio done op=has_connection repeater=%q active=%t", p.Query, active)
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
		Logf("radio send op=send_login cmd=0x1a repeater=%q password_len=%d", p.Repeater, len(p.Password))
		session, err := s.repeaterLogin(ctx, client, p.Repeater, p.Password)
		if err != nil {
			Logf("radio done op=send_login repeater=%q error=%v", p.Repeater, err)
			return nil, err
		}
		Logf("radio done op=send_login repeater=%q expires_at=%s", p.Repeater, session.ExpiresAt.Format(time.RFC3339))
		return session, nil
	case "repeater_status":
		var p queryParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		statusStart := time.Now()
		Logf("repeater status start repeater=%q at=%s", p.Query, statusStart.Format(time.RFC3339))
		sessionStart := time.Now()
		if err := s.ensureRepeaterSession(ctx, client, p.Query, ""); err != nil {
			Logf("repeater status ensure_session failed repeater=%q elapsed=%s error=%v", p.Query, time.Since(sessionStart).Round(time.Millisecond), err)
			return nil, err
		}
		Logf("repeater status ensure_session ok repeater=%q elapsed=%s", p.Query, time.Since(sessionStart).Round(time.Millisecond))
		lookupStart := time.Now()
		ct, err := s.replicaContact(ctx, p.Query)
		Logf("repeater status contact_lookup repeater=%q source=local_replica elapsed=%s error=%v", p.Query, time.Since(lookupStart).Round(time.Millisecond), err)
		if err != nil {
			return nil, err
		}
		Logf("radio send op=send_status_req cmd=0x1b repeater=%q at=%s", p.Query, time.Now().Format(time.RFC3339))
		statusCallStart := time.Now()
		resp, err := client.RepeaterStatusContact(ctx, ct)
		Logf("repeater status radio elapsed=%s", time.Since(statusCallStart).Round(time.Millisecond))
		if err != nil {
			Logf("repeater status done repeater=%q elapsed=%s error=%v", p.Query, time.Since(statusStart).Round(time.Millisecond), err)
			return nil, err
		}
		Logf("repeater status done repeater=%q elapsed=%s stats=%t bytes=%d", p.Query, time.Since(statusStart).Round(time.Millisecond), resp.Stats != nil, len(resp.Text))
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

func (s *Server) contacts(ctx context.Context, client *meshcore.Client, opts contactsParams) (any, error) {
	if opts.Refresh && !opts.Wait {
		if client == nil || !s.healthy() {
			return nil, fmt.Errorf("backend radio unavailable: %s", s.lastErr())
		}
		return s.startContactRefresh(opts.Full), nil
	}
	if opts.Refresh {
		if client == nil || !s.healthy() {
			return nil, fmt.Errorf("backend radio unavailable: %s", s.lastErr())
		}
		return s.syncContacts(ctx, client, opts.Full)
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
		s.lockRadio("replicate")
		defer s.unlockRadio()

		ctx, cancel := context.WithTimeout(context.Background(), initialContactSyncTimeout)
		defer cancel()
		client := s.clientSnapshot()
		if client == nil || !s.healthy() {
			return
		}
		contacts, err := s.syncContacts(ctx, client, false)
		if err != nil {
			Logf("contact sync failed: %v", err)
		} else {
			Logf("contacts replicated: %d", len(contacts))
		}
		channels, err := s.syncChannels(ctx, client)
		if err != nil {
			Logf("channel sync failed: %v", err)
			return
		}
		Logf("channels replicated: %d", len(channels))
	}()
}

func (s *Server) syncContacts(ctx context.Context, client *meshcore.Client, full bool) ([]meshcore.Contact, error) {
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
	s.contactSyncRecv = 0
	s.contactSyncTotal = 0
	s.contactError = ""
	s.mu.Unlock()
	Logf("contacts sync started")
	defer s.finishContactSync()

	var since uint32
	if !full {
		mod, err := s.store.ContactLastMod(ctx, s.uri)
		if err != nil {
			s.recordContactSyncError(err)
			return nil, err
		}
		since = mod
	} else if err := s.store.ClearContacts(ctx, s.uri); err != nil {
		s.recordContactSyncError(err)
		return nil, err
	}

	result, err := client.ContactsSince(ctx, since, func(p meshcore.ContactSyncProgress) {
		s.mu.Lock()
		s.contactSyncRecv = p.Received
		s.contactSyncTotal = p.Total
		s.mu.Unlock()
	})
	if err != nil {
		s.recordContactSyncError(err)
		return nil, err
	}
	if len(result.Contacts) > 0 {
		if err := s.store.UpsertContacts(ctx, s.uri, result.Contacts); err != nil {
			s.recordContactSyncError(err)
			return nil, err
		}
	}
	if result.LastMod != 0 || full {
		if err := s.store.SetContactLastMod(ctx, s.uri, result.LastMod); err != nil {
			s.recordContactSyncError(err)
			return nil, err
		}
	}
	contacts, err := s.replicaContacts(ctx)
	if err != nil {
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

func (s *Server) syncChannels(ctx context.Context, client *meshcore.Client) ([]meshcore.Channel, error) {
	if s.store == nil {
		return nil, fmt.Errorf("backend channel replica is unavailable")
	}
	if client == nil {
		err := fmt.Errorf("backend radio unavailable: no active client")
		s.recordChannelSyncError(err)
		return nil, err
	}
	s.channelSyncMu.Lock()
	defer s.channelSyncMu.Unlock()
	s.mu.Lock()
	s.channelSyncing = true
	s.channelError = ""
	s.mu.Unlock()
	Logf("channels sync started")
	defer s.finishChannelSync()

	channels, err := client.Channels(ctx)
	if err != nil {
		s.recordChannelSyncError(err)
		return nil, err
	}
	if err := s.store.UpsertChannels(ctx, s.uri, channels); err != nil {
		s.recordChannelSyncError(err)
		return nil, err
	}
	s.mu.Lock()
	s.channelCount = len(channels)
	s.channelSyncedAt = time.Now()
	s.channelError = ""
	s.mu.Unlock()
	return channels, nil
}

func (s *Server) recordChannelSyncError(err error) {
	if err == nil {
		return
	}
	s.mu.Lock()
	s.channelError = err.Error()
	s.mu.Unlock()
}

func (s *Server) finishChannelSync() {
	s.mu.Lock()
	s.channelSyncing = false
	s.mu.Unlock()
}

func (s *Server) channels(ctx context.Context, client *meshcore.Client, opts channelsParams) ([]meshcore.Channel, error) {
	if opts.Refresh {
		if client == nil || !s.healthy() {
			return nil, fmt.Errorf("backend radio unavailable: %s", s.lastErr())
		}
		return s.syncChannels(ctx, client)
	}
	return s.replicaChannels(ctx)
}

func (s *Server) replicaChannels(ctx context.Context) ([]meshcore.Channel, error) {
	if s.store == nil {
		return nil, fmt.Errorf("backend channel replica is unavailable")
	}
	entries, err := s.store.Channels(ctx, s.uri)
	if err != nil {
		return nil, err
	}
	channels := make([]meshcore.Channel, len(entries))
	for i, entry := range entries {
		channels[i] = entry.Channel
	}
	return channels, nil
}

// channel returns a channel from the backend's local replica. Channels are
// synced from the radio explicitly via channels --refresh, not fetched per request.
func (s *Server) channel(ctx context.Context, query string) (meshcore.Channel, error) {
	if s.store == nil {
		return meshcore.Channel{}, fmt.Errorf("backend channel replica is unavailable")
	}
	entry, err := s.store.Channel(ctx, s.uri, query)
	if err != nil {
		if IsNotFound(err) {
			return meshcore.Channel{}, fmt.Errorf("no local channel matching %q; run `mc channel list --refresh` to sync from radio", query)
		}
		return meshcore.Channel{}, err
	}
	return entry.Channel, nil
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
	s.contactSyncRecv = 0
	s.contactSyncTotal = 0
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
		Logf("repeater session from config repeater=%q expires_at=%s", sess.Repeater, sess.ExpiresAt.Format(time.RFC3339))
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

func (s *Server) pollStats() {
	ticker := time.NewTicker(keepAliveInterval)
	defer ticker.Stop()

	s.runStatsPoll()
	for {
		select {
		case <-s.stopped:
			return
		case <-ticker.C:
			s.runStatsPoll()
		}
	}
}

func (s *Server) runStatsPoll() {
	if s.stateSnapshot() == stateBridge {
		return
	}
	if !s.tryLockRadio("stats") {
		return
	}
	if !s.healthy() {
		s.tryReconnect()
		s.unlockRadio()
		return
	}
	client := s.clientSnapshot()
	if client == nil {
		s.markDegraded(fmt.Errorf("no active client"))
		s.unlockRadio()
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.statsPollTimeout())
	stats, err := client.Stats(ctx)
	cancel()
	if err != nil {
		s.markDegraded(err)
		s.unlockRadio()
		return
	}
	s.storeDeviceStats(stats)
	s.markReady()
	s.unlockRadio()
}

func (s *Server) status() statusResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	transport := s.uri
	if s.client != nil {
		transport = s.client.Transport()
	}
	reqOK, reqFailed := s.requestCounts()
	out := statusResult{
		Running:           true,
		Healthy:           s.state == stateReady,
		State:             s.state,
		URI:               s.uri,
		Transport:         transport,
		PID:               os.Getpid(),
		StartedAt:         s.startedAt,
		LastSeen:          s.lastSeen,
		LastError:         s.lastError,
		LastErrorAt:       s.lastErrorAt,
		Bridges:           s.bridgeStatusLocked(),
		QueuePending:      s.radioQueueCount(),
		Reconnects:        s.reconnectCount(),
		Clients:           s.ipcClientCount(),
		RequestsCompleted: reqOK,
		RequestsFailed:    reqFailed,
		Version:           Version,
		Contacts: contactStatus{
			Syncing:      s.contactSyncing,
			SyncReceived: s.contactSyncRecv,
			SyncTotal:    s.contactSyncTotal,
			Count:        s.contactCount,
			SyncedAt:     s.contactSyncedAt,
			Error:        s.contactError,
		},
		Channels: channelStatus{
			Syncing:  s.channelSyncing,
			Count:    s.channelCount,
			SyncedAt: s.channelSyncedAt,
			Error:    s.channelError,
		},
		Radio: s.radioStatusLocked(),
	}
	if !s.startedAt.IsZero() {
		out.UptimeSec = int64(time.Since(s.startedAt).Seconds())
	}
	if snap := s.deviceStatusSnapshotLocked(); snap != nil {
		out.Device = snap
	}
	if stats := s.deviceStatsSnapshotLocked(); stats != nil {
		out.Stats = stats
		out.StatsAt = s.deviceStatsAt
	}
	return out
}

func (s *Server) markReady() {
	s.mu.Lock()
	s.state = stateReady
	s.lastSeen = time.Now()
	s.lastError = ""
	s.lastErrorAt = time.Time{}
	s.mu.Unlock()
}

func (s *Server) markDegraded(err error) {
	var old *meshcore.Client
	s.mu.Lock()
	s.state = stateDegraded
	if err != nil {
		s.lastError = err.Error()
		s.lastErrorAt = time.Now()
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
	s.trackReconnect()
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
	s.refreshStartupCaches(context.Background(), client)
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
		s.lastErrorAt = time.Now()
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

	existing, existingErr := s.store.Contact(ctx, s.uri, contact.PublicKey)
	if contact.Name == "" {
		if existingErr != nil {
			return
		}
		contact.Name = existing.Contact.Name
	}
	if contact.LastAdvert.IsZero() {
		contact.LastAdvert = time.Now()
	}
	if contact.Type == "" {
		contact.Type = meshcore.ContactUnknown
	}
	if existingErr == nil {
		if contact.Type == meshcore.ContactUnknown && existing.Contact.Type != "" {
			contact.Type = existing.Contact.Type
		}
		if contact.HasPath == false && existing.Contact.HasPath {
			contact.HasPath = existing.Contact.HasPath
			contact.OutPathEnc = existing.Contact.OutPathEnc
			contact.OutPath = existing.Contact.OutPath
		}
		if contact.Latitude == 0 && contact.Longitude == 0 {
			contact.Latitude = existing.Contact.Latitude
			contact.Longitude = existing.Contact.Longitude
		}
	}
	if err := s.store.UpsertContact(ctx, s.uri, contact); err != nil {
		s.recordContactSyncError(err)
		return
	}
	if contact.LastMod != 0 {
		if current, err := s.store.ContactLastMod(ctx, s.uri); err != nil {
			s.recordContactSyncError(err)
		} else if contact.LastMod > current {
			if err := s.store.SetContactLastMod(ctx, s.uri, contact.LastMod); err != nil {
				s.recordContactSyncError(err)
			}
		}
	}
	if contacts, err := s.replicaContacts(ctx); err == nil {
		s.mu.Lock()
		s.contactCount = len(contacts)
		s.mu.Unlock()
	}
}

func (s *Server) statsPollTimeout() time.Duration {
	if strings.HasPrefix(s.uri, "ble://") {
		return statsPollTimeoutBLE
	}
	return statsPollTimeout
}

func (s *Server) methodUsesRadio(method string, params json.RawMessage) bool {
	switch method {
	case "status", "stop":
		return false
	case "contacts":
		var p contactsParams
		if len(params) > 0 {
			_ = json.Unmarshal(params, &p)
		}
		return p.Refresh && p.Wait
	case "channels":
		var p channelsParams
		if len(params) > 0 {
			_ = json.Unmarshal(params, &p)
		}
		return p.Refresh
	case "contact", "channel", "device_info":
		return false
	case "stats":
		var p statsParams
		if len(params) > 0 {
			_ = json.Unmarshal(params, &p)
		}
		return p.Refresh
	default:
		return true
	}
}

func (s *Server) refreshDeviceInfoCache(ctx context.Context, client *meshcore.Client) {
	if client == nil {
		return
	}
	info, err := client.DeviceInfo(ctx)
	if err != nil {
		return
	}
	s.storeDeviceInfo(info)
}

func (s *Server) storeDeviceInfo(info meshcore.DeviceInfo) {
	s.mu.Lock()
	s.deviceInfo = info
	s.deviceInfoOK = info.Name != "" || info.PublicKey != ""
	s.mu.Unlock()
}

func (s *Server) deviceInfoSnapshot() (meshcore.DeviceInfo, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.deviceInfoOK {
		return meshcore.DeviceInfo{}, false
	}
	return s.deviceInfo, true
}

func (s *Server) deviceStatusSnapshotLocked() *deviceStatusSnapshot {
	if !s.deviceInfoOK {
		return nil
	}
	info := s.deviceInfo
	return &deviceStatusSnapshot{
		Name:            info.Name,
		PublicKey:       info.PublicKey,
		Firmware:        info.FirmwareName,
		FirmwareVersion: info.FirmwareVersion,
		Protocol:        info.ProtocolVersion,
		Capabilities:    info.Capabilities.List(),
		RadioFreqKHz:    info.RadioFreqKHz,
		RadioBWKHz:      info.RadioBWKHz,
		RadioSF:         info.RadioSF,
		RadioCR:         info.RadioCR,
		TxPowerDBm:      info.TxPowerDBm,
		Latitude:        info.Latitude,
		Longitude:       info.Longitude,
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
