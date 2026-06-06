package meshcore

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// RepeaterNeighbour is one entry from a repeater neighbors CLI response.
type RepeaterNeighbour struct {
	PublicKeyPrefix string  `json:"public_key_prefix"`
	Name            string  `json:"name,omitempty"` // resolved from local contacts when known
	Latitude        float64 `json:"latitude,omitempty"`
	Longitude       float64 `json:"longitude,omitempty"`
	DistanceKm      float64 `json:"distance_km,omitempty"` // from repeater when both positions are known
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

// EnrichRepeaterNeighbours fills neighbour names and coordinates from a contact
// list and computes distance from the repeater when both positions are known.
func EnrichRepeaterNeighbours(neighbours []RepeaterNeighbour, repeater Contact, contacts []Contact) []RepeaterNeighbour {
	if len(neighbours) == 0 {
		return neighbours
	}
	out := append([]RepeaterNeighbour(nil), neighbours...)
	repeaterHasCoords := contactHasCoords(repeater)
	for i := range out {
		ct, ok := contactForKeyPrefix(contacts, out[i].PublicKeyPrefix)
		if !ok {
			continue
		}
		out[i].Name = ct.Name
		if contactHasCoords(ct) {
			out[i].Latitude = ct.Latitude
			out[i].Longitude = ct.Longitude
			if repeaterHasCoords {
				out[i].DistanceKm = haversineDistanceKm(repeater.Latitude, repeater.Longitude, ct.Latitude, ct.Longitude)
			}
		}
	}
	return out
}

func contactForKeyPrefix(contacts []Contact, prefix string) (Contact, bool) {
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	if prefix == "" {
		return Contact{}, false
	}
	var matches []Contact
	for _, ct := range contacts {
		key := strings.ToLower(ct.PublicKey)
		if strings.HasPrefix(key, prefix) {
			matches = append(matches, ct)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], true
	case 0:
		return Contact{}, false
	default:
		return matches[0], true
	}
}

func contactHasCoords(ct Contact) bool {
	return ct.Latitude != 0 || ct.Longitude != 0
}

func haversineDistanceKm(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusKm = 6371.0
	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180
	lat1r := lat1 * math.Pi / 180
	lat2r := lat2 * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1r)*math.Cos(lat2r)*math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadiusKm * c
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
	fmt.Fprintf(&b, "%-10s %-24s %-20s %-8s %-16s %s\n", "PREFIX", "NAME", "COORDS", "DIST", "LAST HEARD", "SNR")
	for _, n := range neighbours {
		fmt.Fprintf(&b, "%-10s %-24s %-20s %-8s %-16s %+.1f dB\n",
			n.PublicKeyPrefix,
			neighbourDisplayName(n.Name),
			formatNeighbourCoords(n.Latitude, n.Longitude),
			formatNeighbourDistance(n.DistanceKm),
			formatHeardAgo(n.HeardSecs),
			n.SNRdB,
		)
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

func neighbourDisplayName(name string) string {
	if strings.TrimSpace(name) == "" {
		return "-"
	}
	return name
}

func formatNeighbourCoords(lat, lon float64) string {
	if !contactHasCoords(Contact{Latitude: lat, Longitude: lon}) {
		return "-"
	}
	return fmt.Sprintf("%.5f,%.5f", lat, lon)
}

func formatNeighbourDistance(km float64) string {
	if km <= 0 {
		return "-"
	}
	if km < 1 {
		return fmt.Sprintf("%dm", int(km*1000+0.5))
	}
	if km < 100 {
		return fmt.Sprintf("%.1fkm", km)
	}
	return fmt.Sprintf("%.0fkm", km)
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
