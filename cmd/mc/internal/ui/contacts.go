package ui

import (
	"fmt"
	"strings"
	"time"

	meshcore "github.com/meshcore-cz/meshcore-go"
	"github.com/meshcore-cz/meshcore-go/protocol/pathhash"
)

const contactDetailLabelWidth = 14

// ContactListMeta describes footer metadata for a contact list.
type ContactListMeta struct {
	Count        int
	Syncing      bool
	SyncReceived int
	SyncTotal    int
	SyncedAt     time.Time
	Error        string
	Cached       bool
}

// ContactRow is one rendered contact list row.
type ContactRow struct {
	Name      string
	Type      string
	Age       string
	Advert    string
	Route     string
	Path      string
	Distance  string
	Location  string
	PublicKey string
}

// ContactListData is the input model for contact list rendering.
type ContactListData struct {
	Contacts []ContactRow
	Meta     ContactListMeta
	Wide     bool
}

// ContactDetailData is the input model for contact detail rendering.
type ContactDetailData struct {
	Name        string
	Type        string
	PublicKey   string
	LastSeen    string
	LastSeenAbs string
	Location    string
	Distance    string
	AdvertPath  string
	Path        string
}

// ContactRowFromMeshcore builds a list row from a contact and optional origin coordinates.
func ContactRowFromMeshcore(ct meshcore.Contact, originLat, originLon float64, wide bool) ContactRow {
	key := strings.ToLower(strings.TrimSpace(ct.PublicKey))
	row := ContactRow{
		Name:      ct.Name,
		Type:      HumanContactType(ct.Type),
		Age:       ContactAge(ct.LastAdvert),
		Advert:    AdvertWidth(ct),
		Route:     RouteLabel(ct),
		Path:      PathLabel(ct),
		Distance:  DistanceLabel(originLat, originLon, ct.Latitude, ct.Longitude),
		Location:  LocationLabel(ct.Latitude, ct.Longitude),
		PublicKey: KeyPrefix(key, 12),
	}
	if wide {
		row.PublicKey = key
	}
	return row
}

// ContactDetailFromMeshcore builds detail view data from a contact.
func ContactDetailFromMeshcore(ct meshcore.Contact, originLat, originLon float64) ContactDetailData {
	key := strings.ToLower(strings.TrimSpace(ct.PublicKey))
	data := ContactDetailData{
		Name:       ct.Name,
		Type:       HumanContactType(ct.Type),
		PublicKey:  key,
		LastSeen:   ContactAge(ct.LastAdvert),
		Location:   LocationLabel(ct.Latitude, ct.Longitude),
		Distance:   DistanceLabel(originLat, originLon, ct.Latitude, ct.Longitude),
		AdvertPath: AdvertPathDetail(ct),
		Path:       PathLabel(ct),
	}
	if !ct.LastAdvert.IsZero() {
		data.LastSeenAbs = ct.LastAdvert.Format("2006-01-02 15:04:05")
	}
	return data
}

// HumanContactType maps SDK contact types to human-readable labels.
func HumanContactType(t meshcore.ContactType) string {
	switch t {
	case meshcore.ContactChat:
		return "companion"
	case meshcore.ContactRepeater:
		return "repeater"
	case meshcore.ContactRoom:
		return "room"
	case meshcore.ContactSensor:
		return "sensor"
	default:
		return "unknown"
	}
}

// AdvertWidth renders the observed advert path-hash width.
func AdvertWidth(ct meshcore.Contact) string {
	if ct.OutPathEnc == pathhash.OutPathUnknown {
		return "-"
	}
	return fmt.Sprintf("%dB", pathhash.HashSizeFromPathMeta(ct.OutPathEnc))
}

// RouteLabel renders flood, direct, or hop-count routing.
func RouteLabel(ct meshcore.Contact) string {
	if ct.OutPathEnc == pathhash.OutPathUnknown {
		return "flood"
	}
	hopCount := pathhash.HopCountFromPathMeta(ct.OutPathEnc)
	switch hopCount {
	case 0:
		return "direct"
	case 1:
		return "1 hop"
	default:
		return fmt.Sprintf("%d hops", hopCount)
	}
}

// PathLabel renders stored advert path segments or "-" when unavailable.
func PathLabel(ct meshcore.Contact) string {
	if ct.OutPathEnc == pathhash.OutPathUnknown {
		return "-"
	}
	hashSize := pathhash.HashSizeFromPathMeta(ct.OutPathEnc)
	hopCount := pathhash.HopCountFromPathMeta(ct.OutPathEnc)
	if hopCount == 0 {
		return "-"
	}
	path := pathhash.PathBytes(ct.OutPathEnc, ct.OutPath)
	hops := pathhash.Split(path, hashSize)
	if len(hops) == 0 {
		return "-"
	}
	parts := make([]string, len(hops))
	for i, hop := range hops {
		parts[i] = pathhash.FormatHop(hop)
	}
	return strings.Join(parts, " → ")
}

// LocationLabel renders coordinates or "-".
func LocationLabel(lat, lon float64) string {
	if !HasCoordinates(lat, lon) {
		return "-"
	}
	return fmt.Sprintf("%.6f, %.6f", lat, lon)
}

// DistanceLabel renders straight-line distance from origin to contact.
func DistanceLabel(originLat, originLon, lat, lon float64) string {
	if !HasCoordinates(originLat, originLon) || !HasCoordinates(lat, lon) {
		return "-"
	}
	return FormatDistance(DistanceKM(originLat, originLon, lat, lon))
}

// KeyPrefix returns the first n hex characters of a public key.
func KeyPrefix(k string, n int) string {
	k = strings.ToLower(strings.TrimSpace(k))
	if k == "" {
		return "-"
	}
	if len(k) <= n {
		return k
	}
	return k[:n]
}

// AdvertPathDetail renders the advert path summary for detail view.
func AdvertPathDetail(ct meshcore.Contact) string {
	if ct.OutPathEnc == pathhash.OutPathUnknown {
		return "flood"
	}
	hashSize := pathhash.HashSizeFromPathMeta(ct.OutPathEnc)
	hopCount := pathhash.HopCountFromPathMeta(ct.OutPathEnc)
	hashLabel := fmt.Sprintf("%d-byte hashes", hashSize)
	switch hopCount {
	case 0:
		return "direct · " + hashLabel
	case 1:
		return "1 hop · " + hashLabel
	default:
		return fmt.Sprintf("%d hops · %s", hopCount, hashLabel)
	}
}

// RenderContacts renders a contact list table and footer.
func RenderContacts(data ContactListData, printer Printer) string {
	theme := NewTheme(printer.Out)
	var b strings.Builder
	if data.Wide {
		renderWideContactTable(&b, data.Contacts, theme)
	} else {
		renderDefaultContactTable(&b, data.Contacts, theme)
	}
	b.WriteString("\n")
	b.WriteString(theme.Dim(formatContactListFooter(data.Meta)))
	b.WriteString("\n")
	return b.String()
}

// RenderContactDetail renders a single contact detail view.
func RenderContactDetail(data ContactDetailData, printer Printer) string {
	theme := NewTheme(printer.Out)
	var b strings.Builder
	b.WriteString(contactDetailLine("Name:", data.Name))
	b.WriteString(contactDetailLine("Type:", data.Type))
	b.WriteString(contactDetailLine("Public key:", data.PublicKey))
	b.WriteString(contactDetailLastSeen(data, theme))
	b.WriteString(contactDetailLine("Location:", styleDetailValue(data.Location, theme)))
	b.WriteString(contactDetailLine("Distance:", styleDetailValue(data.Distance, theme)))
	b.WriteString("\n")
	b.WriteString(contactDetailLine("Advert path:", styleRouteValue(data.AdvertPath, theme)))
	b.WriteString(contactDetailLine("Path:", styleDetailValue(data.Path, theme)))
	return b.String()
}

func contactDetailLastSeen(data ContactDetailData, theme Theme) string {
	seen := data.LastSeen
	if data.LastSeenAbs != "" {
		seen = data.LastSeen + "  (" + data.LastSeenAbs + ")"
	}
	return contactDetailLine("Last seen:", styleDetailValue(seen, theme))
}

func contactDetailLine(label, value string) string {
	return fmt.Sprintf("%-*s %s\n", contactDetailLabelWidth, label, value)
}

func styleDetailValue(value string, theme Theme) string {
	if value == "-" || value == "never" {
		return theme.Dim(value)
	}
	return value
}

func styleRouteValue(value string, theme Theme) string {
	if value == "flood" {
		return theme.Dim(value)
	}
	return value
}

func renderDefaultContactTable(b *strings.Builder, rows []ContactRow, theme Theme) {
	widths := defaultContactColumnWidths(rows)
	writeContactHeader(b, defaultContactHeaders(), widths)
	for _, row := range rows {
		writeContactRow(b, []contactCell{
			{text: row.PublicKey, dim: true},
			{text: row.Name},
			{text: row.Type},
			{text: row.Advert, dimDash: true},
			{text: row.Route, dimFlood: true},
			{text: row.Distance, dimDash: true},
			{text: row.Age, dimNever: true},
		}, widths, theme)
	}
}

func renderWideContactTable(b *strings.Builder, rows []ContactRow, theme Theme) {
	widths := wideContactColumnWidths(rows)
	writeContactHeader(b, wideContactHeaders(widths[widePathCol]), widths)
	for _, row := range rows {
		writeContactRow(b, []contactCell{
			{text: row.PublicKey},
			{text: row.Name},
			{text: row.Type},
			{text: row.Advert, dimDash: true},
			{text: row.Route, dimFlood: true},
			{text: row.Path, dimDash: true},
			{text: row.Distance, dimDash: true},
			{text: row.Location, dimDash: true},
			{text: row.Age, dimNever: true},
		}, widths, theme)
	}
}

const (
	keyCol   = 0
	nameCol  = 1
	typeCol  = 2
	advCol   = 3
	routeCol = 4
	distCol  = 5
	ageCol   = 6

	wideKeyCol      = 0
	widePathCol     = 5
	wideDistCol     = 6
	wideLocationCol = 7
	wideAgeCol      = 8
)

func defaultContactHeaders() []string {
	return []string{"KEY", "NAME", "TYPE", "ADV", "ROUTE", "DIST", "AGE"}
}

func wideContactHeaders(pathW int) []string {
	_ = pathW
	return []string{"KEY", "NAME", "TYPE", "ADV", "ROUTE", "PATH", "DIST", "LOCATION", "AGE"}
}

func defaultContactColumnWidths(rows []ContactRow) []int {
	widths := []int{12, 26, 10, 5, 9, 9, 11}
	headers := defaultContactHeaders()
	for i, header := range headers {
		widths[i] = max(widths[i], DisplayWidth(header))
	}
	for _, row := range rows {
		widths[keyCol] = max(widths[keyCol], DisplayWidth(row.PublicKey))
		widths[nameCol] = max(widths[nameCol], DisplayWidth(row.Name))
		widths[typeCol] = max(widths[typeCol], DisplayWidth(row.Type))
		widths[advCol] = max(widths[advCol], DisplayWidth(row.Advert))
		widths[routeCol] = max(widths[routeCol], DisplayWidth(row.Route))
		widths[distCol] = max(widths[distCol], DisplayWidth(row.Distance))
		widths[ageCol] = max(widths[ageCol], DisplayWidth(row.Age))
	}
	return widths
}

func wideContactColumnWidths(rows []ContactRow) []int {
	widths := []int{0, 18, 10, 5, 7, 18, 8, 22, 9}
	headers := wideContactHeaders(widths[widePathCol])
	for i, header := range headers {
		if i == wideKeyCol {
			continue
		}
		widths[i] = max(widths[i], DisplayWidth(header))
	}
	for _, row := range rows {
		widths[nameCol] = max(widths[nameCol], DisplayWidth(row.Name))
		widths[typeCol] = max(widths[typeCol], DisplayWidth(row.Type))
		widths[advCol] = max(widths[advCol], DisplayWidth(row.Advert))
		widths[routeCol] = max(widths[routeCol], DisplayWidth(row.Route))
		widths[widePathCol] = max(widths[widePathCol], DisplayWidth(row.Path))
		widths[wideDistCol] = max(widths[wideDistCol], DisplayWidth(row.Distance))
		widths[wideLocationCol] = max(widths[wideLocationCol], DisplayWidth(row.Location))
		widths[wideAgeCol] = max(widths[wideAgeCol], DisplayWidth(row.Age))
	}
	return widths
}

type contactCell struct {
	text     string
	dim      bool
	dimDash  bool
	dimNever bool
	dimFlood bool
}

func writeContactHeader(b *strings.Builder, headers []string, widths []int) {
	cells := make([]contactCell, len(headers))
	for i, header := range headers {
		cells[i] = contactCell{text: header}
	}
	writeContactRow(b, cells, widths, Theme{})
}

func writeContactRow(b *strings.Builder, cells []contactCell, widths []int, theme Theme) {
	for i, cell := range cells {
		width := 0
		if i < len(widths) {
			width = widths[i]
		}
		b.WriteString(formatContactCell(cell, width, theme))
		if i < len(cells)-1 {
			b.WriteByte(' ')
		}
	}
	b.WriteString("\n")
}

func formatContactCell(cell contactCell, width int, theme Theme) string {
	text := cell.text
	if width <= 0 {
		if cell.dim {
			return theme.Dim(text)
		}
		return text
	}
	pad := width - DisplayWidth(text)
	if pad < 0 {
		pad = 0
	}
	spaces := strings.Repeat(" ", pad)
	switch {
	case cell.dim:
		return theme.Dim(text) + spaces
	case cell.dimDash && text == "-":
		return theme.Dim(text) + spaces
	case cell.dimNever && text == "never":
		return theme.Dim(text) + spaces
	case cell.dimFlood && text == "flood":
		return theme.Dim(text) + spaces
	default:
		return text + spaces
	}
}

func padDisplayWidth(text string, width int) string {
	w := DisplayWidth(text)
	if w >= width {
		return text
	}
	return text + strings.Repeat(" ", width-w)
}

func formatContactListFooter(meta ContactListMeta) string {
	parts := []string{fmt.Sprintf("%d contacts", meta.Count)}
	if meta.Cached {
		parts = append(parts, "local state")
	}
	if meta.Syncing && meta.SyncTotal > 0 {
		parts = append(parts, fmt.Sprintf("syncing %d/%d", meta.SyncReceived, meta.SyncTotal))
	}
	if !meta.SyncedAt.IsZero() {
		parts = append(parts, "last advert "+RelativeTime(meta.SyncedAt))
	}
	if meta.Error != "" {
		if !meta.Cached {
			parts = append(parts, "local state")
		}
		parts = append(parts, "sync error: "+meta.Error)
	}
	return strings.Join(parts, " · ")
}
