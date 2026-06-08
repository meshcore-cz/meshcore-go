package ui

import (
	"fmt"
	"strings"
	"time"
)

// DeviceListRow is one rendered device list row.
type DeviceListRow struct {
	Profile   string
	Selected  bool
	Device    string
	Backend   string
	Radio     string
	Replica   string
	Transport string
	Activity  string
	Endpoint  string
}

// DeviceListMeta describes footer metadata for a device list.
type DeviceListMeta struct {
	Count    int
	Selected string
	Ready    int
	Degraded int
	Stopped  int
}

// DeviceListData is the input model for device list rendering.
type DeviceListData struct {
	Devices []DeviceListRow
	Meta    DeviceListMeta
	Wide    bool
}

// RenderDeviceList renders a device list table and footer.
func RenderDeviceList(data DeviceListData, printer Printer) string {
	theme := NewTheme(printer.Out)
	var b strings.Builder
	if data.Wide {
		renderWideDeviceTable(&b, data.Devices, theme)
	} else {
		renderDefaultDeviceTable(&b, data.Devices, theme)
	}
	b.WriteString("\n")
	b.WriteString(theme.Dim(formatDeviceListFooter(data.Meta)))
	b.WriteString("\n")
	return b.String()
}

func renderDefaultDeviceTable(b *strings.Builder, rows []DeviceListRow, theme Theme) {
	widths := defaultDeviceColumnWidths(rows)
	writeDeviceHeader(b, defaultDeviceHeaders(), widths)
	for _, row := range rows {
		writeDeviceRow(b, deviceRowCells(row, theme, false), widths, theme)
	}
}

func renderWideDeviceTable(b *strings.Builder, rows []DeviceListRow, theme Theme) {
	widths := wideDeviceColumnWidths(rows)
	writeDeviceHeader(b, wideDeviceHeaders(), widths)
	for _, row := range rows {
		writeDeviceRow(b, deviceRowCells(row, theme, true), widths, theme)
	}
}

func defaultDeviceHeaders() []string {
	return []string{"", "PROFILE", "DEVICE", "TRANSPORT", "SESSION", "RADIO", "LOCAL", "ACTIVITY"}
}

func wideDeviceHeaders() []string {
	return []string{"", "PROFILE", "DEVICE", "TRANSPORT", "SESSION", "RADIO", "LOCAL", "ENDPOINT", "ACTIVITY"}
}

const (
	deviceSelectCol    = 0
	deviceProfileCol   = 1
	deviceIDCol        = 2
	deviceTransportCol = 3
	deviceBackendCol   = 4
	deviceRadioCol     = 5
	deviceReplicaCol   = 6
	deviceActivityCol  = 7
	deviceEndpointCol  = 7
)

func defaultDeviceColumnWidths(rows []DeviceListRow) []int {
	widths := []int{1, 12, 8, 9, 9, 13, 7, 18}
	headers := defaultDeviceHeaders()
	for i, header := range headers {
		widths[i] = max(widths[i], DisplayWidth(header))
	}
	for _, row := range rows {
		widths[deviceSelectCol] = max(widths[deviceSelectCol], DisplayWidth(selectionLabel(row)))
		widths[deviceProfileCol] = max(widths[deviceProfileCol], DisplayWidth(row.Profile))
		widths[deviceIDCol] = max(widths[deviceIDCol], DisplayWidth(row.Device))
		widths[deviceTransportCol] = max(widths[deviceTransportCol], DisplayWidth(row.Transport))
		widths[deviceBackendCol] = max(widths[deviceBackendCol], DisplayWidth(row.Backend))
		widths[deviceRadioCol] = max(widths[deviceRadioCol], DisplayWidth(row.Radio))
		widths[deviceReplicaCol] = max(widths[deviceReplicaCol], DisplayWidth(row.Replica))
		widths[deviceActivityCol] = max(widths[deviceActivityCol], DisplayWidth(row.Activity))
	}
	return widths
}

func wideDeviceColumnWidths(rows []DeviceListRow) []int {
	widths := defaultDeviceColumnWidths(rows)
	widths = append(widths[:deviceActivityCol], append([]int{34}, widths[deviceActivityCol])...)
	for i, header := range wideDeviceHeaders() {
		if i < len(widths) {
			widths[i] = max(widths[i], DisplayWidth(header))
		}
	}
	for _, row := range rows {
		widths[deviceEndpointCol] = max(widths[deviceEndpointCol], DisplayWidth(row.Endpoint))
	}
	return widths
}

type deviceCell struct {
	text    string
	health  Health
	dimDash bool
	dim     bool
}

func deviceRowCells(row DeviceListRow, theme Theme, wide bool) []deviceCell {
	cells := []deviceCell{
		{text: selectionLabel(row)},
		{text: row.Profile},
		{text: row.Device, dimDash: true},
		{text: row.Transport},
		{text: row.Backend, health: deviceBackendHealth(row.Backend)},
		{text: row.Radio, health: deviceRadioHealth(row.Radio), dimDash: true},
		{text: row.Replica, health: deviceReplicaHealth(row.Replica)},
	}
	if wide {
		cells = append(cells, deviceCell{text: row.Endpoint, dimDash: true})
	}
	cells = append(cells, deviceActivityCell(row))
	return cells
}

func deviceActivityCell(row DeviceListRow) deviceCell {
	return deviceCell{
		text:    row.Activity,
		health:  deviceActivityHealth(row.Activity),
		dimDash: row.Activity == "-",
		dim:     strings.HasPrefix(row.Activity, "last "),
	}
}

func selectionLabel(row DeviceListRow) string {
	if row.Selected {
		return "*"
	}
	return ""
}

func deviceBackendHealth(state string) Health {
	switch state {
	case "ready":
		return HealthOK
	case "degraded":
		return HealthWarning
	case "stopped":
		return HealthUnknown
	default:
		return HealthUnknown
	}
}

func deviceRadioHealth(state string) Health {
	switch state {
	case "connected":
		return HealthOK
	case "reconnecting":
		return HealthWarning
	default:
		return HealthUnknown
	}
}

func deviceReplicaHealth(state string) Health {
	switch state {
	case "fresh", "available":
		return HealthOK
	case "syncing":
		return HealthWarning
	case "stale":
		return HealthWarning
	case "unknown":
		return HealthUnknown
	default:
		return HealthUnknown
	}
}

func deviceActivityHealth(state string) Health {
	switch {
	case state == "idle":
		return HealthOK
	case state == "-", state == "":
		return HealthUnknown
	case strings.HasPrefix(state, "last "):
		return HealthUnknown
	case state == "reconnecting", strings.HasPrefix(state, "syncing"):
		return HealthWarning
	default:
		return HealthWarning
	}
}

func writeDeviceHeader(b *strings.Builder, headers []string, widths []int) {
	cells := make([]deviceCell, len(headers))
	for i, header := range headers {
		cells[i] = deviceCell{text: header}
	}
	writeDeviceRow(b, cells, widths, Theme{})
}

func writeDeviceRow(b *strings.Builder, cells []deviceCell, widths []int, theme Theme) {
	for i, cell := range cells {
		width := 0
		if i < len(widths) {
			width = widths[i]
		}
		b.WriteString(formatDeviceCell(cell, width, theme))
		if i < len(cells)-1 {
			b.WriteByte(' ')
		}
	}
	b.WriteString("\n")
}

func formatDeviceCell(cell deviceCell, width int, theme Theme) string {
	text := cell.text
	if width <= 0 {
		return styleDeviceCell(cell, text, theme)
	}
	pad := width - DisplayWidth(text)
	if pad < 0 {
		pad = 0
	}
	return styleDeviceCell(cell, text, theme) + strings.Repeat(" ", pad)
}

func styleDeviceCell(cell deviceCell, text string, theme Theme) string {
	switch {
	case cell.health != HealthUnknown && text != "" && text != "-":
		return theme.StatusWord(cell.health, text)
	case cell.dim || (cell.dimDash && text == "-"):
		return theme.Dim(text)
	default:
		return text
	}
}

func formatDeviceListFooter(meta DeviceListMeta) string {
	parts := []string{fmt.Sprintf("%d devices", meta.Count)}
	if meta.Selected != "" {
		parts = append(parts, meta.Selected+" selected")
	}
	if meta.Ready > 0 {
		parts = append(parts, fmt.Sprintf("%d ready", meta.Ready))
	}
	if meta.Degraded > 0 {
		parts = append(parts, fmt.Sprintf("%d degraded", meta.Degraded))
	}
	if meta.Stopped > 0 {
		parts = append(parts, fmt.Sprintf("%d stopped", meta.Stopped))
	}
	return strings.Join(parts, " · ")
}

// DeviceShortID returns an uppercase 8-character device identifier from a key prefix.
func DeviceShortID(key string) string {
	return deviceShortID(key)
}

// HumanTransport renders a transport name for tables.
func HumanTransport(transport string) string {
	switch strings.ToLower(strings.TrimSpace(transport)) {
	case "ble":
		return "BLE"
	case "serial":
		return "SERIAL"
	case "tcp":
		return "TCP"
	case "ws", "wss":
		return strings.ToUpper(transport)
	case "":
		return "-"
	default:
		return strings.ToUpper(transport)
	}
}

// DeviceListBackendState derives the backend column for one profile.
func DeviceListBackendState(active bool, running bool, state string) string {
	if !active || !running {
		return "stopped"
	}
	state = strings.TrimSpace(state)
	if state == "" {
		return "unknown"
	}
	return state
}

// DeviceListRadioState derives the radio column for one profile.
func DeviceListRadioState(active bool, running bool, healthy bool, state string) string {
	if !active || !running {
		return "-"
	}
	if state == "degraded" || !healthy {
		return "reconnecting"
	}
	if state == "ready" && healthy {
		return "connected"
	}
	return "-"
}

// DeviceListReplicaState derives the replica column for one profile.
func DeviceListReplicaState(active bool, running bool, healthy bool, state string, contacts, channels ReplicaInfo) string {
	if !active {
		return "unknown"
	}
	if !running {
		return "unknown"
	}
	if contacts.Syncing || channels.Syncing {
		return "syncing"
	}
	if state == "degraded" || !healthy {
		return "stale"
	}
	if contacts.Error != "" || channels.Error != "" {
		return "stale"
	}
	if contacts.SyncedAt.IsZero() && channels.SyncedAt.IsZero() {
		return "unknown"
	}
	return "fresh"
}

// DeviceListActivityState derives the activity column for one profile.
func DeviceListActivityState(active bool, running bool, state string, contacts, channels ReplicaInfo, radio RadioIOInfo) string {
	if !active || !running {
		return "-"
	}
	if state == "degraded" {
		return "reconnecting"
	}
	if contacts.Syncing {
		return "syncing contacts"
	}
	if channels.Syncing {
		return "syncing channels"
	}
	if radio.Active {
		label := radioMethodLabel(radio.Method)
		if radio.DurationMs > 0 {
			return label + " · " + formatRunningDuration(time.Duration(radio.DurationMs)*time.Millisecond)
		}
		return label
	}
	if !radio.LastAt.IsZero() {
		label := "last " + RelativeTime(radio.LastAt)
		if radio.LastMethod != "" {
			label += " (" + radioMethodLabel(radio.LastMethod) + ")"
		}
		return label
	}
	return "idle"
}
