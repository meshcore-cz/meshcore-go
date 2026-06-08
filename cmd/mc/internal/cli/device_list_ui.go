package cli

import (
	"context"
	"sort"

	localbackend "github.com/meshcore-cz/meshcore-go/backend"
	"github.com/meshcore-cz/meshcore-go/cmd/mc/internal/config"
	"github.com/meshcore-cz/meshcore-go/cmd/mc/internal/ui"
)

// deviceSessionEntries fetches the daemon's authoritative per-device session
// state, keyed by device id. Returns nil when the daemon is not running.
func deviceSessionEntries(ctx context.Context, e *env, backendRunning bool) map[string]localbackend.DeviceListEntry {
	if !backendRunning {
		return nil
	}
	list, err := localbackend.NewClient(resolveBackendSocket(e)).DeviceList(ctx)
	if err != nil {
		return nil
	}
	entries := make(map[string]localbackend.DeviceListEntry, len(list))
	for _, en := range list {
		entries[en.ID] = en
	}
	return entries
}

func deviceListData(cfg *config.Config, st localbackend.Status, entries map[string]localbackend.DeviceListEntry, backendRunning bool) ui.DeviceListData {
	names := deviceListOrder(cfg)
	rows := make([]ui.DeviceListRow, 0, len(names))
	meta := ui.DeviceListMeta{Count: len(names), Selected: cfg.Current}

	// haveEntries reports whether the daemon answered device_list. Older daemons
	// (or any device_list error) leave entries nil; fall back to matching the
	// single status snapshot against each profile by endpoint URI.
	haveEntries := entries != nil

	for _, name := range names {
		d := cfg.Devices[name]
		entry, hasEntry := entries[name]

		var active, isDefault, healthy bool
		var state string
		var contacts, channels ui.ReplicaInfo
		var radio ui.RadioIOInfo

		if haveEntries {
			active = hasEntry && entry.Session != "stopped"
			isDefault = entry.Default
			state = entry.Session
			healthy = entry.Connected
			// Use the detailed default-session status for richer replica/activity
			// columns; other sessions report a lightweight fresh/stale hint.
			if active && isDefault && st.Running {
				contacts = replicaInfoFromStatus(st.Contacts)
				channels = replicaInfoFromChannel(st.Channels)
				radio = radioIOFromStatus(st.Radio)
			} else if active {
				contacts = replicaInfoFromStatus(entry.Contacts)
				channels = replicaInfoFromChannel(entry.Channels)
			}
		} else {
			// Legacy single-session daemon: only the connected device is known.
			active = backendRunning && st.Running && profileMatchesBackend(d, st)
			isDefault = active
			state = st.State
			healthy = st.Healthy
			if active {
				contacts = replicaInfoFromStatus(st.Contacts)
				channels = replicaInfoFromChannel(st.Channels)
				radio = radioIOFromStatus(st.Radio)
			}
		}

		row := ui.DeviceListRow{
			Profile:   name,
			Selected:  name == cfg.Current,
			Device:    deviceListDeviceID(d, st, active && isDefault),
			Backend:   ui.DeviceListBackendState(active, backendRunning, state),
			Radio:     ui.DeviceListRadioState(active, backendRunning, healthy, state),
			Replica:   ui.DeviceListReplicaState(active, backendRunning, healthy, state, contacts, channels),
			Transport: ui.HumanTransport(deviceListTransport(d, entry, hasEntry)),
			Activity:  ui.DeviceListActivityState(active, backendRunning, state, contacts, channels, radio),
			Endpoint:  d.PrimaryURI(),
		}
		rows = append(rows, row)
		switch row.Backend {
		case "ready":
			meta.Ready++
		case "degraded":
			meta.Degraded++
		case "stopped":
			meta.Stopped++
		}
	}

	return ui.DeviceListData{Devices: rows, Meta: meta}
}

// profileMatchesBackend reports whether a saved profile's endpoint matches the
// single connected device reported by a legacy daemon's status.
func profileMatchesBackend(d config.Device, st localbackend.Status) bool {
	uri := d.PrimaryURI()
	if uri == "" || st.URI == "" {
		return false
	}
	return uri == st.URI
}

func deviceListTransport(d config.Device, entry localbackend.DeviceListEntry, hasEntry bool) string {
	if hasEntry && entry.Transport != "" {
		return entry.Transport
	}
	return d.PreferredTransport
}

func deviceListOrder(cfg *config.Config) []string {
	names := make([]string, 0, len(cfg.Devices))
	for name := range cfg.Devices {
		names = append(names, name)
	}
	sort.Strings(names)
	if cfg.Current == "" {
		return names
	}
	out := make([]string, 0, len(names))
	out = append(out, cfg.Current)
	for _, name := range names {
		if name != cfg.Current {
			out = append(out, name)
		}
	}
	return out
}

func deviceListDeviceID(d config.Device, st localbackend.Status, active bool) string {
	if active && st.Device.Available() {
		if id := ui.DeviceShortID(st.Device.PublicKey); id != "UNKNOWN" {
			return id
		}
	}
	if id := ui.DeviceShortID(d.PublicKeyPrefix); id != "UNKNOWN" {
		return id
	}
	if name := d.Name; name != "" {
		return name
	}
	return "-"
}

func deviceListJSON(cfg *config.Config, st localbackend.Status, entries map[string]localbackend.DeviceListEntry, backendRunning bool) []map[string]any {
	data := deviceListData(cfg, st, entries, backendRunning)
	out := make([]map[string]any, len(data.Devices))
	for i, row := range data.Devices {
		entry, hasEntry := entries[row.Profile]
		out[i] = map[string]any{
			"profile":   row.Profile,
			"selected":  row.Selected,
			"device":    row.Device,
			"backend":   row.Backend,
			"radio":     row.Radio,
			"replica":   row.Replica,
			"transport": row.Transport,
			"activity":  row.Activity,
			"endpoint":  row.Endpoint,
		}
		if hasEntry {
			out[i]["local_state"] = map[string]any{
				"contacts": contactStatusJSON(entry.Contacts),
				"channels": channelStatusJSON(entry.Channels),
			}
		}
	}
	return out
}
