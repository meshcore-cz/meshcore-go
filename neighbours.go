package meshcore

import (
	"fmt"
	"strconv"
	"strings"
)

// RepeaterNeighbour is one entry from a repeater neighbors CLI response.
type RepeaterNeighbour struct {
	PublicKeyPrefix string  `json:"public_key_prefix"`
	HeardSecs       uint32  `json:"heard_secs"`
	SNRdB           float64 `json:"snr_db"`
}

// ParseRepeaterNeighbours parses the repeater firmware neighbors response:
// one line per neighbor as PREFIX:secs_ago:snr_x4.
func ParseRepeaterNeighbours(text string) []RepeaterNeighbour {
	text = strings.TrimSpace(text)
	if text == "" || text == "-none-" {
		return nil
	}
	var out []RepeaterNeighbour
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "-none-" {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) != 3 {
			continue
		}
		prefix := strings.ToLower(strings.TrimSpace(parts[0]))
		if len(prefix) != 8 {
			continue
		}
		heard, err := strconv.ParseUint(strings.TrimSpace(parts[1]), 10, 32)
		if err != nil {
			continue
		}
		snrRaw, err := strconv.ParseInt(strings.TrimSpace(parts[2]), 10, 16)
		if err != nil {
			continue
		}
		out = append(out, RepeaterNeighbour{
			PublicKeyPrefix: prefix,
			HeardSecs:       uint32(heard),
			SNRdB:           float64(snrRaw) / 4.0,
		})
	}
	return out
}

// FormatRepeaterNeighbours renders neighbors for terminal output.
func FormatRepeaterNeighbours(repeater string, neighbours []RepeaterNeighbour) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Repeater: %s\n", repeater)
	if len(neighbours) == 0 {
		b.WriteString("\nNo neighbors.\n")
		return b.String()
	}
	fmt.Fprintf(&b, "\n%d neighbor", len(neighbours))
	if len(neighbours) != 1 {
		b.WriteString("s")
	}
	b.WriteString(":\n\n")
	fmt.Fprintf(&b, "%-10s %-16s %s\n", "PREFIX", "LAST HEARD", "SNR")
	for _, n := range neighbours {
		fmt.Fprintf(&b, "%-10s %-16s %+.1f dB\n", n.PublicKeyPrefix, formatHeardAgo(n.HeardSecs), n.SNRdB)
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

func formatHeardAgo(secs uint32) string {
	if secs < 60 {
		return fmt.Sprintf("%ds ago", secs)
	}
	if secs < 3600 {
		return fmt.Sprintf("%dm ago", secs/60)
	}
	if secs < 86400 {
		h := secs / 3600
		m := (secs % 3600) / 60
		if m == 0 {
			return fmt.Sprintf("%dh ago", h)
		}
		return fmt.Sprintf("%dh %dm ago", h, m)
	}
	d := secs / 86400
	h := (secs % 86400) / 3600
	if h == 0 {
		return fmt.Sprintf("%dd ago", d)
	}
	return fmt.Sprintf("%dd %dh ago", d, h)
}
