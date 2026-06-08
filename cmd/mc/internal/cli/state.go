package cli

import (
	"fmt"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	localbackend "github.com/meshcore-cz/meshcore-go/backend"
	"github.com/meshcore-cz/meshcore-go/cmd/mc/internal/config"
	"github.com/meshcore-cz/meshcore-go/cmd/mc/internal/ui"
)

// cmdState inspects and manages per-device local-state databases. The databases
// are device-local state (not a cache): they may be stale, incomplete, or
// locally enriched.
func cmdState(e *env) error {
	switch e.restArg(0) {
	case "", "list":
		return stateList(e)
	case "show":
		return stateShow(e)
	case "purge":
		return statePurge(e)
	case "prune":
		return statePrune(e)
	default:
		return fmt.Errorf("unknown state subcommand %q", e.restArg(0))
	}
}

func stateList(e *env) error {
	summaries, err := localbackend.ListStateSummaries()
	if err != nil {
		return err
	}
	if e.out.JSON {
		return e.out.JSONValue(summaries)
	}
	if len(summaries) == 0 {
		e.out.Human("No device state. Run `mc connect`.\n")
		return nil
	}
	tw := tabwriter.NewWriter(e.out.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "PREFIX\tDEVICE\tCONTACTS\tCHANNELS\tSESSIONS\tSIZE\tUPDATED")
	for _, s := range summaries {
		fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%d\t%s\t%s\n",
			s.Prefix,
			shortKey(s.PublicKey),
			s.Contacts,
			s.Channels,
			s.RepeaterSessions,
			humanBytes(s.SizeBytes),
			ui.RelativeTime(s.ModTime),
		)
	}
	return tw.Flush()
}

func stateShow(e *env) error {
	sum, err := resolveStateDevice(e.restArg(1))
	if err != nil {
		return err
	}
	if e.out.JSON {
		return e.out.JSONValue(sum)
	}
	e.out.Human("Public key:   %s\n", orDash(sum.PublicKey))
	e.out.Human("Prefix:       %s\n", sum.Prefix)
	e.out.Human("Path:         %s\n", sum.Path)
	if !sum.CreatedAt.IsZero() {
		e.out.Human("Created:      %s\n", sum.CreatedAt.Local().Format(time.RFC3339))
	}
	if sum.SchemaVersion != "" {
		e.out.Human("Schema:       %s\n", sum.SchemaVersion)
	}
	e.out.Human("Contacts:     %d\n", sum.Contacts)
	e.out.Human("Channels:     %d\n", sum.Channels)
	e.out.Human("Sessions:     %d\n", sum.RepeaterSessions)
	e.out.Human("Size:         %s\n", humanBytes(sum.SizeBytes))
	if !sum.ModTime.IsZero() {
		e.out.Human("Updated:      %s\n", ui.RelativeTime(sum.ModTime))
	}
	return nil
}

func statePurge(e *env) error {
	sum, err := resolveStateDevice(e.restArg(1))
	if err != nil {
		return err
	}
	removed, err := localbackend.PurgeState(sum.Path)
	if err != nil {
		return err
	}
	if e.out.JSON {
		return e.out.JSONValue(map[string]any{"public_key": sum.PublicKey, "prefix": sum.Prefix, "removed": removed})
	}
	if len(removed) == 0 {
		e.out.Human("Nothing to purge for %s.\n", sum.Prefix)
		return nil
	}
	e.out.Human("Purged local state for %s (%s).\n", sum.Prefix, shortKey(sum.PublicKey))
	return nil
}

func statePrune(e *env) error {
	raw := e.args.flag("older-than")
	if raw == "" {
		return fmt.Errorf("usage: mc state prune --older-than <duration> (e.g. 30d, 12h)")
	}
	maxAge, err := parseHumanDuration(raw)
	if err != nil {
		return err
	}
	cutoff := time.Now().Add(-maxAge)

	summaries, err := localbackend.ListStateSummaries()
	if err != nil {
		return err
	}
	var pruned []map[string]any
	for _, s := range summaries {
		if s.ModTime.IsZero() || !s.ModTime.Before(cutoff) {
			continue
		}
		removed, err := localbackend.PurgeState(s.Path)
		if err != nil {
			return err
		}
		pruned = append(pruned, map[string]any{"public_key": s.PublicKey, "prefix": s.Prefix, "removed": removed})
		e.out.Human("Pruned %s (%s), last updated %s.\n", s.Prefix, shortKey(s.PublicKey), ui.RelativeTime(s.ModTime))
	}
	if len(pruned) == 0 {
		e.out.Human("No device state older than %s.\n", raw)
	}
	return e.out.JSONValue(map[string]any{"older_than": raw, "pruned": pruned})
}

// resolveStateDevice resolves a device argument (saved profile name, full public
// key, or filename/key prefix) to its local-state database summary.
func resolveStateDevice(arg string) (localbackend.StateSummary, error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return localbackend.StateSummary{}, fmt.Errorf("usage: mc state <show|purge> <device>")
	}
	summaries, err := localbackend.ListStateSummaries()
	if err != nil {
		return localbackend.StateSummary{}, err
	}

	target := strings.ToLower(arg)
	if cfg, err := config.Load(); err == nil {
		if dev, ok := cfg.Devices[arg]; ok && dev.PublicKey != "" {
			target = strings.ToLower(strings.TrimSpace(dev.PublicKey))
		}
	}

	for _, s := range summaries {
		if strings.EqualFold(s.PublicKey, target) {
			return s, nil
		}
	}
	var matches []localbackend.StateSummary
	for _, s := range summaries {
		if strings.HasPrefix(strings.ToLower(s.PublicKey), target) || strings.HasPrefix(s.Prefix, target) {
			matches = append(matches, s)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return localbackend.StateSummary{}, fmt.Errorf("no device state matching %q", arg)
	default:
		return localbackend.StateSummary{}, fmt.Errorf("device %q is ambiguous (%d matches)", arg, len(matches))
	}
}

// parseHumanDuration parses a Go duration, additionally accepting day (d) and
// week (w) suffixes, e.g. "30d", "2w", "12h".
func parseHumanDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}
	unit := s[len(s)-1]
	if unit == 'd' || unit == 'w' {
		n, err := strconv.ParseFloat(s[:len(s)-1], 64)
		if err != nil {
			return 0, fmt.Errorf("invalid duration %q", s)
		}
		day := 24 * float64(time.Hour)
		if unit == 'w' {
			return time.Duration(n * 7 * day), nil
		}
		return time.Duration(n * day), nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q (use 30d, 2w, 12h, ...)", s)
	}
	return d, nil
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for n/div >= unit && exp < 4 {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGT"[exp])
}
