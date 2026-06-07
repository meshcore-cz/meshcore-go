package cli

import (
	"sort"

	localbackend "github.com/meshcore-cz/meshcore-go/backend"
	"github.com/meshcore-cz/meshcore-go/cmd/mc/internal/config"
	"github.com/meshcore-cz/meshcore-go/cmd/mc/internal/ui"
)

func deviceListData(cfg *config.Config, st localbackend.Status, backendRunning bool) ui.DeviceListData {
	names := deviceListOrder(cfg)
	rows := make([]ui.DeviceListRow, 0, len(names))
	meta := ui.DeviceListMeta{Count: len(names), Selected: cfg.Current}

	for _, name := range names {
		d := cfg.Devices[name]
		active := backendRunning && profileMatchesBackend(d, st)
		row := ui.DeviceListRow{
			Profile:   name,
			Selected:  name == cfg.Current,
			Device:    deviceListDeviceID(d, st, active),
			Backend:   ui.DeviceListBackendState(active, backendRunning && st.Running, st.State),
			Radio:     ui.DeviceListRadioState(active, backendRunning && st.Running, st.Healthy, st.State),
			Replica:   ui.DeviceListReplicaState(active, backendRunning && st.Running, st.Healthy, st.State, replicaInfoFromStatus(st.Contacts), replicaInfoFromChannel(st.Channels)),
			Transport: ui.HumanTransport(d.PreferredTransport),
			Activity:  ui.DeviceListActivityState(active, backendRunning && st.Running, st.State, replicaInfoFromStatus(st.Contacts), replicaInfoFromChannel(st.Channels), radioIOFromStatus(st.Radio)),
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

func profileMatchesBackend(d config.Device, st localbackend.Status) bool {
	uri := d.PrimaryURI()
	if uri == "" || st.URI == "" {
		return false
	}
	return uri == st.URI
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

func deviceListJSON(cfg *config.Config, st localbackend.Status, backendRunning bool) []map[string]any {
	data := deviceListData(cfg, st, backendRunning)
	out := make([]map[string]any, len(data.Devices))
	for i, row := range data.Devices {
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
	}
	return out
}
