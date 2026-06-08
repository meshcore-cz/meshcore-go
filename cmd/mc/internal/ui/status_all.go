package ui

import (
	"fmt"
	"strings"
	"time"
)

// StatusAllRow is one compact row group for `mc status --all`.
type StatusAllRow struct {
	Profile     string
	Selected    bool
	Session     string
	Transport   string
	Radio       DeviceStatsInfo
	LocalState  LocalStateInfo
	Observer    string
	ConnectedAt time.Time
}

// StatusAllData is the input model for compact multi-device status output.
type StatusAllData struct {
	Rows []StatusAllRow
}

// RenderStatusAll renders `mc status --all`.
func RenderStatusAll(data StatusAllData, printer Printer) string {
	theme := NewTheme(printer.Out)
	var b strings.Builder
	for i, row := range data.Rows {
		if i > 0 {
			b.WriteString("\n")
		}
		prefix := "  "
		if row.Selected {
			prefix = "* "
		}
		b.WriteString(prefix + row.Profile + "\n")
		b.WriteString(statusBlockSubLine("Session", statusAllSessionLabel(row, theme)))
		if row.Transport != "" {
			b.WriteString(statusBlockSubLine("Transport", row.Transport))
		}
		if radio := statusAllRadioLabel(row.Radio, theme); radio != "" {
			b.WriteString(statusBlockSubLine("Radio", radio))
		}
		b.WriteString(statusBlockSubLine("Local state", statusAllLocalStateLabel(row.LocalState, theme)))
		if row.Observer != "" {
			b.WriteString(statusBlockSubLine("Observer", row.Observer))
		}
	}
	return b.String()
}

func statusAllSessionLabel(row StatusAllRow, theme Theme) string {
	state := strings.TrimSpace(row.Session)
	if state == "" || state == "stopped" {
		return "not running"
	}
	health := HealthWarning
	if state == "ready" || state == "bridge" {
		health = HealthOK
	}
	label := theme.StatusWord(health, state)
	if !row.ConnectedAt.IsZero() {
		label += " · " + theme.Dim("connected "+RelativeTime(row.ConnectedAt))
	}
	return label
}

func statusAllRadioLabel(stats DeviceStatsInfo, theme Theme) string {
	if !stats.Available {
		return ""
	}
	parts := []string{theme.StatusWord(HealthOK, "active")}
	if battery := batteryLabel(stats); battery != "" {
		parts = append(parts, "battery "+battery)
	}
	if !stats.UpdatedAt.IsZero() {
		parts = append(parts, theme.Dim("updated "+StatsUpdatedRelative(stats.UpdatedAt)))
	}
	return strings.Join(parts, " · ")
}

func statusAllLocalStateLabel(local LocalStateInfo, theme Theme) string {
	if !local.Initialized {
		return "not initialized"
	}
	parts := []string{
		fmt.Sprintf("%d contacts", local.Contacts),
		fmt.Sprintf("%d channels", local.Channels),
	}
	if !local.UpdatedAt.IsZero() {
		parts = append(parts, theme.Dim("updated "+RelativeTime(local.UpdatedAt)))
	}
	return strings.Join(parts, " · ")
}
