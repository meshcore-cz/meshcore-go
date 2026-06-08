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
	sub := e.restArg(0)
	if e.exec.TemporaryShellBackend {
		switch sub {
		case "start", "restart", "stop", "reset", "serve":
			return fmt.Errorf("cannot %s the temporary shell backend; use `exit`", sub)
		}
	}

	switch sub {
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

// daemonReadyTimeout bounds how long we wait for the supervisor to start
// listening. The daemon binds its socket immediately; device sessions connect
// in the background, so this stays short regardless of transport.
const daemonReadyTimeout = 10 * time.Second

func backendStart(ctx context.Context, e *env) error {
	if st, ok := daemonRunning(ctx, e); ok {
		e.out.Human("Backend already running (pid %d).\n", st.PID)
		return e.out.JSONValue(daemonStatusJSON(st))
	}
	return launchBackend(ctx, e)
}

// launchBackend spawns the backend daemon, waits for the supervisor to come up,
// and prints the human/JSON start summary.
func launchBackend(ctx context.Context, e *env) error {
	st, err := spawnDaemon(ctx, e)
	if err != nil {
		return err
	}
	printBackendStartSummary(e, st)
	return e.out.JSONValue(daemonStatusJSON(st))
}

// spawnDaemon starts the backend daemon process and waits for the supervisor to
// begin listening. It does not wait for device sessions to connect — those
// autostart in the background (or start on demand). An explicit --uri/--device
// is forwarded so that device's session is started immediately as the default.
// It prints nothing, so callers can compose their own messaging.
func spawnDaemon(ctx context.Context, e *env) (localbackend.DaemonStatus, error) {
	exe, err := os.Executable()
	if err != nil {
		return localbackend.DaemonStatus{}, err
	}
	logPath := localbackend.LogPath()
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return localbackend.DaemonStatus{}, err
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return localbackend.DaemonStatus{}, err
	}
	defer logFile.Close()

	args := []string{"backend", "serve"}
	if u := e.args.flag("uri"); u != "" {
		args = append(args, "--uri", u)
	}
	if d := e.args.flag("device"); d != "" {
		args = append(args, "--device", d)
	}
	if e.dbg.Enabled() {
		args = append(args, "--debug")
	}
	cmd := exec.Command(exe, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := startBackground(cmd); err != nil {
		return localbackend.DaemonStatus{}, err
	}
	fmt.Fprintf(logFile, "\n--- backend start %s pid=%d ---\n", time.Now().Format(time.RFC3339), cmd.Process.Pid)
	_ = logFile.Sync()

	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()

	deadline := time.Now().Add(daemonReadyTimeout)
	for time.Now().Before(deadline) {
		if st, ok := daemonRunning(ctx, e); ok {
			return st, nil
		}
		select {
		case err := <-waitDone:
			return localbackend.DaemonStatus{}, backendStartFailed("", fmt.Errorf("backend exited: %w", err))
		default:
		}
		time.Sleep(100 * time.Millisecond)
	}
	return localbackend.DaemonStatus{}, backendStartFailed("", fmt.Errorf("timed out after %s", daemonReadyTimeout))
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
	var stopStatus localbackend.DaemonStatus
	if st, ok := daemonRunning(ctx, e); ok {
		e.out.Human("Stopping backend (pid %d)...\n", st.PID)
		var err error
		stopStatus, stopped, err = stopBackend(ctx, e)
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
	if _, ok := daemonRunning(ctx, e); !ok {
		e.out.Human("Backend is not running.\n")
		return e.out.JSONValue(map[string]any{"running": false, "socket": resolveBackendSocket(e)})
	}
	st, stopped, err := stopBackend(ctx, e)
	if err != nil {
		return err
	}
	e.out.Human("Stopped backend (pid %d).\n", st.PID)
	return e.out.JSONValue(map[string]any{"running": false, "stopped": stopped, "pid": st.PID})
}

func backendRestart(ctx context.Context, e *env) error {
	running, ok := daemonRunning(ctx, e)
	if ok {
		e.out.Human("Stopping backend (pid %d)...\n", running.PID)
		if _, _, err := stopBackend(ctx, e); err != nil {
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

	return launchBackend(ctx, e)
}

func backendStatusCmd(ctx context.Context, e *env) error {
	dst, ok := daemonRunning(ctx, e)
	if !ok {
		if !e.out.JSON {
			data := ui.BackendStatusData{Running: false, Socket: resolveBackendSocket(e)}
			printer := ui.NewPrinter(e.out.Out)
			printer.Print(ui.RenderBackendStatus(data, printer))
			return nil
		}
		return e.out.JSONValue(map[string]any{"running": false, "socket": resolveBackendSocket(e)})
	}

	if !e.out.JSON {
		data := backendStatusDataFromDaemon(ctx, e, dst, e.args.has("verbose"))
		printer := ui.NewPrinter(e.out.Out)
		printer.Print(ui.RenderBackendStatus(data, printer))
		return nil
	}

	out := daemonStatusJSON(dst)
	sessions := backendSessionDetails(ctx, e, dst, e.args.has("verbose"))
	out["sessions"] = backendSessionsStatusJSON(sessions)
	if len(sessions) > 0 {
		out["clients"] = sumBackendSessionClients(sessions)
		ok, failed, pending := sumBackendSessionRequests(sessions)
		out["requests_completed"] = ok
		out["requests_failed"] = failed
		out["requests_pending"] = pending
	}
	return e.out.JSONValue(out)
}

func printDaemonOnlyStatus(e *env, dst localbackend.DaemonStatus) {
	e.out.Human("Backend:\n")
	e.out.Human("  State:       running\n")
	if dst.PID > 0 {
		e.out.Human("  PID:         %d\n", dst.PID)
	}
	if dst.UptimeSec > 0 {
		e.out.Human("  Uptime:      %s\n", ui.FormatDurationSecs(uint32(dst.UptimeSec)))
	}
	if dst.Socket != "" {
		e.out.Human("  Socket:      %s\n", dst.Socket)
	}
	e.out.Human("  No device session connected. Run `mc session start <name>` or `mc status`.\n")
}

func sessionsSummary(entries map[string]localbackend.DeviceListEntry) (ready, degraded, stopped, freshReplicas int) {
	for _, en := range entries {
		switch en.Session {
		case "ready", "bridge":
			ready++
		case "degraded":
			degraded++
		case "stopped":
			stopped++
		}
		if en.Replica == "fresh" {
			freshReplicas++
		}
	}
	return
}

func printBackendSessionsSummary(e *env, entries map[string]localbackend.DeviceListEntry) {
	ready, degraded, stopped, fresh := sessionsSummary(entries)
	e.out.Human("\nSessions:\n")
	e.out.Human("  Devices:     %d configured\n", len(entries))
	parts := fmt.Sprintf("%d ready", ready)
	if degraded > 0 {
		parts += fmt.Sprintf(", %d degraded", degraded)
	}
	if stopped > 0 {
		parts += fmt.Sprintf(", %d stopped", stopped)
	}
	e.out.Human("  Sessions:    %s\n", parts)
	e.out.Human("  Local state: %d fully synced\n", fresh)
	e.out.Human("  Run `mc device list` for per-device detail.\n")
}

func sessionsSummaryJSON(entries map[string]localbackend.DeviceListEntry) map[string]any {
	ready, degraded, stopped, fresh := sessionsSummary(entries)
	return map[string]any{
		"configured":     len(entries),
		"ready":          ready,
		"degraded":       degraded,
		"stopped":        stopped,
		"local_state_ok": fresh,
		"fresh_replicas": fresh,
	}
}

func backendServe(ctx context.Context, e *env) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	// No WithMessageSync: the backend is the sole inbox drainer and drives it
	// explicitly (DrainMessages) so it can persist each message before
	// broadcasting.
	opts := append(e.dbg.DialOptions(), meshcore.WithClientOptions(
		meshcore.WithTimeout(backendContactSyncTimeout),
	))

	daemon, err := localbackend.NewDaemon(localbackend.DaemonOptions{
		Socket:      resolveBackendSocket(e),
		LogRequests: cfg.Backend.LogRequests,
	})
	if err != nil {
		return err
	}

	// An explicit --uri/--device means "start this device's session now" and
	// makes it the default. Without it, the daemon comes up as a supervisor and
	// connects only the devices whose config sets backend.autostart: true.
	explicit := e.args.flag("uri") != "" || e.args.flag("device") != ""
	var defaultID string
	if explicit {
		uri, profile, err := resolveURI(e)
		if err != nil {
			return err
		}
		defaultID = profile
		if defaultID == "" {
			defaultID = deviceNameForURI(cfg, uri)
		}
		if defaultID == "" {
			defaultID = "default"
		}
		daemon.Register(localbackend.SessionProfile{
			ID:        defaultID,
			URI:       uri,
			PublicKey: cfg.Devices[defaultID].PublicKey,
			Bridges:   primaryBridges(cfg, defaultID),
			Autostart: true,
			DialOpts:  opts,
		})
		localbackend.Logf("starting session for %s (%s)", defaultID, uri)
	} else {
		// The default routing target is the current profile, even if its session
		// is not autostarted (it may be started later or connected directly).
		defaultID = cfg.Current
	}

	// Register every saved device. Each autostarts only per its own config; the
	// explicitly-started device (above) is left as-is.
	for name, dev := range cfg.Devices {
		if explicit && name == defaultID {
			continue
		}
		devURI := dev.PrimaryURI()
		if devURI == "" {
			continue
		}
		daemon.Register(localbackend.SessionProfile{
			ID:        name,
			URI:       devURI,
			PublicKey: dev.PublicKey,
			Bridges:   bridgesFromDeviceBackend(dev.Backend),
			Autostart: dev.Backend.Autostart,
			DialOpts:  opts,
		})
	}

	if defaultID != "" {
		daemon.SetDefault(defaultID)
	}
	if cfg.Backend.LogRequests {
		localbackend.Logf("ipc request logging enabled")
	}
	localbackend.Logf("daemon ready on %s", daemon.Socket())
	return daemon.Serve()
}

func stopBackend(ctx context.Context, e *env) (localbackend.DaemonStatus, bool, error) {
	st, ok := daemonRunning(ctx, e)
	if !ok {
		return st, false, fmt.Errorf("backend is not running")
	}
	// "stop" is a daemon-level method; target the daemon, not a device.
	if err := localbackend.NewClient(resolveBackendSocket(e)).Stop(ctx); err != nil {
		return st, false, err
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := daemonRunning(ctx, e); !ok {
			return st, true, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return st, false, fmt.Errorf("backend did not stop")
}

func backendStatus(ctx context.Context, e *env) (localbackend.Status, bool) {
	st, err := backendClientForEnv(e).Status(ctx)
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

func printBackendStartSummary(e *env, st localbackend.DaemonStatus) {
	e.out.Human("Backend up (pid %d).\n", st.PID)
	if st.Socket != "" {
		e.out.Human("Socket: %s\n", st.Socket)
	}

	// Announce which device sessions the daemon is bringing up. This is driven
	// by config (autostart) and the explicit --uri/--device passed to start,
	// since freshly-launched sessions may not yet appear connected.
	cfg, err := config.Load()
	if err != nil {
		return
	}
	announced := map[string]bool{}
	announce := func(name, uri string) {
		if uri == "" || announced[name] {
			return
		}
		announced[name] = true
		if name != "" {
			e.out.Human("Starting session for %s (%s)...\n", name, uri)
		} else {
			e.out.Human("Starting session for %s...\n", uri)
		}
	}

	if dev := e.args.flag("device"); dev != "" {
		if d, ok := cfg.Devices[dev]; ok {
			announce(dev, d.PrimaryURI())
		}
	}
	if uri := e.args.flag("uri"); uri != "" {
		announce(deviceNameForURI(cfg, uri), uri)
	}
	for name, dev := range cfg.Devices {
		if dev.Backend.Autostart {
			announce(name, dev.PrimaryURI())
		}
	}

	if len(announced) == 0 {
		e.out.Human("No device sessions autostarted. Run `mc session start <name>` to connect one.\n")
	}
}

func daemonStatusJSON(st localbackend.DaemonStatus) map[string]any {
	devices := make([]map[string]any, len(st.Devices))
	for i, d := range st.Devices {
		devices[i] = deviceListEntryJSON(d)
	}
	ready, degraded, stopped, fresh := sessionsSummary(daemonEntriesMap(st.Devices))
	return map[string]any{
		"running":           st.Running,
		"pid":               st.PID,
		"started_at":        st.StartedAt,
		"uptime_sec":        st.UptimeSec,
		"version":           st.Version,
		"default":           st.DefaultID,
		"socket":            st.Socket,
		"devices":           devices,
		"sessions_ready":    ready,
		"sessions_degraded": degraded,
		"sessions_stopped":  stopped,
		"replicas_fresh":    fresh,
	}
}

func deviceListEntryJSON(d localbackend.DeviceListEntry) map[string]any {
	return map[string]any{
		"id":        d.ID,
		"default":   d.Default,
		"session":   d.Session,
		"connected": d.Connected,
		"replica":   d.Replica,
		"local_state": map[string]any{
			"contacts": contactStatusJSON(d.Contacts),
			"channels": channelStatusJSON(d.Channels),
		},
		"transport":  d.Transport,
		"uri":        d.URI,
		"last_error": d.LastError,
	}
}

func daemonEntriesMap(entries []localbackend.DeviceListEntry) map[string]localbackend.DeviceListEntry {
	m := make(map[string]localbackend.DeviceListEntry, len(entries))
	for _, e := range entries {
		m[e.ID] = e
	}
	return m
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
	return bridgesFromConfigBridges(cfg.Backend.Bridges)
}

func bridgesFromDeviceBackend(db config.DeviceBackend) []localbackend.BridgeConfig {
	return bridgesFromConfigBridges(db.Bridges)
}

func bridgesFromConfigBridges(bridges []config.Bridge) []localbackend.BridgeConfig {
	out := make([]localbackend.BridgeConfig, 0, len(bridges))
	for _, bridge := range bridges {
		out = append(out, localbackend.BridgeConfig{
			Enabled: bridge.Enabled,
			Type:    bridge.Type,
			Listen:  bridge.Listen,
			Name:    bridge.Name,
		})
	}
	return out
}

// deviceNameForURI returns the saved profile name whose endpoints include uri,
// or "" if none match. Used to recover a profile name when the daemon is
// launched with an explicit --uri.
func deviceNameForURI(cfg *config.Config, uri string) string {
	for name, dev := range cfg.Devices {
		if dev.PrimaryURI() == uri {
			return name
		}
		for _, t := range dev.Transports {
			if t.URI == uri {
				return name
			}
		}
	}
	return ""
}

// primaryBridges returns the bridge listeners for the daemon's default device:
// the device's own per-device bridges when configured, otherwise the legacy
// global backend.bridges (used for `mc backend serve --uri ...`).
func primaryBridges(cfg *config.Config, profile string) []localbackend.BridgeConfig {
	if profile != "" {
		if dev, ok := cfg.Devices[profile]; ok && len(dev.Backend.Bridges) > 0 {
			return bridgesFromDeviceBackend(dev.Backend)
		}
	}
	return bridgesFromConfig(cfg)
}
