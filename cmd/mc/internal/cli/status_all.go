package cli

import (
	"context"
	"strings"
	"time"

	localbackend "github.com/meshcore-cz/meshcore-go/backend"
	"github.com/meshcore-cz/meshcore-go/cmd/mc/internal/config"
	"github.com/meshcore-cz/meshcore-go/cmd/mc/internal/ui"
)

func cmdStatusAll(ctx context.Context, e *env) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if len(cfg.Devices) == 0 {
		if e.out.JSON {
			return e.out.JSONValue([]map[string]any{})
		}
		e.out.Human("No saved profiles. Run `mc connect`.\n")
		return nil
	}

	_, daemonUp := daemonRunning(ctx, e)
	entries := deviceSessionEntries(ctx, e, daemonUp)
	summaries, _ := localbackend.ListStateSummaries()
	data := statusAllData(ctx, e, cfg, entries, summaries, daemonUp)
	if e.out.JSON {
		return e.out.JSONValue(statusAllJSON(data))
	}
	printer := ui.NewPrinter(e.out.Out)
	printer.Print(ui.RenderStatusAll(data, printer))
	return nil
}

func statusAllData(ctx context.Context, e *env, cfg *config.Config, entries map[string]localbackend.DeviceListEntry, summaries []localbackend.StateSummary, daemonUp bool) ui.StatusAllData {
	names := deviceListOrder(cfg)
	rows := make([]ui.StatusAllRow, 0, len(names))
	for _, name := range names {
		dev := cfg.Devices[name]
		entry, hasEntry := entries[name]
		session := "stopped"
		transport := schemeOf(dev.PrimaryURI())
		if dev.PreferredTransport != "" {
			transport = dev.PreferredTransport
		}
		if hasEntry {
			session = entry.Session
			if entry.Transport != "" {
				transport = entry.Transport
			}
		}

		row := ui.StatusAllRow{
			Profile:    name,
			Selected:   name == cfg.Current,
			Session:    session,
			Transport:  transport,
			LocalState: localStateForProfile(dev, summaries),
		}
		if daemonUp && hasEntry && entry.Session != "stopped" {
			st, ok := statusForDevice(ctx, e, name)
			if ok {
				row.Session = st.State
				row.ConnectedAt = st.StartedAt
				if st.Transport != "" {
					row.Transport = st.Transport
				}
				row.Radio = ui.DeviceStatsFromLocal(st.Stats, st.StatsOK, st.StatsAt)
				if st.Device.PublicKey != "" {
					row.LocalState = localStateForPublicKey(st.Device.PublicKey)
				}
			}
		}
		rows = append(rows, row)
	}
	return ui.StatusAllData{Rows: rows}
}

func statusForDevice(ctx context.Context, e *env, name string) (localbackend.Status, bool) {
	st, err := localbackend.NewClientForDevice(resolveBackendSocket(e), name).Status(ctx)
	if err != nil {
		return localbackend.Status{}, false
	}
	return st, true
}

func localStateForProfile(dev config.Device, summaries []localbackend.StateSummary) ui.LocalStateInfo {
	if sum, ok := stateSummaryForProfile(dev, summaries); ok {
		return localStateFromSummary(sum)
	}
	return ui.LocalStateInfo{}
}

func localStatePathForProfile(dev config.Device, summaries []localbackend.StateSummary) string {
	if sum, ok := stateSummaryForProfile(dev, summaries); ok {
		return sum.Path
	}
	return ""
}

func stateSummaryForProfile(dev config.Device, summaries []localbackend.StateSummary) (localbackend.StateSummary, bool) {
	for _, key := range []string{dev.PublicKey, dev.PublicKeyPrefix} {
		key = config.NormalizePublicKey(key)
		if key == "" {
			continue
		}
		for _, sum := range summaries {
			if strings.HasPrefix(config.NormalizePublicKey(sum.PublicKey), key) || strings.HasPrefix(sum.Prefix, key) {
				return sum, true
			}
		}
	}
	return localbackend.StateSummary{}, false
}

func statusAllJSON(data ui.StatusAllData) []map[string]any {
	out := make([]map[string]any, len(data.Rows))
	for i, row := range data.Rows {
		item := map[string]any{
			"profile":     row.Profile,
			"selected":    row.Selected,
			"session":     row.Session,
			"transport":   row.Transport,
			"local_state": row.LocalState,
		}
		if row.Radio.Available {
			item["radio"] = row.Radio
		}
		if !row.ConnectedAt.IsZero() {
			item["connected_at"] = row.ConnectedAt.Format(time.RFC3339Nano)
		}
		if row.Observer != "" {
			item["observer"] = row.Observer
		}
		out[i] = item
	}
	return out
}
