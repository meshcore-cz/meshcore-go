package ui

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	meshcore "github.com/meshcore-cz/meshcore-go"
)

const backendStatsPollInterval = 30 * time.Second

// BridgeInfo is one configured bridge listener.
type BridgeInfo struct {
	Name   string
	Type   string
	Listen string
	Path   string
	Active bool
	Error  string
}

// BackendStatusData is the input model for mc backend status rendering.
type BackendStatusData struct {
	Running           bool
	Healthy           bool
	State             string
	PID               int
	StartedAt         time.Time
	UptimeSec         int64
	Socket            string
	URI               string
	Transport         string
	LastSeen          time.Time
	LastError         string
	LastErrorAt       time.Time
	Contacts          ReplicaInfo
	Channels          ReplicaInfo
	Stats             DeviceStatsInfo
	RadioIO           RadioIOInfo
	Bridges           []BridgeInfo
	QueuePending      int
	Reconnects        int
	Clients           int
	RequestsCompleted int64
	RequestsFailed    int64
	Version           string
	CLIVersion        string
	LogPath           string
	ConfigPath        string
	StatsPollTimeout  time.Duration
	Verbose           bool
	Sessions          []BackendSessionInfo
}

// BackendSessionInfo is one daemon-managed session in backend status output.
type BackendSessionInfo struct {
	Name              string
	Active            bool
	State             string
	Healthy           bool
	Transport         string
	StartedAt         time.Time
	LastActive        time.Time
	LastError         string
	Activity          RadioIOInfo
	LocalState        LocalStateInfo
	LocalStatePath    string
	Clients           int
	RequestsCompleted int64
	RequestsFailed    int64
	QueuePending      int
}

// RenderBackendStatus renders mc backend status for human output.
func RenderBackendStatus(data BackendStatusData, printer Printer) string {
	theme := NewTheme(printer.Out)
	var b strings.Builder

	writeBackendOverview(&b, data, theme)
	if !data.Running {
		return b.String()
	}
	b.WriteString("\n")
	writeBackendIPCSummary(&b, data)
	if len(data.Sessions) > 0 {
		b.WriteString("\n")
		writeBackendSessions(&b, data, theme)
	}

	return b.String()
}

func writeBackendOverview(b *strings.Builder, data BackendStatusData, theme Theme) {
	b.WriteString(statusLine("Backend", backendDaemonLabel(data, theme)))
	if data.PID > 0 {
		b.WriteString(statusLine("PID", fmt.Sprintf("%d", data.PID)))
	}
	if data.UptimeSec > 0 {
		b.WriteString(statusLine("Uptime", FormatDurationSecs(uint32(data.UptimeSec))))
	}
	if data.Socket != "" {
		b.WriteString(statusLine("Socket", displayPath(data.Socket)))
	}
	if data.Running && !data.StartedAt.IsZero() && !data.Verbose {
		b.WriteString(statusLine("Started", data.StartedAt.Format("2006-01-02 15:04:05")))
	}
	if data.Verbose {
		if data.ConfigPath != "" {
			b.WriteString(statusLine("Config", displayPath(data.ConfigPath)))
		}
		if data.LogPath != "" {
			b.WriteString(statusLine("Log", displayPath(data.LogPath)))
		}
	}
}

func backendDaemonLabel(data BackendStatusData, theme Theme) string {
	if !data.Running {
		return "not running"
	}
	if !data.Healthy {
		return theme.StatusWord(HealthError, "unhealthy")
	}
	return theme.StatusWord(HealthOK, "running")
}

func writeBackendIPCSummary(b *strings.Builder, data BackendStatusData) {
	if data.Verbose {
		b.WriteString("IPC\n")
		b.WriteString(sectionLine("Clients", backendClientsLabel(data.Clients)))
		b.WriteString(sectionLine("Requests", backendVerboseRequestsLabel(data.RequestsCompleted, data.QueuePending, data.RequestsFailed)))
		return
	}
	b.WriteString(statusLine("Sessions", backendSessionsSummary(data.Sessions)))
	b.WriteString(statusLine("IPC clients", backendClientsLabel(data.Clients)))
	b.WriteString(statusLine("Requests", backendRequestsHandledLabel(data.RequestsCompleted, data.RequestsFailed)))
}

func writeBackendSessions(b *strings.Builder, data BackendStatusData, theme Theme) {
	b.WriteString("Sessions\n")
	for _, session := range data.Sessions {
		b.WriteString("\n")
		b.WriteString("  " + session.Name)
		if session.Active {
			b.WriteString("  (active)")
		}
		b.WriteString("\n")
		b.WriteString(statusBlockSubLine("State", backendSessionStateLabel(session, theme)))
		if session.Transport != "" {
			b.WriteString(statusBlockSubLine("Transport", session.Transport))
		}
		if session.State != "stopped" {
			if data.Verbose {
				b.WriteString(statusBlockSubLine("Requests", backendRequestsHandledLabel(session.RequestsCompleted, session.RequestsFailed)))
			}
			if activity := backendSessionActivityLabel(session, theme); activity != "" {
				b.WriteString(statusBlockSubLine("Activity", activity))
			}
		}
		if session.LastError != "" {
			b.WriteString(statusBlockSubLine("Error", theme.StatusWord(HealthError, session.LastError)))
		}
		writeBackendSessionLocalState(b, session, data.Verbose)
	}
}

func backendSessionsSummary(sessions []BackendSessionInfo) string {
	total := len(sessions)
	ready, retrying, stopped := 0, 0, 0
	for _, s := range sessions {
		switch s.State {
		case "ready", "bridge":
			ready++
		case "degraded":
			retrying++
		case "stopped":
			stopped++
		}
	}
	parts := []string{fmt.Sprintf("%d total", total)}
	if ready > 0 {
		parts = append(parts, fmt.Sprintf("%d ready", ready))
	}
	if retrying > 0 {
		parts = append(parts, fmt.Sprintf("%d retrying", retrying))
	}
	if stopped > 0 {
		parts = append(parts, fmt.Sprintf("%d stopped", stopped))
	}
	return strings.Join(parts, " · ")
}

func backendSessionStateLabel(session BackendSessionInfo, theme Theme) string {
	state := strings.TrimSpace(session.State)
	if state == "" {
		state = "unknown"
	}
	health := HealthUnknown
	switch state {
	case "ready", "bridge":
		health = HealthOK
	case "degraded":
		health = HealthWarning
	case "stopped":
		return "stopped"
	}
	labelState := state
	if state == "degraded" {
		labelState = "retrying"
	}
	label := theme.StatusWord(health, labelState)
	if (state == "ready" || state == "bridge") && !session.StartedAt.IsZero() {
		label += " · " + theme.Dim("connected "+RelativeTime(session.StartedAt))
	}
	if state == "degraded" && !session.LastActive.IsZero() {
		label += " · " + theme.Dim("last active "+RelativeTime(session.LastActive))
	}
	return label
}

func backendSessionActivityLabel(session BackendSessionInfo, theme Theme) string {
	if session.State == "degraded" {
		if session.LastActive.IsZero() {
			return ""
		}
		return "last active " + RelativeTime(session.LastActive)
	}
	return backendActivitySummary(session.Activity, theme)
}

func writeBackendSessionLocalState(b *strings.Builder, session BackendSessionInfo, verbose bool) {
	local := session.LocalState
	if verbose {
		if session.LocalStatePath != "" {
			b.WriteString(statusBlockSubLine("Local state", displayPath(session.LocalStatePath)))
		} else {
			b.WriteString(statusBlockSubLine("Local state", "not initialized"))
		}
		if local.Initialized {
			if !local.UpdatedAt.IsZero() {
				b.WriteString(statusBlockSubLine("Updated", RelativeTime(local.UpdatedAt)))
			}
			b.WriteString(statusBlockSubLine("Contacts", fmt.Sprintf("%d", local.Contacts)))
			b.WriteString(statusBlockSubLine("Channels", fmt.Sprintf("%d", local.Channels)))
		}
		return
	}
	if !local.Initialized {
		b.WriteString(statusBlockSubLine("Local state", "not initialized"))
		return
	}
	if local.UpdatedAt.IsZero() {
		b.WriteString(statusBlockSubLine("Local state", "available"))
		return
	}
	b.WriteString(statusBlockSubLine("Local state", "updated "+RelativeTime(local.UpdatedAt)))
}

func writeBackendSection(b *strings.Builder, data BackendStatusData, theme Theme) {
	b.WriteString("Backend:\n")
	b.WriteString(sectionLine("State", backendStateLabel(data, theme)))
	if data.PID > 0 {
		b.WriteString(sectionLine("PID", fmt.Sprintf("%d", data.PID)))
	}
	if data.UptimeSec > 0 {
		b.WriteString(sectionLine("Uptime", FormatDurationSecs(uint32(data.UptimeSec))))
	}
	if data.Socket != "" {
		b.WriteString(sectionLine("Socket", theme.Dim(data.Socket)))
	}
}

func writeBackendRadioSection(b *strings.Builder, data BackendStatusData, theme Theme) {
	b.WriteString("Radio:\n")
	b.WriteString(sectionLine("State", backendRadioStateLabel(data, theme)))
	if transport := backendTransportLabel(data.URI); transport != "" {
		b.WriteString(sectionLine("Transport", transport))
	}
	if data.URI != "" {
		b.WriteString(sectionLine("Endpoint", theme.Dim(data.URI)))
	}
	if !data.LastSeen.IsZero() {
		b.WriteString(sectionLine("Last seen", theme.Dim(RelativeTime(data.LastSeen))))
	}
}

func writeBackendReplicaSection(b *strings.Builder, data BackendStatusData, theme Theme) {
	b.WriteString("Local state:\n")
	b.WriteString(sectionLine("Contacts", backendReplicaContactsLabel(data, theme)))
	b.WriteString(sectionLine("Channels", backendReplicaChannelsLabel(data, theme)))
}

func writeBackendDeviceStatsSection(b *strings.Builder, stats DeviceStatsInfo) {
	b.WriteString("Device stats:\n")
	if uptime := uptimeLabel(stats); uptime != "" {
		b.WriteString(sectionLine("Uptime", uptime))
	}
	if battery := batteryLabel(stats); battery != "" {
		b.WriteString(sectionLine("Battery", battery))
	}
	if packets := packetsLabel(stats); packets != "" {
		b.WriteString(sectionLine("Packets", packets))
	}
	if airtime := airtimeLabel(stats); airtime != "" {
		b.WriteString(sectionLine("Airtime", airtime))
	}
	if queue := backendDeviceQueueLabel(stats); queue != "" {
		b.WriteString(sectionLine("Queue", queue))
	}
}

func writeBackendDiagnosticsSection(b *strings.Builder, data BackendStatusData, theme Theme) {
	b.WriteString("Diagnostics:\n")
	if data.Verbose && !data.StartedAt.IsZero() {
		b.WriteString(sectionLine("Started", theme.Dim(data.StartedAt.Format("2006-01-02 15:04:05"))))
	}
	b.WriteString(sectionLine("Activity", backendActivityLabel(data, theme)))
	b.WriteString(sectionLine("Last op", backendLastOpLabel(data, theme)))
	b.WriteString(sectionLine("Stats poll", backendStatsPollLabel(data, theme)))
	b.WriteString(sectionLine("Queue", backendRequestQueueLabel(data.QueuePending)))
	b.WriteString(sectionLine("Reconnects", fmt.Sprintf("%d", data.Reconnects)))
	b.WriteString(sectionLine("Clients", backendClientsLabel(data.Clients)))
	if data.Verbose {
		b.WriteString(sectionLine("Requests", backendRequestsLabel(data.RequestsCompleted, data.RequestsFailed)))
		if data.Version != "" {
			b.WriteString(sectionLine("Version", backendVersionLabel(data, theme)))
		}
		if data.LogPath != "" {
			b.WriteString(sectionLine("Log", theme.Dim(data.LogPath)))
		}
		b.WriteString(sectionLine("Socket mode", theme.Dim("0600")))
	}
	b.WriteString(sectionLine("Last error", backendLastErrorLabel(data, theme)))
}

func writeBackendBridgesSection(b *strings.Builder, bridges []BridgeInfo, theme Theme) {
	b.WriteString("Bridges:\n")
	sorted := append([]BridgeInfo(nil), bridges...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Type != sorted[j].Type {
			return sorted[i].Type < sorted[j].Type
		}
		return sorted[i].Name < sorted[j].Name
	})
	for _, bridge := range sorted {
		label := strings.ToUpper(bridge.Type)
		if label == "" {
			label = strings.ToUpper(bridge.Name)
		}
		b.WriteString(sectionLine(label, backendBridgeLabel(bridge, theme)))
	}
}

func sectionLine(label, value string) string {
	return fmt.Sprintf("%s%-*s %s\n", statusIndent, statusLabelWidth-len(statusIndent), label+":", value)
}

func backendStateLabel(data BackendStatusData, theme Theme) string {
	state := strings.TrimSpace(data.State)
	if state == "" {
		state = "unknown"
	}
	switch state {
	case "ready":
		return theme.StatusWord(HealthOK, state)
	case "degraded":
		return theme.StatusWord(HealthWarning, state)
	default:
		if data.Healthy {
			return theme.StatusWord(HealthOK, state)
		}
		return theme.StatusWord(HealthWarning, state)
	}
}

func backendRadioStateLabel(data BackendStatusData, theme Theme) string {
	if data.Healthy && data.State == "ready" {
		return theme.StatusWord(HealthOK, "connected")
	}
	return theme.StatusWord(HealthError, "unavailable")
}

func backendTransportLabel(uri string) string {
	scheme := transportScheme(uri)
	switch scheme {
	case "ble":
		return "BLE"
	case "serial":
		return "Serial"
	case "tcp":
		return "TCP"
	case "":
		return ""
	default:
		return strings.ToUpper(scheme)
	}
}

func transportScheme(uri string) string {
	if i := strings.IndexByte(uri, ':'); i >= 0 {
		return uri[:i]
	}
	return ""
}

func backendReplicaCached(data BackendStatusData) bool {
	return !data.Healthy || data.State == "degraded"
}

func backendReplicaContactsLabel(data BackendStatusData, theme Theme) string {
	c := data.Contacts
	if c.Syncing {
		word := theme.StatusWord(HealthWarning, "syncing")
		progress := backendContactSyncProgress(c)
		if progress != "" {
			return word + " · " + progress
		}
		return word
	}
	if c.Error != "" {
		return theme.StatusWord(HealthError, c.Error)
	}
	if c.SyncedAt.IsZero() && c.Count == 0 {
		return "not synced"
	}
	parts := []string{fmt.Sprintf("%d", c.Count)}
	if backendReplicaCached(data) {
		parts = append(parts, theme.StatusWord(HealthWarning, "stale"))
	}
	if !c.SyncedAt.IsZero() {
		parts = append(parts, theme.Dim("synced "+RelativeTime(c.SyncedAt)))
	}
	return strings.Join(parts, " · ")
}

func backendReplicaChannelsLabel(data BackendStatusData, theme Theme) string {
	c := data.Channels
	if c.Syncing {
		return theme.StatusWord(HealthWarning, "syncing")
	}
	if c.Error != "" {
		return theme.StatusWord(HealthError, c.Error)
	}
	if c.SyncedAt.IsZero() && c.Count == 0 {
		return "not synced"
	}
	parts := []string{fmt.Sprintf("%d", c.Count)}
	if backendReplicaCached(data) {
		parts = append(parts, theme.StatusWord(HealthWarning, "stale"))
	}
	if !c.SyncedAt.IsZero() {
		parts = append(parts, theme.Dim("synced "+RelativeTime(c.SyncedAt)))
	}
	return strings.Join(parts, " · ")
}

func backendContactSyncProgress(c ReplicaInfo) string {
	if c.SyncTotal > 0 {
		return fmt.Sprintf("%d/%d received", c.SyncReceived, c.SyncTotal)
	}
	if c.SyncReceived > 0 {
		return fmt.Sprintf("%d/? received", c.SyncReceived)
	}
	return ""
}

func backendDeviceQueueLabel(stats DeviceStatsInfo) string {
	if !stats.Available {
		return ""
	}
	if stats.Core.QueueLen == 0 {
		return "0 radio packets pending"
	}
	return fmt.Sprintf("%d radio packets pending", stats.Core.QueueLen)
}

func backendRequestQueueLabel(pending int) string {
	if pending == 0 {
		return "0 backend requests pending"
	}
	return fmt.Sprintf("%d backend requests pending", pending)
}

func backendActivityLabel(data BackendStatusData, theme Theme) string {
	if data.State == "degraded" {
		return theme.StatusWord(HealthWarning, "reconnecting")
	}
	if data.Contacts.Syncing {
		word := theme.StatusWord(HealthWarning, "syncing contacts")
		if data.RadioIO.Active && data.RadioIO.Method == "contacts" && data.RadioIO.DurationMs > 0 {
			return word + " · " + theme.Dim(formatRunningDuration(time.Duration(data.RadioIO.DurationMs)*time.Millisecond))
		}
		return word
	}
	if data.Channels.Syncing {
		word := theme.StatusWord(HealthWarning, "syncing channels")
		if data.RadioIO.Active && data.RadioIO.Method == "channels" && data.RadioIO.DurationMs > 0 {
			return word + " · " + theme.Dim(formatRunningDuration(time.Duration(data.RadioIO.DurationMs)*time.Millisecond))
		}
		return word
	}
	if data.RadioIO.Active {
		word := theme.StatusWord(HealthWarning, radioMethodLabel(data.RadioIO.Method))
		if data.RadioIO.DurationMs > 0 {
			return word + " · " + theme.Dim(formatRunningDuration(time.Duration(data.RadioIO.DurationMs)*time.Millisecond))
		}
		return word
	}
	return theme.StatusWord(HealthOK, "idle")
}

func backendLastOpLabel(data BackendStatusData, theme Theme) string {
	io := data.RadioIO
	if io.LastMethod == "" && io.LastAt.IsZero() {
		return theme.Dim("none")
	}
	method := radioMethodLabel(io.LastMethod)
	if data.State == "degraded" && io.LastMethod == "stats" {
		failed := theme.StatusWord(HealthError, "failed")
		return method + " · " + failed + " · " + theme.Dim(RelativeTime(io.LastAt))
	}
	parts := []string{method}
	if io.LastDurationMs > 0 {
		parts = append(parts, formatDuration(time.Duration(io.LastDurationMs)*time.Millisecond))
	}
	if !io.LastAt.IsZero() {
		parts = append(parts, theme.Dim(RelativeTime(io.LastAt)))
	}
	return strings.Join(parts, " · ")
}

func backendStatsPollLabel(data BackendStatusData, theme Theme) string {
	if data.State == "degraded" || !data.Healthy {
		label := theme.StatusWord(HealthError, "failed")
		if data.Verbose {
			label += " · " + backendStatsPollConfig(data, theme)
		}
		return label
	}
	if data.RadioIO.Active {
		stale := false
		if !data.Stats.UpdatedAt.IsZero() {
			stale = time.Since(data.Stats.UpdatedAt) > backendStatsPollInterval
		} else if !data.LastSeen.IsZero() {
			stale = time.Since(data.LastSeen) > backendStatsPollInterval
		}
		if stale {
			label := theme.StatusWord(HealthWarning, "delayed") + " · " + theme.Dim("radio busy")
			if data.Verbose {
				label += " · " + backendStatsPollConfig(data, theme)
			}
			return label
		}
	}
	if data.Stats.Available && !data.Stats.UpdatedAt.IsZero() {
		updated := StatsUpdatedRelative(data.Stats.UpdatedAt)
		if StatsUpdateStale(data.Stats.UpdatedAt) {
			label := theme.StatusWord(HealthWarning, "stale") + " · " + theme.Dim("updated "+updated)
			if data.Verbose {
				label += " · " + backendStatsPollConfig(data, theme)
			}
			return label
		}
		label := theme.StatusWord(HealthOK, "healthy") + " · " + theme.Dim("updated "+updated)
		if data.Verbose {
			label += " · " + backendStatsPollConfig(data, theme)
		}
		return label
	}
	if !data.LastSeen.IsZero() {
		updated := StatsUpdatedRelative(data.LastSeen)
		if StatsUpdateStale(data.LastSeen) {
			label := theme.StatusWord(HealthWarning, "stale") + " · " + theme.Dim("updated "+updated)
			if data.Verbose {
				label += " · " + backendStatsPollConfig(data, theme)
			}
			return label
		}
		label := theme.StatusWord(HealthOK, "healthy") + " · " + theme.Dim("updated "+updated)
		if data.Verbose {
			label += " · " + backendStatsPollConfig(data, theme)
		}
		return label
	}
	label := theme.StatusWord(HealthWarning, "pending")
	if data.Verbose {
		label += " · " + backendStatsPollConfig(data, theme)
	}
	return label
}

func backendStatsPollConfig(data BackendStatusData, theme Theme) string {
	timeout := data.StatsPollTimeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return theme.Dim(fmt.Sprintf("every %s · timeout %s",
		formatDuration(backendStatsPollInterval),
		formatDuration(timeout)))
}

func backendClientsLabel(clients int) string {
	switch clients {
	case 0:
		return "0 connected"
	case 1:
		return "1 connected"
	default:
		return fmt.Sprintf("%d connected", clients)
	}
}

func backendRequestsLabel(ok, failed int64) string {
	return fmt.Sprintf("%d completed · %d failed", ok, failed)
}

func backendRequestsHandledLabel(ok, failed int64) string {
	return fmt.Sprintf("%s handled · %s failed", formatInt64(ok), formatInt64(failed))
}

func backendVerboseRequestsLabel(ok int64, active int, failed int64) string {
	return fmt.Sprintf("%s handled · %d active · %s failed", formatInt64(ok), active, formatInt64(failed))
}

func formatInt64(n int64) string {
	if n < 0 {
		return "-" + formatInt64(-n)
	}
	s := fmt.Sprintf("%d", n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func displayPath(path string) string {
	if home, ok := homeDir(); ok {
		if path == home {
			return "~"
		}
		if strings.HasPrefix(path, home+"/") {
			return "~" + strings.TrimPrefix(path, home)
		}
	}
	return path
}

func homeDir() (string, bool) {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return "", false
	}
	return home, true
}

func backendLastErrorLabel(data BackendStatusData, theme Theme) string {
	if data.LastError == "" {
		return theme.Dim("none")
	}
	if data.Verbose && !data.LastErrorAt.IsZero() {
		return data.LastError + " · " + theme.Dim(RelativeTime(data.LastErrorAt))
	}
	return data.LastError
}

func backendVersionLabel(data BackendStatusData, theme Theme) string {
	if data.CLIVersion == "" || data.Version == "" || data.CLIVersion == data.Version {
		return data.Version
	}
	return data.Version + " · " + theme.StatusWord(HealthWarning, "cli "+data.CLIVersion)
}

func backendBridgeLabel(bridge BridgeInfo, theme Theme) string {
	addr := bridge.Listen
	if bridge.Type == "pty" && bridge.Path != "" {
		addr = bridge.Path
	}
	if bridge.Error != "" {
		return theme.StatusWord(HealthError, "failed") + " · " + theme.Dim(bridge.Error)
	}
	if bridge.Active {
		if addr != "" {
			return theme.StatusWord(HealthOK, "listening") + " · " + theme.Dim(addr)
		}
		return theme.StatusWord(HealthOK, "listening")
	}
	if addr != "" {
		return theme.Dim("inactive · " + addr)
	}
	return theme.Dim("inactive")
}

// DeviceStatsFromBackend builds a device stats snapshot for backend status.
func DeviceStatsFromBackend(stats meshcore.LocalStats, ok bool, updatedAt time.Time) DeviceStatsInfo {
	return DeviceStatsFromLocal(stats, ok, updatedAt)
}
