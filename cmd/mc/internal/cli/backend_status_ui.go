package cli

import (
	"time"

	localbackend "github.com/meshcore-cz/meshcore-go/backend"
	"github.com/meshcore-cz/meshcore-go/cmd/mc/internal/ui"
)

func backendStatusDataFromStatus(st localbackend.Status, verbose bool) ui.BackendStatusData {
	statsPollTimeout := 15 * time.Second
	if schemeOf(st.URI) == "ble" {
		statsPollTimeout = 30 * time.Second
	}
	return ui.BackendStatusData{
		Running:           st.Running,
		Healthy:           st.Healthy,
		State:             st.State,
		PID:               st.PID,
		StartedAt:         st.StartedAt,
		UptimeSec:         st.UptimeSec,
		Socket:            st.Socket,
		URI:               st.URI,
		Transport:         st.Transport,
		LastSeen:          st.LastSeen,
		LastError:         st.LastError,
		LastErrorAt:       st.LastErrorAt,
		Contacts:          replicaInfoFromStatus(st.Contacts),
		Channels:          replicaInfoFromChannel(st.Channels),
		Stats:             ui.DeviceStatsFromBackend(st.Stats, st.StatsOK, st.StatsAt),
		RadioIO:           radioIOFromStatus(st.Radio),
		Bridges:           bridgesFromStatus(st.Bridges),
		QueuePending:      st.QueuePending,
		Reconnects:        st.Reconnects,
		Clients:           st.Clients,
		RequestsCompleted: st.RequestsCompleted,
		RequestsFailed:    st.RequestsFailed,
		Version:           st.Version,
		CLIVersion:        Version,
		LogPath:           localbackend.LogPath(),
		StatsPollTimeout:  statsPollTimeout,
		Verbose:           verbose,
	}
}

func bridgesFromStatus(bridges []localbackend.BridgeStatus) []ui.BridgeInfo {
	out := make([]ui.BridgeInfo, 0, len(bridges))
	for _, bridge := range bridges {
		out = append(out, ui.BridgeInfo{
			Name:   bridge.Name,
			Type:   bridge.Type,
			Listen: bridge.Listen,
			Path:   bridge.Path,
			Active: bridge.Active,
			Error:  bridge.Error,
		})
	}
	return out
}
