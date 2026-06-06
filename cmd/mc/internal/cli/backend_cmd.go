package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	meshcore "github.com/meshcore-cz/meshcore-go"
	localbackend "github.com/meshcore-cz/meshcore-go/backend"
	"github.com/meshcore-cz/meshcore-go/cmd/mc/internal/config"
)

const backendContactSyncTimeout = 90 * time.Second

func cmdBackend(ctx context.Context, e *env) error {
	switch e.restArg(0) {
	case "start":
		return backendStart(ctx, e)
	case "restart":
		return backendRestart(ctx, e)
	case "stop":
		return backendStop(ctx, e)
	case "", "status":
		return backendStatusCmd(ctx, e)
	case "serve":
		return backendServe(ctx, e)
	default:
		return fmt.Errorf("unknown backend subcommand %q", e.restArg(0))
	}
}

func backendStart(ctx context.Context, e *env) error {
	if st, ok := backendStatus(ctx); ok {
		e.out.Human("Backend already running for %s (pid %d).\n", st.URI, st.PID)
		return e.out.JSONValue(backendStatusJSON(st))
	}

	uri, _, err := resolveURI(e)
	if err != nil {
		return err
	}
	return backendStartURI(ctx, e, uri)
}

func backendStartURI(ctx context.Context, e *env, uri string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	logPath := localbackend.LogPath()
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return err
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer logFile.Close()

	args := []string{"backend", "serve", "--uri", uri}
	if e.dbg.Enabled() {
		args = append(args, "--debug")
	}
	cmd := exec.Command(exe, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := startBackground(cmd); err != nil {
		return err
	}
	_ = cmd.Process.Release()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if st, ok := backendStatus(ctx); ok {
			e.out.Human("Backend started for %s (pid %d).\n", st.URI, st.PID)
			e.out.Human("Socket: %s\n", st.Socket)
			e.out.Human("Log:    %s\n", logPath)
			syncBackendContacts(ctx, e)
			if synced, ok := backendStatus(ctx); ok {
				st = synced
			}
			return e.out.JSONValue(backendStatusJSON(st))
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("backend did not become ready; see %s", logPath)
}

func backendStop(ctx context.Context, e *env) error {
	client := localbackend.NewClient("")
	st, err := client.Status(ctx)
	if err != nil {
		e.out.Human("Backend is not running.\n")
		return e.out.JSONValue(map[string]any{"running": false, "socket": client.Socket()})
	}
	st, stopped, err := stopBackend(ctx)
	if err != nil {
		return err
	}
	e.out.Human("Stopped backend for %s (pid %d).\n", st.URI, st.PID)
	return e.out.JSONValue(map[string]any{"running": false, "stopped": stopped, "pid": st.PID, "uri": st.URI})
}

func backendRestart(ctx context.Context, e *env) error {
	running, ok := backendStatus(ctx)
	uri := ""
	if e.args.flag("uri") != "" || e.args.flag("device") != "" || !ok {
		resolved, _, err := resolveURI(e)
		if err != nil {
			return err
		}
		uri = resolved
	} else {
		uri = running.URI
	}

	if ok {
		e.out.Human("Stopping backend for %s (pid %d)...\n", running.URI, running.PID)
		if _, _, err := stopBackend(ctx); err != nil {
			return err
		}
	} else {
		e.out.Human("Backend is not running; starting a new backend.\n")
	}

	return backendStartURI(ctx, e, uri)
}

func backendStatusCmd(ctx context.Context, e *env) error {
	st, ok := backendStatus(ctx)
	if !ok {
		e.out.Human("Backend: not running\n")
		return e.out.JSONValue(map[string]any{"running": false, "socket": localbackend.SocketPath()})
	}
	e.out.Human("Backend:  running\n")
	e.out.Human("State:    %s\n", st.State)
	e.out.Human("PID:      %d\n", st.PID)
	e.out.Human("Endpoint: %s\n", st.URI)
	e.out.Human("Transport: %s\n", st.Transport)
	e.out.Human("Socket:   %s\n", st.Socket)
	printContactStatus(e, st)
	for _, bridge := range st.Bridges {
		switch bridge.Type {
		case "tcp":
			e.out.Human("Bridge:   tcp %s active=%t\n", bridge.Listen, bridge.Active)
		case "pty":
			e.out.Human("Bridge:   pty %s active=%t\n", bridge.Path, bridge.Active)
		default:
			e.out.Human("Bridge:   %s active=%t\n", bridge.Type, bridge.Active)
		}
		if bridge.Error != "" {
			e.out.Human("Bridge err: %s\n", bridge.Error)
		}
		if bridge.Note != "" {
			e.out.Human("Bridge note: %s\n", bridge.Note)
		}
	}
	if st.LastError != "" {
		e.out.Human("Last err: %s\n", st.LastError)
	}
	return e.out.JSONValue(backendStatusJSON(st))
}

func backendServe(ctx context.Context, e *env) error {
	uri, _, err := resolveURI(e)
	if err != nil {
		return err
	}
	opts := append(e.dbg.DialOptions(), meshcore.WithClientOptions(
		meshcore.WithMessageSync(),
		meshcore.WithTimeout(backendContactSyncTimeout),
	))
	bridges, err := configuredBridges()
	if err != nil {
		return err
	}
	server, err := localbackend.NewServerWithBridges(ctx, uri, bridges, opts...)
	if err != nil {
		return err
	}
	return server.Serve()
}

func stopBackend(ctx context.Context) (localbackend.Status, bool, error) {
	client := localbackend.NewClient("")
	st, err := client.Status(ctx)
	if err != nil {
		return st, false, err
	}
	if err := client.Stop(ctx); err != nil {
		return st, false, err
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := backendStatus(ctx); !ok {
			return st, true, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return st, false, fmt.Errorf("backend did not stop")
}

func backendStatus(ctx context.Context) (localbackend.Status, bool) {
	st, err := localbackend.NewClient("").Status(ctx)
	if err != nil {
		return localbackend.Status{Running: false, Socket: localbackend.SocketPath()}, false
	}
	return st, true
}

func backendStatusJSON(st localbackend.Status) map[string]any {
	return map[string]any{
		"running":    st.Running,
		"healthy":    st.Healthy,
		"state":      st.State,
		"pid":        st.PID,
		"uri":        st.URI,
		"transport":  st.Transport,
		"socket":     st.Socket,
		"last_seen":  st.LastSeen,
		"last_error": st.LastError,
		"bridges":    st.Bridges,
		"contacts": map[string]any{
			"syncing":   st.Contacts.Syncing,
			"count":     st.Contacts.Count,
			"synced_at": st.Contacts.SyncedAt,
			"error":     st.Contacts.Error,
		},
	}
}

func syncBackendContacts(ctx context.Context, e *env) {
	e.out.Human("Syncing contacts...\n")
	syncCtx, cancel := context.WithTimeout(ctx, backendContactSyncTimeout)
	defer cancel()
	contacts, err := localbackend.NewClient("").ContactsWithOptions(syncCtx, false, true)
	if err != nil {
		e.out.Human("Contact sync failed: %v\n", err)
		return
	}
	e.out.Human("Contacts synced: %d\n", len(contacts))
}

func printContactStatus(e *env, st localbackend.Status) {
	if st.Contacts.Syncing {
		e.out.Human("Contacts: syncing")
	} else if !st.Contacts.SyncedAt.IsZero() {
		e.out.Human("Contacts: %d synced at %s", st.Contacts.Count, st.Contacts.SyncedAt.Format("2006-01-02 15:04:05"))
	} else {
		e.out.Human("Contacts: not synced")
	}
	if st.Contacts.Error != "" {
		e.out.Human(" (error: %s)", st.Contacts.Error)
	}
	e.out.Human("\n")
}

func configuredBridges() ([]localbackend.BridgeConfig, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	out := make([]localbackend.BridgeConfig, 0, len(cfg.Backend.Bridges))
	for _, bridge := range cfg.Backend.Bridges {
		out = append(out, localbackend.BridgeConfig{
			Enabled: bridge.Enabled,
			Type:    bridge.Type,
			Listen:  bridge.Listen,
			Name:    bridge.Name,
		})
	}
	return out, nil
}
