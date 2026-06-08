package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/meshcore-cz/meshcore-go/cmd/mc/internal/config"
	"github.com/meshcore-cz/meshcore-go/cmd/mc/internal/output"
)

func TestSplitShellFields(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "plain",
			in:   "status",
			want: []string{"status"},
		},
		{
			name: "quoted message",
			in:   `send alice "hello there" --wait`,
			want: []string{"send", "alice", "hello there", "--wait"},
		},
		{
			name: "single quoted channel",
			in:   `channel send Public 'hello mesh'`,
			want: []string{"channel", "send", "Public", "hello mesh"},
		},
		{
			name: "escaped space",
			in:   `send alice hello\ there`,
			want: []string{"send", "alice", "hello there"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := splitShellFields(tt.in)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("splitShellFields(%q) = %#v, want %#v", tt.in, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("splitShellFields(%q) = %#v, want %#v", tt.in, got, tt.want)
				}
			}
		})
	}
}

func TestSplitShellFieldsErrors(t *testing.T) {
	for _, in := range []string{`send alice "unterminated`, `send alice trailing\`} {
		if _, err := splitShellFields(in); err == nil {
			t.Fatalf("splitShellFields(%q) succeeded, want error", in)
		}
	}
}

func TestInheritShellArgs(t *testing.T) {
	parent := parsedArgs{flags: map[string]string{}, bools: map[string]bool{"debug": true, "json": true}}

	got := inheritShellArgs([]string{"status"}, parent)
	want := []string{"status", "--debug", "--json"}
	if len(got) != len(want) {
		t.Fatalf("inheritShellArgs() = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("inheritShellArgs() = %#v, want %#v", got, want)
		}
	}

	child := inheritShellArgs([]string{"status", "--json"}, parent)
	if len(child) != 3 || child[0] != "status" || child[1] != "--json" || child[2] != "--debug" {
		t.Fatalf("inheritShellArgs dedup = %#v", child)
	}
}

func TestResolveBackendSocket(t *testing.T) {
	e := &env{exec: ExecuteOptions{BackendSocket: "/tmp/custom.sock"}}
	if got := resolveBackendSocket(e); got != "/tmp/custom.sock" {
		t.Fatalf("resolveBackendSocket() = %q, want custom socket", got)
	}
}

func TestExecuteNestedShellRejected(t *testing.T) {
	err := Execute(context.Background(), []string{"shell"}, ExecuteOptions{InShell: true})
	if err == nil || !strings.Contains(err.Error(), "already running inside mc shell") {
		t.Fatalf("Execute() = %v, want nested shell error", err)
	}
}

func TestExecuteAliasDispatch(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "b.sock")
	for _, args := range [][]string{{"s"}, {"c"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			err := Execute(context.Background(), args, ExecuteOptions{
				BackendSocket: socket,
				RequireIPC:    true,
				InShell:       true,
			})
			if err == nil || !strings.Contains(err.Error(), "shell backend unavailable") {
				t.Fatalf("Execute(%v) = %v, want shell backend unavailable", args, err)
			}
		})
	}
}

func TestExecuteAliasBackendStatusWithoutIPC(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "b.sock")
	err := Execute(context.Background(), []string{"b", "status"}, ExecuteOptions{
		BackendSocket: socket,
		RequireIPC:    true,
		InShell:       true,
	})
	if err != nil {
		t.Fatalf("Execute(b status) = %v, want nil", err)
	}
}

func TestExecuteAliasRepeaterListWithoutBackend(t *testing.T) {
	err := Execute(context.Background(), []string{"rep", "list"}, ExecuteOptions{
		RequireIPC: true,
		InShell:    true,
	})
	if err != nil {
		t.Fatalf("Execute(rep list) = %v, want nil", err)
	}
}

func TestExecuteHelpTrace(t *testing.T) {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	err = Execute(context.Background(), []string{"help", "trace"}, ExecuteOptions{InShell: true})

	w.Close()
	out, _ := io.ReadAll(r)
	os.Stdout = old

	if err != nil {
		t.Fatalf("Execute(help trace) = %v", err)
	}
	if !strings.Contains(string(out), "mc trace") {
		t.Fatalf("help trace output = %q, want trace help", out)
	}
}

func TestTemporaryBackendLifecycleBlocked(t *testing.T) {
	for _, sub := range []string{"stop", "restart", "reset", "serve", "start"} {
		t.Run(sub, func(t *testing.T) {
			pa := parsedArgs{flags: map[string]string{}, bools: map[string]bool{}, positionals: []string{"backend", sub}}
			e := &env{
				args: pa,
				rest: pa.positionals[1:],
				out:  newTestPrinter(),
				exec: ExecuteOptions{TemporaryShellBackend: true},
			}
			err := cmdBackend(context.Background(), e)
			if err == nil || !strings.Contains(err.Error(), "temporary shell backend") {
				t.Fatalf("cmdBackend(%s) = %v, want temporary backend guard", sub, err)
			}
		})
	}
}

func TestOpenShellSessionUsesExistingBackend(t *testing.T) {
	withTestConfig(t, "handheld", "serial:///dev/ttyTEST0", func(t *testing.T) {
		socket, stop := startFakeBackend(t, fakeBackendStatus{
			Healthy: true,
			URI:     "serial:///dev/ttyTEST0",
			PID:     4242,
		})
		defer stop()
		t.Setenv("MC_BACKEND_SOCKET", socket)

		pa := parsedArgs{flags: map[string]string{}, bools: map[string]bool{}, positionals: []string{"shell"}}
		e := &env{args: pa, out: newTestPrinter(), dbg: newDebug(pa)}

		session, err := openShellSession(context.Background(), e)
		if err != nil {
			t.Fatal(err)
		}
		defer session.close()

		if session.Temporary {
			t.Fatal("expected attach to existing backend, got temporary session")
		}
		if session.Profile != "handheld" {
			t.Fatalf("profile = %q, want handheld", session.Profile)
		}
		if session.Socket != socket {
			t.Fatalf("socket = %q, want %q", session.Socket, socket)
		}
	})
}

func TestWaitForShellBackend(t *testing.T) {
	socket, stop := startFakeBackend(t, fakeBackendStatus{Healthy: true})
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan error, 1)
	if err := waitForShellBackend(ctx, "serial:///dev/ttyTEST0", socket, done); err != nil {
		t.Fatalf("waitForShellBackend() = %v", err)
	}
}

func TestTemporaryBackendSocketRemovedOnClose(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "mc-shell-test-*")
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(tmpDir, "backend.sock")

	session := &shellSession{
		Profile:   "handheld",
		Socket:    socket,
		Temporary: true,
		cleanup: func() {
			os.RemoveAll(tmpDir)
		},
	}

	if _, err := os.Create(socket); err != nil {
		t.Fatal(err)
	}
	session.close()

	if _, err := os.Stat(tmpDir); !os.IsNotExist(err) {
		t.Fatalf("temp dir still exists after close: %v", err)
	}
}

func TestCompletionWords(t *testing.T) {
	got := completionWords(`send alice "hello`)
	if len(got) != 3 || got[0] != "send" || got[1] != "alice" || got[2] != "hello" {
		t.Fatalf("completionWords() = %#v", got)
	}
}

func TestFindCobraCommandAlias(t *testing.T) {
	cmd, ok := findCobraCommand(NewRoot(&App{}), "s")
	if !ok || cmd.Name() != "status" {
		t.Fatalf("findCobraCommand(s) = %#v, %v", cmd, ok)
	}
}

func TestQuoteCompletion(t *testing.T) {
	if got := quoteCompletion(`hello world`); got != `"hello world"` {
		t.Fatalf("quoteCompletion() = %q", got)
	}
}

func TestIsTerminalNoiseLine(t *testing.T) {
	if !isTerminalNoiseLine("^[[38;19R") {
		t.Fatal("expected CPR display form to be noise")
	}
	if !isTerminalNoiseLine("\x1b[38;19R") {
		t.Fatal("expected CPR escape form to be noise")
	}
	if isTerminalNoiseLine("status") {
		t.Fatal("status should not be noise")
	}
}

func TestRunShellExitReturns(t *testing.T) {
	in := strings.NewReader("status\nexit\n")
	parent := parsedArgs{flags: map[string]string{}, bools: map[string]bool{"json": true}, positionals: []string{"shell"}}
	e := &env{args: parent, out: newTestPrinter(), dbg: newDebug(parent)}
	session := &shellSession{Profile: "handheld", Socket: "/nonexistent.sock"}

	err := runShell(context.Background(), e, session, in)
	if err != nil {
		t.Fatalf("runShell() = %v", err)
	}
}

type fakeBackendStatus struct {
	Healthy   bool
	URI       string
	Transport string
	PID       int
}

func startFakeBackend(t *testing.T, st fakeBackendStatus) (string, func()) {
	t.Helper()

	dir, err := os.MkdirTemp("/tmp", "mcb")
	if err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(dir, "b.sock")
	ln, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go handleFakeBackendConn(conn, st)
		}
	}()

	return socket, func() {
		ln.Close()
		<-done
		os.RemoveAll(dir)
	}
}

func handleFakeBackendConn(conn net.Conn, st fakeBackendStatus) {
	defer conn.Close()

	dec := json.NewDecoder(conn)
	enc := json.NewEncoder(conn)

	var req struct {
		ID     uint64 `json:"id"`
		Method string `json:"method"`
	}
	if err := dec.Decode(&req); err != nil {
		return
	}

	switch req.Method {
	case "status":
		result := map[string]any{
			"running":   true,
			"healthy":   st.Healthy,
			"state":     "ready",
			"uri":       st.URI,
			"transport": st.Transport,
			"pid":       st.PID,
		}
		payload, _ := json.Marshal(result)
		_ = enc.Encode(map[string]any{
			"id":     req.ID,
			"ok":     true,
			"result": json.RawMessage(payload),
		})
	default:
		_ = enc.Encode(map[string]any{
			"id":    req.ID,
			"ok":    false,
			"error": "unsupported method",
		})
	}
}

func withTestConfig(t *testing.T, profile, uri string, fn func(t *testing.T)) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg := &config.Config{
		Version: 1,
		Current: profile,
		Devices: map[string]config.Device{
			profile: {
				Name:               "Test Radio",
				PublicKeyPrefix:    "EFF01EF2",
				PreferredTransport: "serial",
				Transports: []config.Endpoint{
					{URI: uri},
				},
			},
		},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	fn(t)
}

func newTestPrinter() *output.Printer {
	p := output.New(false)
	p.Out = &bytes.Buffer{}
	return p
}
