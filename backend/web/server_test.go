package web

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// fakeDaemon is a minimal IPC server speaking the backend wire protocol: one
// JSON request per connection, one JSON response back. It lets the web layer be
// tested without a real radio or daemon.
type fakeDaemon struct {
	t       *testing.T
	ln      net.Listener
	replies map[string]any // method -> result value (nil => ok with no result)
	fail    map[string]string
}

func startFakeDaemon(t *testing.T) *fakeDaemon {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "mc.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	d := &fakeDaemon{t: t, ln: ln, replies: map[string]any{}, fail: map[string]string{}}
	go d.serve()
	t.Cleanup(func() { ln.Close() })
	return d
}

func (d *fakeDaemon) socket() string { return d.ln.Addr().String() }

func (d *fakeDaemon) serve() {
	for {
		conn, err := d.ln.Accept()
		if err != nil {
			return
		}
		go d.handle(conn)
	}
}

func (d *fakeDaemon) handle(conn net.Conn) {
	defer conn.Close()
	var req struct {
		ID     uint64          `json:"id"`
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		return
	}
	resp := map[string]any{"id": req.ID, "ok": true}
	if msg, ok := d.fail[req.Method]; ok {
		resp["ok"] = false
		resp["error"] = msg
	} else if result, ok := d.replies[req.Method]; ok && result != nil {
		raw, _ := json.Marshal(result)
		resp["result"] = json.RawMessage(raw)
	}
	_ = json.NewEncoder(conn).Encode(resp)
}

func newTestServer(t *testing.T, d *fakeDaemon) http.Handler {
	t.Helper()
	return New(Options{Socket: d.socket()}).http.Handler
}

func TestStatusEndpoint(t *testing.T) {
	d := startFakeDaemon(t)
	d.replies["backend_status"] = map[string]any{"running": true, "pid": 4242, "version": "test"}
	d.fail["status"] = "no session" // device session best-effort omitted

	srv := httptest.NewServer(newTestServer(t, d))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/status")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	var out struct {
		Daemon struct {
			Running bool
			PID     int
		}
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !out.Daemon.Running || out.Daemon.PID != 4242 {
		t.Fatalf("unexpected daemon status: %+v", out.Daemon)
	}
}

func TestRawEndpoint(t *testing.T) {
	d := startFakeDaemon(t)
	d.replies["raw_send"] = map[string]any{"type": "raw", "code": 1}

	srv := httptest.NewServer(newTestServer(t, d))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/raw", "application/json",
		strings.NewReader(`{"payload":"deadbeef"}`))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status %d: %s", resp.StatusCode, body)
	}
	var out struct {
		Result struct {
			Type string
		}
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Result.Type != "raw" {
		t.Fatalf("unexpected raw result: %+v", out.Result)
	}
}

func TestRawEndpointRejectsBadHex(t *testing.T) {
	d := startFakeDaemon(t)
	srv := httptest.NewServer(newTestServer(t, d))
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/raw", "application/json",
		strings.NewReader(`{"payload":"xyz"}`))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestStaticSPAFallback(t *testing.T) {
	srv := httptest.NewServer(staticHandler())
	defer srv.Close()

	for _, path := range []string{"/", "/some/client/route"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("get %s: %v", path, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("get %s: status %d", path, resp.StatusCode)
		}
		if !strings.Contains(string(body), "<html") {
			t.Fatalf("get %s: expected SPA shell, got %.80q", path, body)
		}
	}
}
