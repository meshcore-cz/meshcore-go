package ui

import (
	"fmt"
	"time"
)

// ContactAge formats advert age using a single largest unit ("17h", "706d").
func ContactAge(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t)
	if d < 0 {
		d = -d
	}
	switch {
	case d < time.Second:
		ms := d.Round(time.Millisecond).Milliseconds()
		if ms < 1 {
			ms = 1
		}
		return fmt.Sprintf("%dms", ms)
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// RelativeTime formats t as a compact relative time ("3s ago", "2h ago").
func RelativeTime(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t)
	if d < 0 {
		d = -d
	}
	switch {
	case d < time.Second:
		ms := d.Round(time.Millisecond).Milliseconds()
		if ms < 1 {
			ms = 1
		}
		return fmt.Sprintf("%dms ago", ms)
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		h := int(d.Hours())
		m := int(d.Minutes()) % 60
		if m == 0 {
			return fmt.Sprintf("%dh ago", h)
		}
		return fmt.Sprintf("%dh %dm ago", h, m)
	default:
		days := int(d.Hours()) / 24
		h := int(d.Hours()) % 24
		if h == 0 {
			return fmt.Sprintf("%dd ago", days)
		}
		return fmt.Sprintf("%dd %dh ago", days, h)
	}
}

// StatsFreshnessMaxAge is how long cached radio stats stay fresh before showing stale.
const StatsFreshnessMaxAge = 90 * time.Second

// StatsUpdateStale reports whether cached radio stats are older than StatsFreshnessMaxAge.
func StatsUpdateStale(updatedAt time.Time) bool {
	return !updatedAt.IsZero() && time.Since(updatedAt) > StatsFreshnessMaxAge
}

// StatsUpdatedRelative formats when cached radio stats were last refreshed.
// Values under 5ms are shown as "now".
func StatsUpdatedRelative(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	if time.Since(t) < 5*time.Millisecond {
		return "now"
	}
	return RelativeTime(t)
}
