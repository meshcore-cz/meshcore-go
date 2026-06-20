// Package web serves the optional embedded dashboard and JSON control API for a
// running backend daemon. It holds no radio state of its own: every request is
// a thin wrapper over backend.Client talking to the daemon's Unix socket, so the
// web server reuses the exact same control surface as the CLI.
package web

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	localbackend "github.com/meshcore-cz/meshcore-go/backend"
)

// Options configures a web Server.
type Options struct {
	// Socket is the daemon Unix socket the API proxies to.
	Socket string
	// Addr is the host:port the HTTP server binds.
	Addr string
}

// Server serves the dashboard and JSON API.
type Server struct {
	socket string
	http   *http.Server
}

// New builds a web Server. Call ListenAndServe to start it.
func New(opts Options) *Server {
	s := &Server{socket: opts.Socket}
	mux := http.NewServeMux()
	s.routes(mux)
	s.http = &http.Server{
		Addr:              opts.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return s
}

// ListenAndServe binds the address and serves until Shutdown is called. It
// returns http.ErrServerClosed on a clean shutdown.
func (s *Server) ListenAndServe() error { return s.http.ListenAndServe() }

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error { return s.http.Shutdown(ctx) }

func (s *Server) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("GET /api/devices", s.handleDevices)
	mux.HandleFunc("POST /api/devices/{id}/{action}", s.handleDeviceAction)
	mux.HandleFunc("GET /api/contacts", s.handleContacts)
	mux.HandleFunc("GET /api/channels", s.handleChannels)
	mux.HandleFunc("GET /api/messages", s.handleMessages)
	mux.HandleFunc("POST /api/send", s.handleSend)
	mux.HandleFunc("POST /api/send-channel", s.handleSendChannel)
	mux.HandleFunc("POST /api/advert", s.handleAdvert)
	mux.HandleFunc("POST /api/raw", s.handleRaw)
	mux.HandleFunc("POST /api/mesh-packet", s.handleMeshPacket)
	mux.HandleFunc("GET /ws", s.handleWS)
	mux.Handle("/", staticHandler())
}

// client returns a backend client routed to the device named by the optional
// "device" query parameter (empty targets the daemon's default device).
func (s *Server) client(r *http.Request) *localbackend.Client {
	return localbackend.NewClientForDevice(s.socket, r.URL.Query().Get("device"))
}

// reqContext bounds a request against radio operations that can take a while
// (e.g. send, advert) while still honouring client disconnects.
func reqContext(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), 120*time.Second)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	out, err := s.statusSnapshot(ctx, r.URL.Query().Get("device"))
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// statusSnapshot returns the daemon status plus the targeted device's session
// status (best-effort: the daemon may be up with no session connected). Shared
// by the REST status endpoint and the WebSocket status ticker.
func (s *Server) statusSnapshot(ctx context.Context, device string) (map[string]any, error) {
	daemon, err := localbackend.NewClient(s.socket).BackendStatus(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"daemon": daemon}
	if st, err := localbackend.NewClientForDevice(s.socket, device).Status(ctx); err == nil {
		out["device"] = st
	}
	return out, nil
}

func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	devices, err := localbackend.NewClient(s.socket).DeviceList(ctx)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": devices})
}

func (s *Server) handleDeviceAction(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	action := r.PathValue("action")
	ctx, cancel := reqContext(r)
	defer cancel()
	c := localbackend.NewClient(s.socket)
	var (
		res localbackend.DeviceActionResult
		err error
	)
	switch action {
	case "start":
		res, err = c.DeviceStart(ctx, id)
	case "stop":
		res, err = c.DeviceStop(ctx, id)
	case "restart":
		res, err = c.DeviceRestart(ctx, id)
	default:
		writeError(w, http.StatusBadRequest, fmt.Errorf("unknown device action %q", action))
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleContacts(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := reqContext(r)
	defer cancel()
	refresh := boolParam(r, "refresh")
	contacts, err := s.client(r).ContactsWithOptions(ctx, false, refresh, refresh, false)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"contacts": contacts})
}

func (s *Server) handleChannels(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := reqContext(r)
	defer cancel()
	channels, err := s.client(r).ChannelsWithOptions(ctx, boolParam(r, "refresh"))
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"channels": channels})
}

func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	limit := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		limit, _ = strconv.Atoi(v)
	}
	filter := localbackend.MessageFilter{
		Direction: r.URL.Query().Get("direction"),
		Kind:      r.URL.Query().Get("kind"),
		Peer:      r.URL.Query().Get("peer"),
		Channel:   r.URL.Query().Get("channel"),
		Limit:     limit,
	}
	msgs, err := s.client(r).Messages(ctx, filter)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": msgs})
}

func (s *Server) handleSend(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Recipient string `json:"recipient"`
		Text      string `json:"text"`
		Wait      bool   `json:"wait"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if body.Recipient == "" || body.Text == "" {
		writeError(w, http.StatusBadRequest, errors.New("recipient and text are required"))
		return
	}
	ctx, cancel := reqContext(r)
	defer cancel()
	c := s.client(r)
	receipt, err := c.SendText(ctx, body.Recipient, body.Text)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	out := map[string]any{"receipt": receipt}
	if body.Wait {
		ack, err := c.WaitForAcknowledgement(ctx, receipt)
		if err != nil {
			out["ack_error"] = err.Error()
		} else {
			out["ack"] = ack
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleSendChannel(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Channel string `json:"channel"`
		Text    string `json:"text"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if body.Channel == "" || body.Text == "" {
		writeError(w, http.StatusBadRequest, errors.New("channel and text are required"))
		return
	}
	ctx, cancel := reqContext(r)
	defer cancel()
	receipt, err := s.client(r).SendChannelText(ctx, body.Channel, body.Text)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"receipt": receipt})
}

func (s *Server) handleAdvert(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Flood bool `json:"flood"`
	}
	// Body is optional; ignore decode errors for an empty body.
	_ = json.NewDecoder(r.Body).Decode(&body)
	ctx, cancel := reqContext(r)
	defer cancel()
	if err := s.client(r).Advertise(ctx, body.Flood); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sent": true})
}

func (s *Server) handleRaw(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Payload string `json:"payload"` // hex
	}
	if !decodeBody(w, r, &body) {
		return
	}
	payload, err := decodeHex(body.Payload)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ctx, cancel := reqContext(r)
	defer cancel()
	res, err := s.client(r).RawSend(ctx, payload)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": res})
}

func (s *Server) handleMeshPacket(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Priority byte   `json:"priority"`
		Packet   string `json:"packet"` // hex
	}
	if !decodeBody(w, r, &body) {
		return
	}
	pkt, err := decodeHex(body.Packet)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ctx, cancel := reqContext(r)
	defer cancel()
	if err := s.client(r).SendMeshPacket(ctx, body.Priority, pkt); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sent": true})
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]any{"error": err.Error()})
}

func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
		return false
	}
	return true
}

func boolParam(r *http.Request, name string) bool {
	v := r.URL.Query().Get(name)
	return v == "1" || strings.EqualFold(v, "true")
}

func decodeHex(s string) ([]byte, error) {
	s = strings.ReplaceAll(strings.TrimSpace(s), " ", "")
	if s == "" {
		return nil, errors.New("empty hex payload")
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("invalid hex: %w", err)
	}
	return b, nil
}
