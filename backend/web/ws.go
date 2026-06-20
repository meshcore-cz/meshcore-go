package web

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/coder/websocket"
	meshcore "github.com/meshcore-cz/meshcore-go"
	localbackend "github.com/meshcore-cz/meshcore-go/backend"
)

// wsStatusInterval is how often a fresh status snapshot is pushed to connected
// dashboards.
const wsStatusInterval = 3 * time.Second

// wsEnvelope is the frame format sent to the browser: a type discriminator plus
// the payload. Event frames carry a backend.Event; status frames carry a
// statusSnapshot map.
type wsEnvelope struct {
	Type string `json:"type"` // "status" | "event" | "packet" | "rf"
	Data any    `json:"data"`
}

// handleWS upgrades to a WebSocket and streams live backend events (incoming
// messages, adverts, acks) alongside periodic status snapshots. Both come from
// the same daemon the REST API proxies to, so the socket needs no radio state of
// its own.
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// The server binds loopback by default; accept same-host origins.
		InsecureSkipVerify: true,
	})
	if err != nil {
		return
	}
	defer conn.CloseNow()

	// CloseRead cancels ctx when the client disconnects (or sends a frame),
	// which unwinds the event subscription and the ticker loop.
	ctx := conn.CloseRead(r.Context())

	device := r.URL.Query().Get("device")
	client := localbackend.NewClientForDevice(s.socket, device)

	// Event and raw-packet streams are best-effort: the daemon may have no
	// connected session, in which case the subscriptions error and we fall back
	// to status-only updates.
	var events <-chan localbackend.Event
	if ch, err := client.Watch(ctx); err == nil {
		events = ch
	}
	// Bidirectional companion-frame log (host↔radio) for the companion log.
	var packets <-chan meshcore.RawPacket
	if ch, err := client.WatchRaw(ctx); err == nil {
		packets = ch
	}
	// Unified over-the-air RF log (received + transmitted) for the RF log view.
	// RX carries SNR/RSSI, TX carries send priority; both decode with meshpkt.
	var rfPackets <-chan meshcore.RFPacket
	if ch, err := client.WatchRFLog(ctx); err == nil {
		rfPackets = ch
	}

	// Push an initial snapshot immediately so the UI populates without waiting a
	// full tick.
	s.pushStatus(ctx, conn, device)

	ticker := time.NewTicker(wsStatusInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.pushStatus(ctx, conn, device)
		case ev, ok := <-events:
			if !ok {
				events = nil // stream closed; keep pushing status
				continue
			}
			if !writeWS(ctx, conn, wsEnvelope{Type: "event", Data: ev}) {
				return
			}
		case pkt, ok := <-packets:
			if !ok {
				packets = nil // stream closed; keep pushing status
				continue
			}
			if !writeWS(ctx, conn, wsEnvelope{Type: "packet", Data: pkt}) {
				return
			}
		case rf, ok := <-rfPackets:
			if !ok {
				rfPackets = nil // stream closed; keep pushing status
				continue
			}
			if !writeWS(ctx, conn, wsEnvelope{Type: "rf", Data: rfEntry(rf)}) {
				return
			}
		}
	}
}

func (s *Server) pushStatus(ctx context.Context, conn *websocket.Conn, device string) {
	snapCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	snap, err := s.statusSnapshot(snapCtx, device)
	cancel()
	if err != nil {
		return
	}
	writeWS(ctx, conn, wsEnvelope{Type: "status", Data: snap})
}

func writeWS(ctx context.Context, conn *websocket.Conn, env wsEnvelope) bool {
	data, err := json.Marshal(env)
	if err != nil {
		return true
	}
	writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return conn.Write(writeCtx, websocket.MessageText, data) == nil
}
