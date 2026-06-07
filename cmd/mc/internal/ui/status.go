package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

const statusLabelWidth = 14

// DeviceInfo is the connected radio identity shown in status output.
type DeviceInfo struct {
	Name            string
	PublicKey       string
	Firmware        string
	FirmwareVersion string
	Protocol        string
	Transport       string
	TransportURI    string
	Capabilities    []string
	Radio           RadioInfo
	Available       bool
}

// RadioInfo describes cached local radio configuration.
type RadioInfo struct {
	FrequencyKHz uint32
	BandwidthKHz uint32
	Spreading    byte
	CodingRate   byte
	TxPowerDBm   byte
}

// ReplicaInfo describes a local backend replica.
type ReplicaInfo struct {
	Syncing      bool
	SyncReceived int
	SyncTotal    int
	Count        int
	SyncedAt     time.Time
	Error        string
}

// RadioIOInfo describes backend transport lock state for status output.
type RadioIOInfo struct {
	Active     bool
	Method     string
	DurationMs int64
}

// BackendInfo is backend daemon state for status output.
type BackendInfo struct {
	Running   bool
	Healthy   bool
	State     string
	PID       int
	URI       string
	LastError string
	LastSeen  time.Time
	Contacts  ReplicaInfo
	Channels  ReplicaInfo
	RadioIO   RadioIOInfo
}

// StatusData is the input model for status rendering.
type StatusData struct {
	Device  DeviceInfo
	Backend BackendInfo
}

// RenderStatus renders mc status for human output.
func RenderStatus(data StatusData, printer Printer) string {
	theme := NewTheme(printer.Out)
	var b strings.Builder

	if data.Device.Available {
		b.WriteString(statusLine("Device", deviceLabel(data.Device)))
		b.WriteString(statusLine("Firmware", firmwareLabel(data.Device)))
		b.WriteString(statusLine("Protocol", orDash(data.Device.Protocol)))
		b.WriteString(statusLine("Transport", transportLabel(data.Device)))
		b.WriteString("\n")
		b.WriteString(statusLine("Public key", orDash(strings.ToLower(strings.TrimSpace(data.Device.PublicKey)))))
		if radio := radioLabel(data.Device.Radio); radio != "" {
			b.WriteString(statusLine("Radio", radio))
		}
	} else {
		b.WriteString(statusLine("Device", "unavailable"))
		if data.Backend.URI != "" {
			b.WriteString(statusLine("Transport", data.Backend.URI))
		}
	}
	b.WriteString("\n")

	if data.Backend.Running {
		b.WriteString(statusLine("Backend", backendLabel(data.Backend, theme)))
	} else if data.Device.Available {
		b.WriteString(statusLine("Backend", "not running"))
	}

	if data.Backend.Running {
		b.WriteString(statusLine("Replica", replicaLabel(data.Backend, theme)))
		b.WriteString(statusLine("Activity", RadioIOLabel(data.Backend.RadioIO)))
		b.WriteString(statusLine("Contacts", contactsLabel(data.Backend.Contacts)))
		b.WriteString(statusLine("Channels", channelsLabel(data.Backend.Channels)))
		b.WriteString("\n")
	}

	if data.Backend.LastError != "" {
		b.WriteString(statusLine("Last error", data.Backend.LastError))
	}

	return b.String()
}

// RenderDeviceShow renders full connected-device details including capabilities.
func RenderDeviceShow(dev DeviceInfo, printer Printer) string {
	if !dev.Available {
		return statusLine("Device", "unavailable")
	}
	var b strings.Builder
	b.WriteString(statusLine("Device", deviceLabel(dev)))
	b.WriteString(statusLine("Firmware", firmwareLabel(dev)))
	b.WriteString(statusLine("Protocol", orDash(dev.Protocol)))
	b.WriteString(statusLine("Transport", transportLabel(dev)))
	b.WriteString(statusLine("Public key", orDash(strings.ToLower(strings.TrimSpace(dev.PublicKey)))))
	b.WriteString(statusLine("Capabilities", formatCapabilities(dev.Capabilities)))
	return b.String()
}

func statusLine(label, value string) string {
	return fmt.Sprintf("%-*s %s\n", statusLabelWidth, label+":", value)
}

func deviceLabel(dev DeviceInfo) string {
	if id := deviceShortID(dev.PublicKey); id != "UNKNOWN" {
		return id
	}
	return orDash(dev.Name)
}

func firmwareLabel(dev DeviceInfo) string {
	name := strings.TrimSpace(dev.Firmware)
	version := strings.TrimSpace(dev.FirmwareVersion)
	switch {
	case name != "" && version != "":
		return name + " " + version
	case name != "":
		return name
	case version != "":
		return version
	default:
		return "-"
	}
}

func transportLabel(dev DeviceInfo) string {
	if addr := deviceAddress(dev); addr != "-" {
		return addr
	}
	return orDash(dev.Transport)
}

func radioLabel(r RadioInfo) string {
	if r.FrequencyKHz == 0 && r.BandwidthKHz == 0 && r.Spreading == 0 && r.CodingRate == 0 && r.TxPowerDBm == 0 {
		return ""
	}
	parts := []string{}
	if r.FrequencyKHz > 0 {
		parts = append(parts, fmt.Sprintf("%.3f MHz", float64(r.FrequencyKHz)/1000))
	}
	if r.BandwidthKHz > 0 {
		parts = append(parts, fmt.Sprintf("BW %d kHz", r.BandwidthKHz))
	}
	if r.Spreading > 0 {
		parts = append(parts, fmt.Sprintf("SF%d", r.Spreading))
	}
	if r.CodingRate > 0 {
		parts = append(parts, fmt.Sprintf("CR 4/%d", r.CodingRate))
	}
	if r.TxPowerDBm > 0 {
		parts = append(parts, fmt.Sprintf("TX %d dBm", r.TxPowerDBm))
	}
	return strings.Join(parts, " · ")
}

func deviceAddress(dev DeviceInfo) string {
	if u := strings.TrimSpace(dev.TransportURI); u != "" {
		return u
	}
	if u := strings.TrimSpace(dev.Transport); u != "" && strings.Contains(u, "://") {
		return u
	}
	return "-"
}

func backendLabel(be BackendInfo, theme Theme) string {
	state := strings.TrimSpace(be.State)
	if state == "" {
		state = "unknown"
	}
	display := state
	if state == "ready" {
		display = "running"
	}
	word := theme.StatusWord(backendHealth(be), display)
	if be.PID > 0 {
		return word + fmt.Sprintf(" (pid %d)", be.PID)
	}
	return word
}

func backendHealth(be BackendInfo) Health {
	if be.Healthy && be.State == "ready" {
		return HealthOK
	}
	if be.Running {
		return HealthError
	}
	return HealthUnknown
}

func replicaLabel(be BackendInfo, theme Theme) string {
	word := replicaStateWord(be)
	return theme.StatusWord(replicaHealth(be), word)
}

func replicaHealth(be BackendInfo) Health {
	if be.Contacts.Syncing || be.Channels.Syncing {
		return HealthWarning
	}
	if be.Contacts.Error != "" || be.Channels.Error != "" {
		return HealthError
	}
	if be.Contacts.SyncedAt.IsZero() && be.Channels.SyncedAt.IsZero() {
		return HealthWarning
	}
	return HealthOK
}

func replicaStateWord(be BackendInfo) string {
	if be.Contacts.Syncing || be.Channels.Syncing {
		return "syncing"
	}
	if be.Contacts.Error != "" || be.Channels.Error != "" {
		return "error"
	}
	if be.Contacts.SyncedAt.IsZero() && be.Channels.SyncedAt.IsZero() {
		return "empty"
	}
	return "fresh"
}

func contactsLabel(c ReplicaInfo) string {
	if c.Syncing {
		return replicaSyncProgress(c)
	}
	if c.Error != "" {
		return c.Error
	}
	if c.SyncedAt.IsZero() {
		return "not replicated"
	}
	return fmt.Sprintf("%d, updated %s", c.Count, RelativeTime(c.SyncedAt))
}

func channelsLabel(c ReplicaInfo) string {
	if c.Syncing {
		return "replicating"
	}
	if c.Error != "" {
		return c.Error
	}
	if c.SyncedAt.IsZero() {
		return "not replicated"
	}
	return fmt.Sprintf("%d, updated %s", c.Count, RelativeTime(c.SyncedAt))
}

func replicaSyncProgress(contacts ReplicaInfo) string {
	if contacts.SyncTotal > 0 {
		pct := contacts.SyncReceived * 100 / contacts.SyncTotal
		return fmt.Sprintf("replicating %d%% (%d/%d)", pct, contacts.SyncReceived, contacts.SyncTotal)
	}
	if contacts.SyncReceived > 0 {
		return fmt.Sprintf("replicating (%d/?)", contacts.SyncReceived)
	}
	return "replicating"
}

func RadioIOLabel(io RadioIOInfo) string {
	if !io.Active {
		return "idle"
	}
	label := radioMethodLabel(io.Method)
	if io.DurationMs <= 0 {
		return label
	}
	return fmt.Sprintf("%s (%s)", label, formatDuration(time.Duration(io.DurationMs)*time.Millisecond))
}

func radioMethodLabel(method string) string {
	switch method {
	case "replicate":
		return "replicate"
	case "watch_raw":
		return "watch raw"
	case "send_text":
		return "send"
	case "repeater_status":
		return "repeater status"
	case "repeater_exec":
		return "repeater exec"
	case "repeater_login":
		return "repeater login"
	case "channel_send":
		return "channel send"
	case "keepalive":
		return "keepalive"
	default:
		return strings.ReplaceAll(method, "_", " ")
	}
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	if s == 0 {
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%dm%ds", m, s)
}

func formatCapabilities(items []string) string {
	if len(items) == 0 {
		return "-"
	}
	sorted := append([]string(nil), items...)
	sort.Strings(sorted)
	return strings.Join(sorted, ", ")
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

func deviceShortID(key string) string {
	key = strings.TrimSpace(key)
	if len(key) >= 8 {
		return strings.ToUpper(key[:8])
	}
	if key != "" {
		return strings.ToUpper(key)
	}
	return "UNKNOWN"
}
