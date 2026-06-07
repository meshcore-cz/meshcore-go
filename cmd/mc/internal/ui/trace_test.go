package ui

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func tracePrinter(t *testing.T) (Printer, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	return NewPrinter(&buf), &buf
}

func TestTraceNodePlainLabel(t *testing.T) {
	tests := []struct {
		name string
		node TraceNode
		want string
	}{
		{"hash only", TraceNode{Hash: "2525"}, "[2525]"},
		{"hash and name", TraceNode{Hash: "2525", Name: "mc.kololec.cz"}, "[2525] mc.kololec.cz"},
		{"ambiguous", TraceNode{Hash: "25", Ambiguous: true, Names: []string{"a", "b"}}, "[25]"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.node.PlainLabel(); got != tc.want {
				t.Fatalf("PlainLabel() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTracePrefixLabel(t *testing.T) {
	tests := []struct {
		bytes int
		want  string
	}{
		{1, "1B"},
		{2, "2B"},
		{4, "4B"},
		{8, "8B"},
		{3, "3B"},
	}
	for _, tc := range tests {
		if got := tracePrefixLabel(tc.bytes); got != tc.want {
			t.Fatalf("tracePrefixLabel(%d) = %q, want %q", tc.bytes, got, tc.want)
		}
	}
}

func TestRenderTraceDirectGolden(t *testing.T) {
	printer, _ := tracePrinter(t)
	data := TraceData{
		Target:      TraceNode{Hash: "2525", Name: "mc.kololec.cz"},
		Request:     "explicit path · 2525",
		PrefixBytes: 2,
		Tag:         "b7e46d9a",
		RoundTrip:   687 * time.Millisecond,
		Legs: []TraceLeg{
			{Number: 1, From: TraceNode{Hash: "eff0", Name: "EFF01EF2"}, To: TraceNode{Hash: "2525", Name: "mc.kololec.cz"}, SNRDB: 12.5},
			{Number: 2, From: TraceNode{Hash: "2525", Name: "mc.kololec.cz"}, To: TraceNode{Hash: "eff0", Name: "EFF01EF2"}, SNRDB: 11.5},
		},
	}
	out := RenderTrace(data, printer)
	_ = strings.TrimSpace(`
Trace:         [2525] mc.kololec.cz
Request:       explicit path · 2525
Prefix:        2B
Round trip:    687ms

LEG  FROM                    TO                      SNR
───  ──────────────────────  ──────────────────────  ─────────
1    [eff0] EFF01EF2 → [2525] mc.kololec.cz  +12.5 dB
2    [2525] mc.kololec.cz → [eff0] EFF01EF2  +11.5 dB

2 legs · weakest +11.5 dB
`)
	// Normalize spacing around arrow for comparison.
	got := strings.TrimSpace(out)
	if !strings.Contains(got, "Trace:         [2525] mc.kololec.cz") {
		t.Fatalf("missing trace header:\n%s", out)
	}
	if !strings.Contains(got, "Request:       explicit path · 2525") {
		t.Fatalf("missing request:\n%s", out)
	}
	if !strings.Contains(got, "Prefix:        2B") {
		t.Fatalf("missing prefix:\n%s", out)
	}
	if !strings.Contains(got, "Tag:           b7e46d9a") {
		t.Fatalf("missing tag:\n%s", out)
	}
	if !strings.Contains(got, "Round trip:    687ms") {
		t.Fatalf("missing round trip:\n%s", out)
	}
	if !strings.Contains(got, "LEG") || !strings.Contains(got, "FROM") || !strings.Contains(got, "SNR") {
		t.Fatalf("missing table header:\n%s", out)
	}
	if !strings.Contains(got, "+12.5 dB") || !strings.Contains(got, "+11.5 dB") {
		t.Fatalf("missing SNR values:\n%s", out)
	}
	if !strings.Contains(got, "2 legs · weakest +11.5 dB") {
		t.Fatalf("missing footer:\n%s", out)
	}
	if strings.Contains(got, "← weak") {
		t.Fatalf("positive SNR should not be weak:\n%s", out)
	}
	if strings.Contains(got, "\x1b[") {
		t.Fatalf("buffer output must not contain ANSI escapes:\n%s", out)
	}
}

func TestRenderTraceWeakLink(t *testing.T) {
	printer, _ := tracePrinter(t)
	data := TraceData{
		Target:      TraceNode{Hash: "3f18", Name: "hilltop-room"},
		Request:     "explicit path · a90d → 57db → 3f18",
		PrefixBytes: 2,
		RoundTrip:   1420 * time.Millisecond,
		Legs: []TraceLeg{
			{Number: 3, From: TraceNode{Hash: "57db", Name: "AKAT-Tester1"}, To: TraceNode{Hash: "3f18", Name: "hilltop-room"}, SNRDB: -2.0},
		},
	}
	out := RenderTrace(data, printer)
	if !strings.Contains(out, "-2.0 dB  ← weak") {
		t.Fatalf("missing weak marker:\n%s", out)
	}
}

func TestTraceFooter(t *testing.T) {
	tests := []struct {
		name string
		legs []TraceLeg
		want string
	}{
		{
			name: "one leg",
			legs: []TraceLeg{{SNRDB: 4.0}},
			want: "1 leg · weakest +4.0 dB",
		},
		{
			name: "two legs",
			legs: []TraceLeg{{SNRDB: 12.5}, {SNRDB: 11.5}},
			want: "2 legs · weakest +11.5 dB",
		},
		{
			name: "six legs",
			legs: []TraceLeg{{SNRDB: 10}, {SNRDB: 6.5}, {SNRDB: -2.0}, {SNRDB: -1.5}, {SNRDB: 4}, {SNRDB: 8.5}},
			want: "6 legs · weakest -2.0 dB",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := traceFooter(tc.legs); got != tc.want {
				t.Fatalf("traceFooter() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRenderTraceAmbiguousWarning(t *testing.T) {
	printer, buf := tracePrinter(t)
	data := TraceData{
		Target:      TraceNode{Hash: "25", Ambiguous: true, Names: []string{"mc.kololec.cz", "mc.koren.cz"}},
		Request:     "explicit path · 25",
		PrefixBytes: 1,
		RoundTrip:   500 * time.Millisecond,
		Legs: []TraceLeg{
			{Number: 1, From: TraceNode{Hash: "eff0", Name: "EFF01EF2"}, To: TraceNode{Hash: "25", Ambiguous: true, Names: []string{"mc.kololec.cz", "mc.koren.cz"}}, SNRDB: 4.0},
		},
		Ambiguous: []TraceNode{
			{Hash: "25", Ambiguous: true, Names: []string{"mc.koren.cz", "mc.kololec.cz"}},
		},
	}
	out := RenderTrace(data, printer)
	if !strings.Contains(out, "Warning: prefix [25] matches multiple contacts:") {
		t.Fatalf("missing warning header:\n%s", out)
	}
	kololec := strings.Index(out, "mc.kololec.cz")
	koren := strings.Index(out, "mc.koren.cz")
	if kololec < 0 || koren < 0 || kololec > koren {
		t.Fatalf("names must be alphabetical:\n%s", out)
	}
	if strings.Contains(out, "[25] mc.") {
		t.Fatalf("ambiguous leg must not include arbitrary name:\n%s", out)
	}
	if strings.Contains(buf.String(), "\x1b[") {
		t.Fatalf("buffer output must not contain ANSI escapes")
	}
}

func TestRenderTraceNoSignal(t *testing.T) {
	printer, _ := tracePrinter(t)
	data := TraceData{
		Target:      TraceNode{Hash: "2525", Name: "mc.kololec.cz"},
		Request:     "explicit path · 2525",
		PrefixBytes: 2,
		RoundTrip:   687 * time.Millisecond,
	}
	out := RenderTrace(data, printer)
	if !strings.Contains(out, "Signal:        no data") {
		t.Fatalf("missing no-signal line:\n%s", out)
	}
	if strings.Contains(out, "LEG  FROM") {
		t.Fatalf("must not render table without SNR data:\n%s", out)
	}
	if strings.Contains(out, "weakest") {
		t.Fatalf("must not render footer without SNR data:\n%s", out)
	}
}

func TestRenderTraceUnicodeAlignment(t *testing.T) {
	printer, _ := tracePrinter(t)
	data := TraceData{
		Target:      TraceNode{Hash: "57db", Name: "AKAT-Tester1 🗼"},
		Request:     "explicit path · a90d → 57db",
		PrefixBytes: 2,
		RoundTrip:   time.Second,
		Legs: []TraceLeg{
			{Number: 1, From: TraceNode{Hash: "a90d", Name: "Alešova"}, To: TraceNode{Hash: "57db", Name: "AKAT-Tester1 🗼"}, SNRDB: 6.5},
		},
	}
	out := RenderTrace(data, printer)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	var legLine string
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "1 ") {
			legLine = line
			break
		}
	}
	if legLine == "" {
		t.Fatalf("missing leg row:\n%s", out)
	}
	arrow := strings.Index(legLine, "→")
	if arrow < 0 {
		t.Fatalf("missing arrow in %q", legLine)
	}
	fromPart := strings.TrimSpace(legLine[len("1    "):arrow])
	toPart := strings.TrimSpace(legLine[arrow+len("→") : strings.Index(legLine, "+")])
	if DisplayWidth(fromPart) > 22 {
		t.Fatalf("from column too wide: %q (%d)", fromPart, DisplayWidth(fromPart))
	}
	if DisplayWidth(toPart) > 22 {
		t.Fatalf("to column too wide: %q (%d)", toPart, DisplayWidth(toPart))
	}
	if !strings.Contains(legLine, "AKAT-Tester1 🗼") {
		t.Fatalf("missing emoji name:\n%s", legLine)
	}
}

func TestRenderTraceANSIStyled(t *testing.T) {
	// Force color by using a pseudo-terminal isn't practical; verify dim/warning helpers directly.
	theme := Theme{enabled: true}
	node := TraceNode{Hash: "2525", Name: "mc.kololec.cz"}
	styled := node.StyledLabel(theme)
	if !strings.Contains(styled, "\x1b[2m[2525]\x1b[0m") {
		t.Fatalf("hash not dim: %q", styled)
	}
	if !strings.HasSuffix(styled, " mc.kololec.cz") {
		t.Fatalf("name should stay unstyled: %q", styled)
	}
	marginal := traceSNRStyled(-2.0, traceSNRColMin, theme)
	if !strings.Contains(marginal, "\x1b[1;33m") || !strings.Contains(marginal, "← weak") {
		t.Fatalf("marginal SNR not yellow: %q", marginal)
	}
	bad := traceSNRStyled(-8.0, traceSNRColMin, theme)
	if !strings.Contains(bad, "\x1b[1;31m") || !strings.Contains(bad, "← weak") {
		t.Fatalf("bad SNR not red: %q", bad)
	}
	good := traceSNRStyled(12.5, traceSNRColMin, theme)
	if !strings.Contains(good, "\x1b[1;32m") {
		t.Fatalf("good SNR should be green: %q", good)
	}
}

func TestTraceSNRHealth(t *testing.T) {
	tests := []struct {
		snr    float64
		health Health
	}{
		{12.5, HealthOK},
		{5.0, HealthOK},
		{4.9, HealthWarning},
		{0, HealthWarning},
		{-2.0, HealthWarning},
		{-5.0, HealthWarning},
		{-5.1, HealthError},
		{-12.0, HealthError},
	}
	for _, tc := range tests {
		if got := traceSNRHealth(tc.snr); got != tc.health {
			t.Fatalf("traceSNRHealth(%+.1f) = %v, want %v", tc.snr, got, tc.health)
		}
	}
}
