package cli

import (
	"time"

	meshcore "github.com/meshcore-cz/meshcore-go"
	localbackend "github.com/meshcore-cz/meshcore-go/backend"
	"github.com/meshcore-cz/meshcore-go/cmd/mc/internal/ui"
)

func printStyledStatus(e *env, st localbackend.Status, dev localbackend.DeviceStatus, stats meshcore.LocalStats, statsOK bool, statsAt time.Time) error {
	data := statusDataFromBackend(st, dev, stats, statsOK, statsAt)
	printer := ui.NewPrinter(e.out.Out)
	printer.Print(ui.RenderStatus(data, printer))
	return nil
}

func deviceInfoFromBackend(st localbackend.Status, dev localbackend.DeviceStatus, stats meshcore.LocalStats, statsOK bool, statsAt time.Time) ui.DeviceInfo {
	if statsAt.IsZero() {
		statsAt = st.StatsAt
	}
	transport := dev.Transport
	if transport == "" {
		transport = st.Transport
	}
	return ui.DeviceInfo{
		Name:            dev.Name,
		PublicKey:       dev.PublicKey,
		Firmware:        dev.Firmware,
		FirmwareVersion: dev.FirmwareVersion,
		Protocol:        dev.Protocol,
		Transport:       transport,
		TransportURI:    st.URI,
		Capabilities:    append([]string(nil), dev.Capabilities...),
		Radio: ui.RadioInfo{
			FrequencyKHz: dev.RadioFreqKHz,
			BandwidthKHz: dev.RadioBWKHz,
			Spreading:    dev.RadioSF,
			CodingRate:   dev.RadioCR,
			TxPowerDBm:   dev.TxPowerDBm,
		},
		Available: dev.Available(),
		Stats:     ui.DeviceStatsFromLocal(stats, statsOK, statsAt),
	}
}

func statusDataFromBackend(st localbackend.Status, dev localbackend.DeviceStatus, stats meshcore.LocalStats, statsOK bool, statsAt time.Time) ui.StatusData {
	return ui.StatusData{
		Device: deviceInfoFromBackend(st, dev, stats, statsOK, statsAt),
		Backend: ui.BackendInfo{
			Running:   st.Running,
			Healthy:   st.Healthy,
			State:     st.State,
			PID:       st.PID,
			URI:       st.URI,
			LastError: st.LastError,
			LastSeen:  st.LastSeen,
			Contacts:  replicaInfoFromStatus(st.Contacts),
			Channels:  replicaInfoFromChannel(st.Channels),
			RadioIO:   radioIOFromStatus(st.Radio),
		},
	}
}

func radioIOFromStatus(r localbackend.RadioStatus) ui.RadioIOInfo {
	return ui.RadioIOInfo{
		Active:         r.Active,
		Method:         r.Method,
		DurationMs:     r.DurationMs,
		LastAt:         r.LastAt,
		LastMethod:     r.LastMethod,
		LastDurationMs: r.LastDurationMs,
	}
}

func replicaInfoFromStatus(cs localbackend.ContactStatus) ui.ReplicaInfo {
	return ui.ReplicaInfo{
		Syncing:      cs.Syncing,
		SyncReceived: cs.SyncReceived,
		SyncTotal:    cs.SyncTotal,
		Count:        cs.Count,
		SyncedAt:     cs.SyncedAt,
		Error:        cs.Error,
	}
}

func replicaInfoFromChannel(cs localbackend.ChannelStatus) ui.ReplicaInfo {
	return ui.ReplicaInfo{
		Syncing:  cs.Syncing,
		Count:    cs.Count,
		SyncedAt: cs.SyncedAt,
		Error:    cs.Error,
	}
}

func statusDataUnavailableBackend(st localbackend.Status) ui.StatusData {
	return ui.StatusData{
		Device: ui.DeviceInfo{
			Transport:    st.Transport,
			TransportURI: st.URI,
			Available:    false,
		},
		Backend: ui.BackendInfo{
			Running:   st.Running,
			Healthy:   st.Healthy,
			State:     st.State,
			PID:       st.PID,
			URI:       st.URI,
			LastError: st.LastError,
			LastSeen:  st.LastSeen,
			Contacts:  replicaInfoFromStatus(st.Contacts),
			Channels:  replicaInfoFromChannel(st.Channels),
			RadioIO:   radioIOFromStatus(st.Radio),
		},
	}
}

func printStyledUnavailableStatus(e *env, st localbackend.Status) error {
	data := statusDataUnavailableBackend(st)
	printer := ui.NewPrinter(e.out.Out)
	printer.Print(ui.RenderStatus(data, printer))
	return nil
}
