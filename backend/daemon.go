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

	meshcore "github.com/meshcore-cz/meshcore-go"
)

// SessionProfile describes a logical device the daemon can run as an isolated
// DeviceSession. A registered profile is not necessarily running: autostart and
// `mc session start` control its lifecycle.
type SessionProfile struct {
	ID        string
	URI       string
	Bridges   []BridgeConfig
	Autostart bool
	DialOpts  []meshcore.DialOption
}

// DaemonOptions configures a backend Daemon.
type DaemonOptions struct {
	Socket      string
	Store       Store
	LogRequests bool
}

// Daemon supervises one or more isolated DeviceSessions behind a single Unix
// socket. It owns the listener, the shared replica store, and request routing;
// each session owns its own radio connection and state.
type Daemon struct {
	socket      string
	store       Store
	logRequests bool

	listener net.Listener
	stopOnce sync.Once
	stopped  chan struct{}

	startedAt time.Time

	mu        sync.RWMutex
	profiles  map[string]SessionProfile
	sessions  map[string]*DeviceSession
	order     []string
	defaultID string

	// startLocks serialises session dials per device id so that concurrent
	// start requests (e.g. autostart racing an explicit device_start) do not
	// open two connections to the same radio.
	startLocks map[string]*sync.Mutex
}

// NewDaemon prepares a daemon. If opts.Store is nil a default SQLite store is
// opened and owned by the daemon.
func NewDaemon(opts DaemonOptions) (*Daemon, error) {
	socket := opts.Socket
	if socket == "" {
		socket = SocketPath()
	}
	store := opts.Store
	if store == nil {
		var err error
		store, err = OpenSQLiteStore("")
		if err != nil {
			return nil, fmt.Errorf("opening backend store: %w", err)
		}
	}
	return &Daemon{
		socket:      socket,
		store:       store,
		logRequests: opts.LogRequests,
		stopped:     make(chan struct{}),
		profiles:    make(map[string]SessionProfile),
		sessions:    make(map[string]*DeviceSession),
		startLocks:  make(map[string]*sync.Mutex),
	}, nil
}

// startLock returns the per-device dial lock, creating it on first use.
func (d *Daemon) startLock(id string) *sync.Mutex {
	d.mu.Lock()
	defer d.mu.Unlock()
	lk, ok := d.startLocks[id]
	if !ok {
		lk = &sync.Mutex{}
		d.startLocks[id] = lk
	}
	return lk
}

// Socket returns the daemon's Unix socket path.
func (d *Daemon) Socket() string { return d.socket }

// Register records a session profile so it can be started on demand. The first
// registered profile becomes the default target unless one is set explicitly
// with SetDefault.
func (d *Daemon) Register(p SessionProfile) {
	if p.ID == "" {
		p.ID = sessionSlug(p.URI)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.profiles[p.ID]; !ok {
		d.order = append(d.order, p.ID)
	}
	d.profiles[p.ID] = p
	if d.defaultID == "" {
		d.defaultID = p.ID
	}
}

// SetDefault selects the default device targeted by requests with no explicit
// device.
func (d *Daemon) SetDefault(id string) {
	d.mu.Lock()
	d.defaultID = id
	d.mu.Unlock()
}

// Serve dials the default session, autostarts other configured sessions, and
// then accepts IPC connections until Stop is called. Failure to bring up the
// default session is fatal so that `mc backend start` reports it promptly.
func (d *Daemon) Serve() error {
	if err := os.MkdirAll(filepath.Dir(d.socket), 0o700); err != nil {
		return err
	}
	if err := os.Remove(d.socket); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	ln, err := net.Listen("unix", d.socket)
	if err != nil {
		return err
	}
	if err := os.Chmod(d.socket, 0o600); err != nil {
		ln.Close()
		return err
	}
	d.listener = ln
	d.startedAt = time.Now()

	// The daemon is the supervisor: it comes up independently of any radio.
	// Only devices marked autostart connect now; everything else stays stopped
	// until `mc session start` (or a direct/first-use connection).
	for _, id := range d.profileIDs() {
		p, ok := d.profile(id)
		if !ok || !p.Autostart {
			continue
		}
		go func(id string) {
			if _, err := d.startSession(context.Background(), id); err != nil {
				Logf("autostart device %q failed: %v", id, err)
			}
		}(id)
	}

	defer func() {
		ln.Close()
		os.Remove(d.socket)
		d.stopAllSessions()
		d.store.Close()
		close(d.stopped)
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-d.stopped:
				return nil
			default:
				if errors.Is(err, net.ErrClosed) {
					return nil
				}
				return err
			}
		}
		go d.handle(conn)
	}
}

// Stop closes the listener, which unwinds Serve and stops every session.
func (d *Daemon) Stop() {
	d.stopOnce.Do(func() {
		if d.listener != nil {
			d.listener.Close()
		}
	})
}

// DefaultID returns the current default device id.
func (d *Daemon) DefaultID() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.defaultID
}

func (d *Daemon) profile(id string) (SessionProfile, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	p, ok := d.profiles[id]
	return p, ok
}

func (d *Daemon) profileIDs() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return append([]string(nil), d.order...)
}

// startSession dials and starts the session for a registered profile. It is a
// no-op if the session is already running.
func (d *Daemon) startSession(ctx context.Context, id string) (*DeviceSession, error) {
	// Fast path: already running.
	d.mu.RLock()
	if s, ok := d.sessions[id]; ok {
		d.mu.RUnlock()
		return s, nil
	}
	d.mu.RUnlock()

	// Serialise dials for this device so concurrent starts (autostart racing an
	// explicit device_start) don't open two connections to the same radio.
	lk := d.startLock(id)
	lk.Lock()
	defer lk.Unlock()

	// Re-check after acquiring the dial lock; another caller may have finished.
	d.mu.RLock()
	s, running := d.sessions[id]
	p, known := d.profiles[id]
	d.mu.RUnlock()
	if running {
		return s, nil
	}
	if !known {
		return nil, fmt.Errorf("unknown device %q", id)
	}

	s, err := newSession(ctx, p.URI, SessionOptions{
		ID:      id,
		Store:   d.store,
		Bridges: p.Bridges,
	}, p.DialOpts...)
	if err != nil {
		return nil, err
	}

	d.mu.Lock()
	d.sessions[id] = s
	d.mu.Unlock()

	s.start()
	return s, nil
}

// stopSession stops a running session. The bool reports whether a running
// session was actually stopped (false if it was already stopped).
func (d *Daemon) stopSession(id string) (bool, error) {
	d.mu.Lock()
	s, ok := d.sessions[id]
	if ok {
		delete(d.sessions, id)
	}
	d.mu.Unlock()
	if !ok {
		if _, known := d.profile(id); !known {
			return false, fmt.Errorf("unknown device %q", id)
		}
		return false, nil
	}
	s.stop()
	return true, nil
}

// runningSession reports whether a session for id is currently running.
func (d *Daemon) runningSession(id string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	_, ok := d.sessions[id]
	return ok
}

func (d *Daemon) stopAllSessions() {
	d.mu.Lock()
	sessions := make([]*DeviceSession, 0, len(d.sessions))
	for _, s := range d.sessions {
		sessions = append(sessions, s)
	}
	d.sessions = make(map[string]*DeviceSession)
	d.mu.Unlock()
	for _, s := range sessions {
		s.stop()
	}
}

// session resolves the running session for a request. An empty id resolves to
// the default device.
func (d *Daemon) session(id string) (*DeviceSession, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if id == "" {
		id = d.defaultID
	}
	if id == "" {
		return nil, fmt.Errorf("no device selected")
	}
	if s, ok := d.sessions[id]; ok {
		return s, nil
	}
	if _, known := d.profiles[id]; known {
		return nil, fmt.Errorf("device %q is not running; run `mc session start %s`", id, id)
	}
	return nil, fmt.Errorf("unknown device %q", id)
}

func (d *Daemon) handle(conn net.Conn) {
	defer conn.Close()

	var req request
	enc := json.NewEncoder(conn)
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		if d.logRequests {
			Logf("ipc request decode error: %v", err)
		}
		_ = enc.Encode(response{OK: false, Error: err.Error()})
		return
	}
	start := time.Now()
	if d.logRequests {
		logIPCRequest(req.ID, req.Method, req.Params)
	}

	switch req.Method {
	case "stop":
		d.respond(enc, req, start, map[string]bool{"stopping": true}, nil)
		go d.Stop()
		return
	case "backend_status":
		d.respond(enc, req, start, d.backendStatus(), nil)
		return
	case "device_list":
		d.respond(enc, req, start, d.deviceList(), nil)
		return
	case "device_start":
		id := d.deviceArg(req)
		wasRunning := d.runningSession(id)
		_, err := d.startSession(context.Background(), id)
		d.respond(enc, req, start, deviceActionResult{Device: id, Running: err == nil, Changed: err == nil && !wasRunning}, err)
		return
	case "device_stop":
		id := d.deviceArg(req)
		stopped, err := d.stopSession(id)
		d.respond(enc, req, start, deviceActionResult{Device: id, Running: false, Changed: stopped}, err)
		return
	case "device_restart":
		id := d.deviceArg(req)
		if _, err := d.stopSession(id); err != nil {
			d.respond(enc, req, start, deviceActionResult{Device: id}, err)
			return
		}
		_, err := d.startSession(context.Background(), id)
		d.respond(enc, req, start, deviceActionResult{Device: id, Running: err == nil, Changed: err == nil}, err)
		return
	}

	sess, err := d.session(req.Device)
	if err != nil {
		d.respond(enc, req, start, nil, err)
		return
	}
	if sess.serve(conn, req, enc, start, d.logRequests) {
		go d.Stop()
	}
}

// deviceArg returns the device targeted by a device_* method, falling back to a
// "device" param and then the default device.
func (d *Daemon) deviceArg(req request) string {
	if req.Device != "" {
		return req.Device
	}
	var p deviceParams
	if len(req.Params) > 0 {
		_ = json.Unmarshal(req.Params, &p)
	}
	if p.Device != "" {
		return p.Device
	}
	return d.DefaultID()
}

func (d *Daemon) respond(enc *json.Encoder, req request, start time.Time, result any, err error) {
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
	if d.logRequests {
		logIPCResponse(req.ID, req.Method, err, time.Since(start))
	}
}

func (d *Daemon) backendStatus() daemonStatusResult {
	out := daemonStatusResult{
		Running:   true,
		PID:       os.Getpid(),
		StartedAt: d.startedAt,
		Version:   Version,
		DefaultID: d.DefaultID(),
		Devices:   d.deviceList().Devices,
	}
	if !d.startedAt.IsZero() {
		out.UptimeSec = int64(time.Since(d.startedAt).Seconds())
	}
	return out
}

func (d *Daemon) deviceList() deviceListResult {
	d.mu.RLock()
	ids := append([]string(nil), d.order...)
	defID := d.defaultID
	sessions := make(map[string]*DeviceSession, len(d.sessions))
	for id, s := range d.sessions {
		sessions[id] = s
	}
	profiles := make(map[string]SessionProfile, len(d.profiles))
	for id, p := range d.profiles {
		profiles[id] = p
	}
	d.mu.RUnlock()

	// List the default device first, then the rest in registration order.
	if defID != "" {
		ordered := make([]string, 0, len(ids))
		ordered = append(ordered, defID)
		for _, id := range ids {
			if id != defID {
				ordered = append(ordered, id)
			}
		}
		ids = ordered
	}

	out := deviceListResult{Devices: make([]deviceListEntry, 0, len(ids))}
	for _, id := range ids {
		if s, ok := sessions[id]; ok {
			out.Devices = append(out.Devices, s.listEntry(id == defID))
			continue
		}
		p := profiles[id]
		out.Devices = append(out.Devices, deviceListEntry{
			ID:        id,
			Default:   id == defID,
			Session:   "stopped",
			Connected: false,
			Transport: transportScheme(p.URI),
			URI:       p.URI,
		})
	}
	return out
}
