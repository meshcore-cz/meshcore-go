package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	meshcore "github.com/meshcore-cz/meshcore-go"
	localbackend "github.com/meshcore-cz/meshcore-go/backend"
	"github.com/meshcore-cz/meshcore-go/cmd/mc/internal/config"
	"github.com/meshcore-cz/meshcore-go/cmd/mc/internal/ui"
)

const (
	backendContactSyncTimeout = 90 * time.Second
	backendReadySerial        = 15 * time.Second
	backendReadyBLE           = 45 * time.Second
	backendReadyDefault       = 30 * time.Second
)

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
	case "log", "logs":
		return backendLog(ctx, e)
	case "reset":
		return backendReset(ctx, e)
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
	fmt.Fprintf(logFile, "\n--- backend start %s uri=%s pid=%d ---\n", time.Now().Format(time.RFC3339), uri, cmd.Process.Pid)
	_ = logFile.Sync()

	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()

	deadline := time.Now().Add(backendReadyTimeout(uri))
	for time.Now().Before(deadline) {
		if st, ok := backendStatus(ctx); ok && st.Healthy {
			printBackendStartSummary(e, st)
			return e.out.JSONValue(backendStatusJSON(st))
		}
		select {
		case err := <-waitDone:
			return backendStartFailed(uri, fmt.Errorf("backend exited: %w", err))
		default:
		}
		time.Sleep(100 * time.Millisecond)
	}
	return backendStartFailed(uri, fmt.Errorf("timed out after %s", backendReadyTimeout(uri)))
}

func backendReadyTimeout(uri string) time.Duration {
	switch schemeOf(uri) {
	case "ble":
		return backendReadyBLE
	case "serial":
		return backendReadySerial
	default:
		return backendReadyDefault
	}
}

func backendStartFailed(uri string, err error) error {
	msg := fmt.Sprintf("backend did not become ready: %v; see `mc backend log`", err)
	if schemeOf(uri) == "ble" {
		msg += " (BLE connect can take up to ~45s)"
	}
	return fmt.Errorf("%s", msg)
}

const defaultBackendLogLines = 100

func backendLog(ctx context.Context, e *env) error {
	path := localbackend.LogPath()
	if e.args.has("follow") || e.args.has("f") {
		return backendLogFollow(ctx, e, path)
	}

	lines, err := backendLogLineCount(e)
	if err != nil {
		return err
	}
	content, err := readLogLines(path, lines)
	if err != nil {
		if os.IsNotExist(err) {
			if e.out.JSON {
				return e.out.JSONValue(map[string]any{"path": path, "lines": []string{}, "exists": false})
			}
			e.out.Human("Log file not found: %s\n", path)
			return nil
		}
		return err
	}

	if e.out.JSON {
		return e.out.JSONValue(map[string]any{
			"path":   path,
			"lines":  content,
			"exists": true,
			"count":  len(content),
		})
	}
	for _, line := range content {
		e.out.Human("%s\n", line)
	}
	return nil
}

func backendLogLineCount(e *env) (int, error) {
	raw := e.args.flag("lines")
	if raw == "" {
		raw = e.args.flag("n")
	}
	if raw == "" {
		return defaultBackendLogLines, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("invalid log line count %q", raw)
	}
	return n, nil
}

func backendLogFollow(ctx context.Context, e *env, path string) error {
	if e.out.JSON {
		return fmt.Errorf("backend log --follow does not support --json")
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			e.out.Human("Log file not found: %s\n", path)
			return nil
		}
		return err
	}
	defer f.Close()

	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return err
	}
	reader := bufio.NewReader(f)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				return err
			}
			time.Sleep(200 * time.Millisecond)
			continue
		}
		e.out.Human("%s", strings.TrimSuffix(line, "\n")+"\n")
	}
}

func readLogLines(path string, maxLines int) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := strings.TrimRight(string(data), "\n")
	if text == "" {
		return nil, nil
	}
	all := strings.Split(text, "\n")
	if len(all) <= maxLines {
		return all, nil
	}
	return all[len(all)-maxLines:], nil
}

func backendReset(ctx context.Context, e *env) error {
	var stopped bool
	var stopStatus localbackend.Status
	if st, ok := backendStatus(ctx); ok {
		e.out.Human("Stopping backend for %s (pid %d)...\n", st.URI, st.PID)
		var err error
		stopStatus, stopped, err = stopBackend(ctx)
		if err != nil {
			return err
		}
		if !stopped {
			return fmt.Errorf("backend did not stop")
		}
	}

	removed, err := resetBackendState(e)
	if err != nil {
		return err
	}

	if e.out.JSON {
		out := map[string]any{
			"stopped": stopped,
			"removed": removed,
		}
		if stopped {
			out["uri"] = stopStatus.URI
			out["pid"] = stopStatus.PID
		}
		return e.out.JSONValue(out)
	}
	return nil
}

func resetBackendState(e *env) ([]string, error) {
	removed, err := localbackend.ResetState()
	if err != nil {
		return nil, err
	}
	if e.out.JSON {
		return removed, nil
	}
	if len(removed) == 0 {
		e.out.Human("No backend state files found.\n")
		return removed, nil
	}
	e.out.Human("Removed backend state:\n")
	for _, path := range removed {
		e.out.Human("  %s\n", path)
	}
	return removed, nil
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
	} else if !e.args.has("reset") {
		e.out.Human("Backend is not running; starting a new backend.\n")
	}

	if e.args.has("reset") {
		if _, err := resetBackendState(e); err != nil {
			return err
		}
	}

	return backendStartURI(ctx, e, uri)
}

func backendStatusCmd(ctx context.Context, e *env) error {
	st, ok := backendStatus(ctx)
	if !ok {
		e.out.Human("Backend: not running\n")
		return e.out.JSONValue(map[string]any{"running": false, "socket": localbackend.SocketPath()})
	}
	if !e.out.JSON {
		data := backendStatusDataFromStatus(st, e.args.has("verbose"))
		printer := ui.NewPrinter(e.out.Out)
		printer.Print(ui.RenderBackendStatus(data, printer))
	}
	return e.out.JSONValue(backendStatusJSON(st))
}

func backendServe(ctx context.Context, e *env) error {
	uri, _, err := resolveURI(e)
	if err != nil {
		return err
	}
	localbackend.Logf("connecting to %s", uri)
	opts := append(e.dbg.DialOptions(), meshcore.WithClientOptions(
		meshcore.WithMessageSync(),
		meshcore.WithTimeout(backendContactSyncTimeout),
	))
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	server, err := localbackend.NewServerWithBridges(ctx, uri, bridgesFromConfig(cfg), opts...)
	if err != nil {
		localbackend.Logf("connect failed: %v", err)
		return err
	}
	if cfg.Backend.LogRequests {
		server.SetLogRequests(true)
		localbackend.Logf("ipc request logging enabled")
	}
	localbackend.Logf("ready on %s", localbackend.SocketPath())
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
	out := map[string]any{
		"running":            st.Running,
		"healthy":            st.Healthy,
		"state":              st.State,
		"pid":                st.PID,
		"started_at":         st.StartedAt,
		"uptime_sec":         st.UptimeSec,
		"uri":                st.URI,
		"transport":          st.Transport,
		"socket":             st.Socket,
		"last_seen":          st.LastSeen,
		"last_error":         st.LastError,
		"last_error_at":      st.LastErrorAt,
		"bridges":            st.Bridges,
		"contacts":           contactStatusJSON(st.Contacts),
		"channels":           channelStatusJSON(st.Channels),
		"radio":              radioStatusJSON(st.Radio),
		"queue_pending":      st.QueuePending,
		"reconnects":         st.Reconnects,
		"clients":            st.Clients,
		"requests_completed": st.RequestsCompleted,
		"requests_failed":    st.RequestsFailed,
		"version":            st.Version,
	}
	if st.Device.Available() {
		out["device"] = map[string]any{
			"name":             st.Device.Name,
			"public_key":       st.Device.PublicKey,
			"firmware":         st.Device.Firmware,
			"firmware_version": st.Device.FirmwareVersion,
			"protocol":         st.Device.Protocol,
			"capabilities":     st.Device.Capabilities,
		}
	}
	if st.StatsOK {
		out["stats"] = st.Stats
		out["stats_at"] = st.StatsAt
	}
	return out
}

func contactStatusJSON(cs localbackend.ContactStatus) map[string]any {
	out := map[string]any{
		"syncing":   cs.Syncing,
		"count":     cs.Count,
		"synced_at": cs.SyncedAt,
		"error":     cs.Error,
	}
	if cs.Syncing {
		out["sync_received"] = cs.SyncReceived
		if cs.SyncTotal > 0 {
			out["sync_total"] = cs.SyncTotal
			out["sync_percent"] = contactSyncPercent(cs.SyncReceived, cs.SyncTotal)
		}
	}
	return out
}

func contactSyncPercent(received, total int) int {
	if total <= 0 || received <= 0 {
		return 0
	}
	if received >= total {
		return 100
	}
	return received * 100 / total
}

func formatContactSyncStatus(cs localbackend.ContactStatus) string {
	if cs.SyncTotal > 0 {
		return fmt.Sprintf("replicating %d%% (%d/%d)", contactSyncPercent(cs.SyncReceived, cs.SyncTotal), cs.SyncReceived, cs.SyncTotal)
	}
	if cs.SyncReceived > 0 {
		return fmt.Sprintf("replicating (%d/?)", cs.SyncReceived)
	}
	return "replicating"
}

func printBackendStartSummary(e *env, st localbackend.Status) {
	replicating := st.Contacts.Syncing || st.Channels.Syncing ||
		(st.Contacts.SyncedAt.IsZero() && st.Channels.SyncedAt.IsZero())
	if replicating {
		e.out.Human("Backend started for %s (pid %d). Replication of contacts and channels started.\n", st.URI, st.PID)
	} else {
		e.out.Human("Backend started for %s (pid %d). Contacts and channels replicated.\n", st.URI, st.PID)
	}
	e.out.Human("Socket: %s\n", st.Socket)
}

func channelStatusJSON(cs localbackend.ChannelStatus) map[string]any {
	return map[string]any{
		"syncing":   cs.Syncing,
		"count":     cs.Count,
		"synced_at": cs.SyncedAt,
		"error":     cs.Error,
	}
}

func radioStatusJSON(r localbackend.RadioStatus) map[string]any {
	out := map[string]any{
		"active": r.Active,
		"idle":   r.Idle || !r.Active,
	}
	if r.Method != "" {
		out["method"] = r.Method
	}
	if !r.Since.IsZero() {
		out["since"] = r.Since
	}
	if r.DurationMs > 0 {
		out["duration_ms"] = r.DurationMs
	}
	if !r.LastAt.IsZero() {
		out["last_at"] = r.LastAt
	}
	if r.LastMethod != "" {
		out["last_method"] = r.LastMethod
	}
	if r.LastDurationMs > 0 {
		out["last_duration_ms"] = r.LastDurationMs
	}
	return out
}

func configuredBridges() ([]localbackend.BridgeConfig, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	return bridgesFromConfig(cfg), nil
}

func bridgesFromConfig(cfg *config.Config) []localbackend.BridgeConfig {
	out := make([]localbackend.BridgeConfig, 0, len(cfg.Backend.Bridges))
	for _, bridge := range cfg.Backend.Bridges {
		out = append(out, localbackend.BridgeConfig{
			Enabled: bridge.Enabled,
			Type:    bridge.Type,
			Listen:  bridge.Listen,
			Name:    bridge.Name,
		})
	}
	return out
}
