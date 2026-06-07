package ui

import (
	"bytes"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	meshcore "github.com/meshcore-cz/meshcore-go"
	"github.com/meshcore-cz/meshcore-go/protocol/pathhash"
)

func contactWithMeta(enc byte, path []byte) meshcore.Contact {
	return meshcore.Contact{
		OutPathEnc: enc,
		OutPath:    path,
	}
}

func TestHumanContactType(t *testing.T) {
	tests := []struct {
		in   meshcore.ContactType
		want string
	}{
		{meshcore.ContactChat, "companion"},
		{meshcore.ContactRepeater, "repeater"},
		{meshcore.ContactRoom, "room"},
		{meshcore.ContactSensor, "sensor"},
		{meshcore.ContactUnknown, "unknown"},
	}
	for _, tc := range tests {
		if got := HumanContactType(tc.in); got != tc.want {
			t.Errorf("HumanContactType(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRouteMetadata(t *testing.T) {
	tests := []struct {
		name      string
		enc       byte
		path      []byte
		wantAdv   string
		wantRoute string
		wantPath  string
	}{
		{"flood", pathhash.OutPathUnknown, nil, "-", "flood", "-"},
		{"direct 1B", 0x00, nil, "1B", "direct", "-"},
		{"direct 2B", 0x40, nil, "2B", "direct", "-"},
		{"direct 3B", 0x80, nil, "3B", "direct", "-"},
		{"1 hop", 0x01, []byte{0xa9}, "1B", "1 hop", "a9"},
		{"3 hops 2B", 0x43, []byte{0xa9, 0x0d, 0x57, 0xdb, 0x3f, 0x18, 0x00, 0x00}, "2B", "3 hops", "a90d → 57db → 3f18"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ct := contactWithMeta(tc.enc, tc.path)
			if got := AdvertWidth(ct); got != tc.wantAdv {
				t.Errorf("AdvertWidth() = %q, want %q", got, tc.wantAdv)
			}
			if got := RouteLabel(ct); got != tc.wantRoute {
				t.Errorf("RouteLabel() = %q, want %q", got, tc.wantRoute)
			}
			if got := PathLabel(ct); got != tc.wantPath {
				t.Errorf("PathLabel() = %q, want %q", got, tc.wantPath)
			}
		})
	}
}

func TestDistanceLabel(t *testing.T) {
	originLat, originLon := 50.0755, 14.4378
	if got := DistanceLabel(0, 0, 50.0, 14.0); got != "-" {
		t.Fatalf("missing origin = %q, want -", got)
	}
	if got := DistanceLabel(originLat, originLon, 0, 0); got != "-" {
		t.Fatalf("missing contact location = %q, want -", got)
	}
	if got := DistanceLabel(originLat, originLon, originLat, originLon); got != "0 m" {
		t.Fatalf("same coordinates = %q, want 0 m", got)
	}
	kmNear := DistanceKM(originLat, originLon, originLat+0.00665, originLon)
	if kmNear < 0.73 || kmNear > 0.75 {
		t.Fatalf("sub-km distance = %.3f km, want ~0.74 km", kmNear)
	}
	if got := FormatDistance(kmNear); got != "740 m" && got != "739 m" {
		t.Fatalf("sub-km = %q, want 740 m", got)
	}
	kmMid := DistanceKM(originLat, originLon, originLat+0.0372, originLon)
	if got := FormatDistance(kmMid); got != "4.1 km" {
		t.Fatalf("4.14 km = %q, want 4.1 km", got)
	}
	if got := FormatDistance(61.7); got != "62 km" {
		t.Fatalf("61.7 km = %q, want 62 km", got)
	}
}

func TestKeyPrefix(t *testing.T) {
	if got := KeyPrefix("3B46CE7CB2F3ABCDEF", 12); got != "3b46ce7cb2f3" {
		t.Fatalf("got %q", got)
	}
	if got := KeyPrefix("", 12); got != "-" {
		t.Fatalf("empty = %q", got)
	}
}

func TestFormatContactListFooter(t *testing.T) {
	tests := []struct {
		name string
		meta ContactListMeta
		want string
	}{
		{
			name: "synced",
			meta: ContactListMeta{Count: 449, SyncedAt: time.Now().Add(-11 * time.Minute)},
			want: "449 contacts · last advert",
		},
		{
			name: "no sync time",
			meta: ContactListMeta{Count: 449},
			want: "449 contacts",
		},
		{
			name: "syncing",
			meta: ContactListMeta{
				Count:        449,
				Syncing:      true,
				SyncReceived: 183,
				SyncTotal:    449,
				SyncedAt:     time.Now().Add(-11 * time.Minute),
			},
			want: "449 contacts · syncing 183/449 · last advert",
		},
		{
			name: "cached",
			meta: ContactListMeta{
				Count:    449,
				Cached:   true,
				SyncedAt: time.Now().Add(-10*time.Hour - 56*time.Minute),
			},
			want: "449 contacts · cached · last advert",
		},
		{
			name: "sync error",
			meta: ContactListMeta{Count: 449, Cached: true, Error: "timeout"},
			want: "449 contacts · cached · sync error: timeout",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := formatContactListFooter(tc.meta)
			if !strings.HasPrefix(got, tc.want) {
				t.Fatalf("got %q, want prefix %q", got, tc.want)
			}
		})
	}
}

func TestContactTableAlignment(t *testing.T) {
	rows := []ContactRow{
		{Name: "OK2USB_RPT_Brno", Type: "repeater", Age: "18h", Advert: "-", Route: "flood", Distance: "-", PublicKey: "86f8616fbaee"},
		{Name: "OK5TVR☣️", Type: "companion", Age: "695d", Advert: "-", Route: "flood", Distance: "-", PublicKey: "aaaaaaaaaaaa"},
		{Name: "OKROU.SOLAR☀️", Type: "repeater", Age: "59m", Advert: "-", Route: "flood", Distance: "-", PublicKey: "bbbbbbbbbbbb"},
		{Name: "OVA-FUTURUM 🗼🔋☀️", Type: "repeater", Age: "1d", Advert: "-", Route: "flood", Distance: "-", PublicKey: "cccccccccccc"},
		{Name: "Ovenecká ☀️🔋🗼", Type: "repeater", Age: "2h", Advert: "-", Route: "flood", Distance: "-", PublicKey: "dddddddddddd"},
		{Name: "olany.meshcore.cz", Type: "repeater", Age: "3d", Advert: "-", Route: "flood", Distance: "-", PublicKey: "eeeeeeeeeeee"},
	}
	widths := defaultContactColumnWidths(rows)
	data := ContactListData{
		Contacts: rows,
		Meta:     ContactListMeta{Count: len(rows)},
	}
	out := RenderContacts(data, NewPrinter(bytes.NewBuffer(nil)))
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) < 4 {
		t.Fatalf("expected header + rows + blank + footer, got %d lines:\n%s", len(lines), out)
	}

	headerCells := splitContactTableLine(lines[0], widths)
	for i, row := range rows {
		line := lines[i+1]
		cells := splitContactTableLine(line, widths)
		if len(cells) != len(widths) {
			t.Fatalf("row %d column count = %d, want %d:\n%s", i+1, len(cells), len(widths), line)
		}
		if cells[keyCol] != row.PublicKey {
			t.Fatalf("row %d key = %q, want %q", i+1, cells[keyCol], row.PublicKey)
		}
		if cells[nameCol] != row.Name {
			t.Fatalf("row %d name = %q, want %q", i+1, cells[nameCol], row.Name)
		}
		if cells[routeCol] != row.Route {
			t.Fatalf("row %d route = %q, want %q", i+1, cells[routeCol], row.Route)
		}
		if cells[ageCol] != row.Age {
			t.Fatalf("row %d age = %q, want %q", i+1, cells[ageCol], row.Age)
		}
	}
	_ = headerCells
}

func splitContactTableLine(line string, widths []int) []string {
	cells := make([]string, len(widths))
	idx := 0
	for i, width := range widths {
		if idx >= len(line) {
			break
		}
		start := idx
		for idx < len(line) && DisplayWidth(line[start:idx]) < width {
			_, size := utf8.DecodeRuneInString(line[idx:])
			if size == 0 {
				idx++
				continue
			}
			idx += size
		}
		for idx < len(line) && line[idx] == ' ' && DisplayWidth(line[start:idx]) < width {
			idx++
		}
		cells[i] = strings.TrimSpace(line[start:idx])
		if idx < len(line) && line[idx] == ' ' {
			idx++
		}
	}
	return cells
}

func TestFormatContactCellStyledPadding(t *testing.T) {
	theme := NewTheme(bytes.NewBuffer(nil))
	theme.enabled = true

	got := formatContactCell(contactCell{text: "flood", dimFlood: true}, 9, theme)
	if !strings.HasPrefix(stripANSI(got), "flood") {
		t.Fatalf("flood prefix = %q", got)
	}
	if DisplayWidth(stripANSI(got)) != 9 {
		t.Fatalf("styled flood width = %d, want 9 (%q)", DisplayWidth(stripANSI(got)), got)
	}

	got = formatContactCell(contactCell{text: "-", dimDash: true}, 5, theme)
	if DisplayWidth(stripANSI(got)) != 5 {
		t.Fatalf("styled dash width = %d, want 5 (%q)", DisplayWidth(stripANSI(got)), got)
	}

	got = formatContactCell(contactCell{text: "mctt_📟rep19"}, 26, theme)
	if DisplayWidth(got) != 26 {
		t.Fatalf("emoji name width = %d, want 26 (%q)", DisplayWidth(got), got)
	}
}

func stripANSI(s string) string {
	var b strings.Builder
	esc := false
	for _, r := range s {
		if esc {
			if r == 'm' {
				esc = false
			}
			continue
		}
		if r == '\x1b' {
			esc = true
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func TestRenderContactsDefault(t *testing.T) {
	data := ContactListData{
		Contacts: []ContactRow{
			{
				Name:      "AKAT-Tester1 🗼",
				Type:      "repeater",
				Age:       ContactAge(time.Now().Add(-6 * time.Minute)),
				Advert:    "2B",
				Route:     "3 hops",
				Distance:  "62 km",
				PublicKey: "57dbabc15c30",
			},
			{
				Name:      "011111000101",
				Type:      "companion",
				Age:       "18d",
				Advert:    "-",
				Route:     "flood",
				Distance:  "-",
				PublicKey: "3b46ce7cb2f3",
			},
		},
		Meta: ContactListMeta{Count: 2, SyncedAt: time.Now().Add(-11 * time.Minute)},
	}
	out := RenderContacts(data, NewPrinter(bytes.NewBuffer(nil)))
	if !strings.Contains(out, "AKAT-Tester1 🗼") {
		t.Fatalf("missing emoji name:\n%s", out)
	}
	if !strings.Contains(out, "NAME") || !strings.Contains(out, "ROUTE") || !strings.Contains(out, "DIST") {
		t.Fatalf("missing headers:\n%s", out)
	}
	if !strings.Contains(out, "companion") || !strings.Contains(out, "flood") {
		t.Fatalf("missing type/route labels:\n%s", out)
	}
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("expected plain output without ANSI:\n%s", out)
	}
	if !strings.Contains(out, "2 contacts · last advert") {
		t.Fatalf("missing footer:\n%s", out)
	}
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) < 4 {
		t.Fatalf("expected header + rows + blank + footer, got:\n%s", out)
	}
}

func TestRenderContactsWide(t *testing.T) {
	data := ContactListData{
		Wide: true,
		Contacts: []ContactRow{
			{
				Name:      "Alešova",
				Type:      "repeater",
				Age:       "6m",
				Advert:    "1B",
				Route:     "1 hop",
				Path:      "a9",
				Distance:  "4.1 km",
				Location:  "50.479238, 13.990112",
				PublicKey: "a90d1408ff61103e0123456789abcdef",
			},
		},
		Meta: ContactListMeta{Count: 1},
	}
	out := RenderContacts(data, NewPrinter(bytes.NewBuffer(nil)))
	if !strings.Contains(out, "PATH") || !strings.Contains(out, "LOCATION") {
		t.Fatalf("missing wide headers:\n%s", out)
	}
	if !strings.Contains(out, "a90d1408ff61103e0123456789abcdef") {
		t.Fatalf("missing full key:\n%s", out)
	}
	if !strings.Contains(out, "50.479238, 13.990112") {
		t.Fatalf("missing location:\n%s", out)
	}
}

func TestRenderContactDetail(t *testing.T) {
	tests := []struct {
		name string
		data ContactDetailData
		want []string
	}{
		{
			name: "flood",
			data: ContactDetailData{
				Name:        "011111000101",
				Type:        "companion",
				PublicKey:   "3b46ce7cb2f3",
				LastSeen:    "18d",
				LastSeenAbs: "2026-05-20 12:14:09",
				Location:    "-",
				Distance:    "-",
				AdvertPath:  "flood",
				Path:        "-",
			},
			want: []string{"Type:          companion", "Advert path:   flood", "Distance:      -"},
		},
		{
			name: "direct",
			data: ContactDetailData{
				Name:        "3gy.pt repeater",
				Type:        "repeater",
				PublicKey:   "2690e0d0b5e9",
				LastSeen:    "42m",
				LastSeenAbs: "2026-06-07 13:58:11",
				Location:    "50.123456, 14.123456",
				Distance:    "8.4 km",
				AdvertPath:  "direct · 2-byte hashes",
				Path:        "-",
			},
			want: []string{"Advert path:   direct · 2-byte hashes", "Location:      50.123456, 14.123456"},
		},
		{
			name: "routed",
			data: ContactDetailData{
				Name:        "AKAT-Tester1 🗼",
				Type:        "repeater",
				PublicKey:   "57dbabc15c30",
				LastSeen:    "8m",
				LastSeenAbs: "2026-06-07 14:32:08",
				Location:    "49.195100, 16.606800",
				Distance:    "62 km",
				AdvertPath:  "3 hops · 2-byte hashes",
				Path:        "a90d → 57db → 3f18",
			},
			want: []string{"Name:          AKAT-Tester1 🗼", "Path:          a90d → 57db → 3f18"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := RenderContactDetail(tc.data, NewPrinter(bytes.NewBuffer(nil)))
			for _, want := range tc.want {
				if !strings.Contains(out, want) {
					t.Fatalf("missing %q in:\n%s", want, out)
				}
			}
			if strings.Contains(out, "Has path:") {
				t.Fatalf("unexpected Has path field:\n%s", out)
			}
		})
	}
}

func TestAdvertPathDetail(t *testing.T) {
	if got := AdvertPathDetail(contactWithMeta(pathhash.OutPathUnknown, nil)); got != "flood" {
		t.Fatalf("flood = %q", got)
	}
	if got := AdvertPathDetail(contactWithMeta(0x40, nil)); got != "direct · 2-byte hashes" {
		t.Fatalf("direct = %q", got)
	}
	if got := AdvertPathDetail(contactWithMeta(0x43, []byte{0xa9, 0x0d, 0x57, 0xdb, 0x3f, 0x18})); got != "3 hops · 2-byte hashes" {
		t.Fatalf("routed = %q", got)
	}
}
