package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

const statusLabelWidth = 14
const statusIndent = "  "

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
	Stats           DeviceStatsInfo
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
	Active         bool
	Method         string
	DurationMs     int64
	LastAt         time.Time
	LastMethod     string
	LastDurationMs int64
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
		b.WriteString(statusLine("Public key", orDash(strings.ToLower(strings.TrimSpace(data.Device.PublicKey)))))
		b.WriteString("\n")
		writeRadioSection(&b, data.Device, data.Backend, theme)
	} else {
		b.WriteString(statusLine("Device", "unavailable"))
		if data.Backend.URI != "" {
			b.WriteString(statusLine("Transport", data.Backend.URI))
		}
	}
	b.WriteString("\n")

	if data.Backend.Running {
		b.WriteString(statusLine("Backend", backendLabel(data.Backend, theme)))
		b.WriteString(statusSubLine("Activity", ActivityLabel(data.Backend.RadioIO, theme)))
		b.WriteString(statusSubLine("Replica", replicaLabel(data.Backend, theme)))
		b.WriteString("\n")
	} else if data.Device.Available {
		b.WriteString(statusLine("Backend", "not running"))
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

func statusSubLine(label, value string) string {
	return fmt.Sprintf("%s%-*s %s\n", statusIndent, statusLabelWidth-len(statusIndent), label+":", value)
}

func writeRadioSection(b *strings.Builder, dev DeviceInfo, backend BackendInfo, theme Theme) {
	if !hasRadioSection(dev) {
		return
	}
	health, word := radioSectionHealth(dev, backend)
	header := theme.StatusWord(health, word)
	if dev.Stats.Available && !dev.Stats.UpdatedAt.IsZero() {
		header += " · updated " + RelativeTime(dev.Stats.UpdatedAt)
	} else if !dev.Stats.Available {
		header += " · not synced"
	}
	b.WriteString(statusLine("Radio", header))
	if modem := modemLabel(dev.Radio); modem != "" {
		b.WriteString(statusSubLine("Modem", modem))
	}
	for _, line := range deviceStatsLines(dev.Stats) {
		b.WriteString(line)
	}
}

func hasRadioSection(dev DeviceInfo) bool {
	return modemLabel(dev.Radio) != "" || dev.Stats.Available
}

func radioSectionHealth(dev DeviceInfo, backend BackendInfo) (Health, string) {
	if dev.Stats.Available {
		if dev.Stats.Core.ErrorFlags != 0 {
			return HealthError, "error"
		}
		if backend.Running && !backend.Healthy {
			return HealthError, "error"
		}
		if !dev.Stats.UpdatedAt.IsZero() && time.Since(dev.Stats.UpdatedAt) > 90*time.Second {
			return HealthWarning, "stale"
		}
		return HealthOK, "ok"
	}
	if backend.Running && !backend.Healthy {
		return HealthError, "unavailable"
	}
	return HealthWarning, "pending"
}

func deviceStatsLines(stats DeviceStatsInfo) []string {
	if !stats.Available {
		return nil
	}
	var lines []string
	add := func(label, value string) {
		if value != "" {
			lines = append(lines, statusSubLine(label, value))
		}
	}
	add("Signal", signalLabel(stats))
	add("Battery", batteryLabel(stats))
	add("Uptime", uptimeLabel(stats))
	add("Packets", packetsLabel(stats))
	add("Airtime", airtimeLabel(stats))
	add("Queue", queueLabel(stats))
	return lines
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

func modemLabel(r RadioInfo) string {
	return radioLabel(r)
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
		parts = append(parts, "BW "+FormatBandwidthHz(r.BandwidthKHz))
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
	word := theme.StatusWord(replicaHealth(be), replicaStateWord(be))
	if details := replicaDetails(be); details != "" {
		return word + " · " + details
	}
	return word
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

func replicaDetails(be BackendInfo) string {
	switch replicaStateWord(be) {
	case "fresh":
		parts := []string{
			fmt.Sprintf("%d contacts", be.Contacts.Count),
			fmt.Sprintf("%d channels", be.Channels.Count),
		}
		if updated := replicaLatestUpdate(be.Contacts, be.Channels); !updated.IsZero() {
			parts = append(parts, "updated "+RelativeTime(updated))
		}
		return strings.Join(parts, " · ")
	default:
		return strings.Join([]string{
			replicaItemDetail("contacts", be.Contacts),
			replicaItemDetail("channels", be.Channels),
		}, " · ")
	}
}

func replicaItemDetail(kind string, c ReplicaInfo) string {
	if c.Syncing {
		if kind == "contacts" {
			return "contacts " + replicaSyncProgress(c)
		}
		return "channels replicating"
	}
	if c.Error != "" {
		return kind + " " + c.Error
	}
	if c.SyncedAt.IsZero() {
		return kind + " not replicated"
	}
	if kind == "contacts" {
		return fmt.Sprintf("%d contacts", c.Count)
	}
	return fmt.Sprintf("%d channels", c.Count)
}

func replicaLatestUpdate(a, b ReplicaInfo) time.Time {
	latest := a.SyncedAt
	if b.SyncedAt.After(latest) {
		latest = b.SyncedAt
	}
	return latest
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

func ActivityLabel(io RadioIOInfo, theme Theme) string {
	if io.Active {
		word := theme.StatusWord(HealthWarning, radioMethodLabel(io.Method))
		if io.DurationMs <= 0 {
			return word
		}
		return word + " (" + formatRunningDuration(time.Duration(io.DurationMs)*time.Millisecond) + ")"
	}

	idle := theme.StatusWord(HealthOK, "idle")
	if io.LastAt.IsZero() {
		return idle
	}
	parts := []string{idle}
	if io.LastMethod != "" {
		last := "last: " + radioMethodLabel(io.LastMethod)
		if io.LastDurationMs > 0 {
			last += " (" + formatDuration(time.Duration(io.LastDurationMs)*time.Millisecond) + ")"
		}
		parts = append(parts, last)
	}
	parts = append(parts, RelativeTime(io.LastAt))
	return strings.Join(parts, " · ")
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
	case "stats":
		return "stats"
	default:
		return strings.ReplaceAll(method, "_", " ")
	}
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		ms := d.Round(time.Millisecond).Milliseconds()
		if ms < 1 {
			ms = 1
		}
		return fmt.Sprintf("%dms", ms)
	}
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

func formatRunningDuration(d time.Duration) string {
	if d < time.Second {
		ms := d.Round(time.Millisecond).Milliseconds()
		if ms < 1 {
			ms = 1
		}
		return fmt.Sprintf("%dms", ms)
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	m := int(d.Minutes())
	s := d.Seconds() - float64(m*60)
	return fmt.Sprintf("%dm%.1fs", m, s)
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
