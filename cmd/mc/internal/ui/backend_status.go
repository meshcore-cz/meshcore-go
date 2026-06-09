package ui

import (
	"fmt"
	"os"
	"strings"
	"time"
)

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

func sectionLine(label, value string) string {
	return fmt.Sprintf("%s%-*s %s\n", statusIndent, statusLabelWidth-len(statusIndent), label+":", value)
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
