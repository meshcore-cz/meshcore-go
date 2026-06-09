package cli

import (
	"context"

	localbackend "github.com/meshcore-cz/meshcore-go/backend"
	"github.com/meshcore-cz/meshcore-go/cmd/mc/internal/config"
	"github.com/meshcore-cz/meshcore-go/cmd/mc/internal/ui"
)

func backendStatusDataFromDaemon(ctx context.Context, e *env, st localbackend.DaemonStatus, verbose bool) ui.BackendStatusData {
	sessions := backendSessionDetails(ctx, e, st, verbose)
	handled, failed, pending := sumBackendSessionRequests(sessions)
	cfgPath, _ := config.Path()
	return ui.BackendStatusData{
		Running:           true,
		Healthy:           true,
		State:             "running",
		PID:               st.PID,
		StartedAt:         st.StartedAt,
		UptimeSec:         st.UptimeSec,
		Socket:            st.Socket,
		Clients:           sumBackendSessionClients(sessions),
		RequestsCompleted: handled,
		RequestsFailed:    failed,
		QueuePending:      pending,
		Version:           st.Version,
		CLIVersion:        Version,
		LogPath:           localbackend.LogPath(),
		ConfigPath:        cfgPath,
		Verbose:           verbose,
		Sessions:          sessions,
	}
}

func backendSessionDetails(ctx context.Context, e *env, st localbackend.DaemonStatus, verbose bool) []ui.BackendSessionInfo {
	cfg, _ := config.Load()
	summaries, _ := localbackend.ListStateSummaries()
	rows := make([]ui.BackendSessionInfo, 0, len(st.Devices))
	for _, entry := range st.Devices {
		row := ui.BackendSessionInfo{
			Name:      entry.ID,
			Active:    entry.Default,
			State:     entry.Session,
			Healthy:   entry.Connected,
			Transport: backendSessionTransport(entry),
			LastError: entry.LastError,
		}
		if cfg != nil {
			if dev, ok := cfg.Devices[entry.ID]; ok {
				row.LocalState = localStateForProfile(dev, summaries)
				row.LocalStatePath = localStatePathForProfile(dev, summaries)
			}
		}
		if entry.Session != "stopped" {
			if sess, ok := statusForDevice(ctx, e, entry.ID); ok {
				row.State = sess.State
				row.Healthy = sess.Healthy
				row.StartedAt = sess.StartedAt
				row.LastActive = sess.LastSeen
				row.LastError = sess.LastError
				row.Activity = radioIOFromStatus(sess.Radio)
				row.Clients = sess.Clients
				row.RequestsCompleted = sess.RequestsCompleted
				row.RequestsFailed = sess.RequestsFailed
				row.QueuePending = sess.QueuePending
				if sess.URI != "" {
					row.Transport = sess.URI
				}
				if sess.Device.PublicKey != "" {
					row.LocalState = localStateForPublicKey(sess.Device.PublicKey)
					row.LocalStatePath = localStatePathForPublicKey(sess.Device.PublicKey)
				}
			}
		}
		rows = append(rows, row)
	}
	return rows
}

func backendSessionTransport(entry localbackend.DeviceListEntry) string {
	if entry.URI != "" {
		return entry.URI
	}
	return entry.Transport
}

func sumBackendSessionClients(sessions []ui.BackendSessionInfo) int {
	// The current daemon protocol exposes connected IPC clients on detailed
	// session status, not on the daemon status snapshot. Avoid double counting by
	// taking the largest observed value across session probes.
	maxClients := 0
	for _, session := range sessions {
		if session.Clients > maxClients {
			maxClients = session.Clients
		}
	}
	return maxClients
}

func sumBackendSessionRequests(sessions []ui.BackendSessionInfo) (handled, failed int64, pending int) {
	for _, session := range sessions {
		handled += session.RequestsCompleted
		failed += session.RequestsFailed
		pending += session.QueuePending
	}
	return handled, failed, pending
}

func backendSessionsStatusJSON(sessions []ui.BackendSessionInfo) []map[string]any {
	out := make([]map[string]any, len(sessions))
	for i, session := range sessions {
		item := map[string]any{
			"id":                 session.Name,
			"default":            session.Active,
			"state":              session.State,
			"healthy":            session.Healthy,
			"transport":          session.Transport,
			"local_state":        session.LocalState,
			"requests_completed": session.RequestsCompleted,
			"requests_failed":    session.RequestsFailed,
			"queue_pending":      session.QueuePending,
		}
		if !session.StartedAt.IsZero() {
			item["started_at"] = session.StartedAt
		}
		if !session.LastActive.IsZero() {
			item["last_active"] = session.LastActive
		}
		if session.LastError != "" {
			item["last_error"] = session.LastError
		}
		if session.LocalStatePath != "" {
			item["local_state_path"] = session.LocalStatePath
		}
		out[i] = item
	}
	return out
}
