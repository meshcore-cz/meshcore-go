package ui

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

const traceLabelWidth = 14

const (
	traceLegColWidth  = 4
	traceNodeColWidth = 22
	traceSNRColMin    = 9

	traceSNRGoodDB = 5.0  // >= strong link
	traceSNRWeakDB = -5.0 // < very weak link
)

// TraceNode is one endpoint on a traced route.
type TraceNode struct {
	Hash      string
	Name      string
	Ambiguous bool
	Names     []string
}

// PlainLabel returns the unstyled hash-first node label for width calculations.
func (n TraceNode) PlainLabel() string {
	if n.Ambiguous || n.Name == "" {
		return "[" + n.Hash + "]"
	}
	return "[" + n.Hash + "] " + n.Name
}

// StyledLabel renders a hash-first node label with restrained terminal styling.
func (n TraceNode) StyledLabel(theme Theme) string {
	hash := theme.Dim("[" + n.Hash + "]")
	if n.Ambiguous || n.Name == "" {
		return hash
	}
	return hash + " " + n.Name
}

// TraceLeg is one measured directional link on a trace round trip.
type TraceLeg struct {
	Number int
	From   TraceNode
	To     TraceNode
	SNRDB  float64
}

// TraceData is the input model for trace rendering.
type TraceData struct {
	Target      TraceNode
	Request     string
	PrefixBytes int
	Tag         string
	RoundTrip   time.Duration
	Legs        []TraceLeg
	Ambiguous   []TraceNode
}

// RenderTrace renders mc trace for human output.
func RenderTrace(data TraceData, printer Printer) string {
	theme := NewTheme(printer.Out)
	var b strings.Builder

	b.WriteString(traceLine("Trace", data.Target.StyledLabel(theme)))
	if data.Request != "" {
		b.WriteString(traceLine("Request", data.Request))
	}
	b.WriteString(traceLine("Prefix", tracePrefixLabel(data.PrefixBytes)))
	if data.Tag != "" {
		b.WriteString(traceLine("Tag", theme.Dim(data.Tag)))
	}
	b.WriteString(traceLine("Round trip", formatTraceRoundTrip(data.RoundTrip)))

	if len(data.Legs) == 0 {
		b.WriteString(traceLine("Signal", "no data"))
	} else {
		b.WriteString("\n")
		writeTraceTable(&b, data.Legs, theme)
		b.WriteString("\n")
		b.WriteString(theme.Dim(traceFooter(data.Legs)))
		b.WriteString("\n")
	}

	for _, node := range sortedAmbiguousNodes(data.Ambiguous) {
		b.WriteString("\n")
		writeTraceAmbiguousWarning(&b, node, theme)
	}

	return b.String()
}

func traceLine(label, value string) string {
	return fmt.Sprintf("%-*s %s\n", traceLabelWidth, label+":", value)
}

func tracePrefixLabel(bytes int) string {
	switch bytes {
	case 1:
		return "1B"
	case 2:
		return "2B"
	case 4:
		return "4B"
	case 8:
		return "8B"
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

func formatTraceRoundTrip(d time.Duration) string {
	if d < time.Second {
		ms := d.Round(time.Millisecond).Milliseconds()
		if ms < 1 {
			ms = 1
		}
		return fmt.Sprintf("%dms", ms)
	}
	if d < time.Minute {
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	if s == 0 {
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%dm%ds", m, s)
}

func writeTraceTable(b *strings.Builder, legs []TraceLeg, theme Theme) {
	fromWidth, toWidth, snrWidth := traceTableWidths(legs)

	b.WriteString(traceTableHeader(fromWidth, toWidth, snrWidth))
	b.WriteString(traceTableSeparator(fromWidth, toWidth, snrWidth, theme))
	for _, leg := range legs {
		writeTraceLegRow(b, leg, fromWidth, toWidth, snrWidth, theme)
	}
}

func traceTableWidths(legs []TraceLeg) (fromWidth, toWidth, snrWidth int) {
	fromWidth = traceNodeColWidth
	toWidth = traceNodeColWidth
	snrWidth = traceSNRColMin
	for _, leg := range legs {
		if w := DisplayWidth(leg.From.PlainLabel()); w > fromWidth {
			fromWidth = w
		}
		if w := DisplayWidth(leg.To.PlainLabel()); w > toWidth {
			toWidth = w
		}
		if w := DisplayWidth(traceSNRPlain(leg.SNRDB)); w > snrWidth {
			snrWidth = w
		}
	}
	return fromWidth, toWidth, snrWidth
}

func traceTableHeader(fromWidth, toWidth, snrWidth int) string {
	return fmt.Sprintf("%-*s  %s  %s  %s\n",
		traceLegColWidth, "LEG",
		padDisplayWidth("FROM", fromWidth),
		padDisplayWidth("TO", toWidth),
		padDisplayWidth("SNR", snrWidth))
}

func traceTableSeparator(fromWidth, toWidth, snrWidth int, theme Theme) string {
	legSep := strings.Repeat("─", 3)
	fromSep := strings.Repeat("─", fromWidth)
	toSep := strings.Repeat("─", toWidth)
	snrSep := strings.Repeat("─", snrWidth)
	return theme.Dim(fmt.Sprintf("%s  %s  %s  %s\n",
		padDisplayWidth(legSep, traceLegColWidth),
		fromSep,
		toSep,
		snrSep))
}

func writeTraceLegRow(b *strings.Builder, leg TraceLeg, fromWidth, toWidth, snrWidth int, theme Theme) {
	arrow := theme.Dim("→")
	from := styledPaddedNode(leg.From, fromWidth, theme)
	to := styledPaddedNode(leg.To, toWidth, theme)
	snr := traceSNRStyled(leg.SNRDB, snrWidth, theme)

	b.WriteString(fmt.Sprintf("%-*d  %s %s %s  %s\n",
		traceLegColWidth, leg.Number,
		from, arrow, to, snr))
}

func styledPaddedNode(node TraceNode, width int, theme Theme) string {
	plain := node.PlainLabel()
	padded := padDisplayWidth(plain, width)
	styled := node.StyledLabel(theme)
	if plain == padded {
		return styled
	}
	pad := width - DisplayWidth(plain)
	if pad <= 0 {
		return styled
	}
	return styled + strings.Repeat(" ", pad)
}

func traceSNRPlain(snr float64) string {
	text := fmt.Sprintf("%+.1f dB", snr)
	if snr < 0 {
		return text + "  ← weak"
	}
	return text
}

func traceSNRHealth(snr float64) Health {
	switch {
	case snr >= traceSNRGoodDB:
		return HealthOK
	case snr < traceSNRWeakDB:
		return HealthError
	default:
		return HealthWarning
	}
}

func traceSNRStyled(snr float64, width int, theme Theme) string {
	plain := traceSNRPlain(snr)
	styled := theme.StatusWord(traceSNRHealth(snr), plain)
	padded := padDisplayWidth(plain, width)
	if plain == padded {
		return styled
	}
	return styled + strings.Repeat(" ", width-DisplayWidth(plain))
}

func traceFooter(legs []TraceLeg) string {
	weakest := legs[0].SNRDB
	for _, leg := range legs[1:] {
		if leg.SNRDB < weakest {
			weakest = leg.SNRDB
		}
	}
	count := len(legs)
	word := "legs"
	if count == 1 {
		word = "leg"
	}
	return fmt.Sprintf("%d %s · weakest %+.1f dB", count, word, weakest)
}

func sortedAmbiguousNodes(nodes []TraceNode) []TraceNode {
	seen := map[string]bool{}
	out := make([]TraceNode, 0, len(nodes))
	for _, node := range nodes {
		if !node.Ambiguous || seen[node.Hash] {
			continue
		}
		seen[node.Hash] = true
		dup := node
		dup.Names = append([]string(nil), node.Names...)
		sort.Strings(dup.Names)
		out = append(out, dup)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Hash < out[j].Hash })
	return out
}

func writeTraceAmbiguousWarning(b *strings.Builder, node TraceNode, theme Theme) {
	warning := theme.StatusWord(HealthWarning, "Warning:")
	hash := theme.Dim("[" + node.Hash + "]")
	b.WriteString(fmt.Sprintf("%s prefix %s matches multiple contacts:\n", warning, hash))
	for _, name := range node.Names {
		b.WriteString("  " + name + "\n")
	}
}
