package cli

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	meshcore "github.com/meshcore-cz/meshcore-go"
	localbackend "github.com/meshcore-cz/meshcore-go/backend"
	"github.com/meshcore-cz/meshcore-go/cmd/mc/internal/ui"
)

func clockDelta(deviceTime time.Time) string {
	d := time.Since(deviceTime)
	if d < 0 {
		d = -d
	}
	return d.Round(time.Second).String()
}

func cmdVersion(e *env) error {
	info := map[string]string{
		"mc":       Version,
		"meshcore": Version,
		"commit":   Commit,
		"go":       runtime.Version(),
		"os":       runtime.GOOS,
		"arch":     runtime.GOARCH,
	}
	if err := e.out.JSONValue(info); err != nil {
		return err
	}
	e.out.Human("mc        %s\n", Version)
	e.out.Human("meshcore   %s\n", Version)
	e.out.Human("commit     %s\n", Commit)
	e.out.Human("go         %s\n", runtime.Version())
	e.out.Human("os         %s\n", runtime.GOOS)
	e.out.Human("arch       %s\n", runtime.GOARCH)
	return nil
}

func cmdStatus(ctx context.Context, e *env) error {
	if e.args.has("all") {
		return cmdStatusAll(ctx, e)
	}
	st, backendRunning := backendStatus(ctx, e)
	if backendRunning && !e.args.has("direct") {
		if !st.Healthy {
			if e.out.JSON {
				return e.out.JSONValue(map[string]any{
					"device":      map[string]any{"available": false},
					"local_state": localStateForOutput(localStateForPublicKey(st.Device.PublicKey)),
					"backend":     backendStatusForOutput(st, true),
				})
			}
			return printStyledUnavailableStatus(e, st)
		}
		// Use only the cached status snapshot — no live radio I/O.
		dev := st.Device
		dev.Transport = st.Transport
		stats, statsOK, statsAt, err := statusStats(ctx, e, st)
		if err != nil {
			return err
		}
		return printMCStatus(e, st, dev, stats, statsOK, statsAt)
	}

	backend, err := openBackend(ctx, e)
	if err != nil {
		return err
	}
	defer backend.Close()

	info, err := backend.DeviceInfo(ctx)
	if err != nil {
		return err
	}
	stats, statsErr := backend.Stats(ctx)
	statsAt := time.Time{}
	if statsErr == nil {
		statsAt = time.Now()
	}
	return printMCStatus(e, st, deviceStatusFromInfo(info, backend.Transport(), backendRunning), stats, statsErr == nil, statsAt)
}

func statusStats(ctx context.Context, e *env, st localbackend.Status) (meshcore.LocalStats, bool, time.Time, error) {
	if !e.args.has("live") {
		return st.Stats, st.StatsOK, st.StatsAt, nil
	}
	stats, err := backendClientForEnv(e).StatsWithOptions(ctx, true)
	if err != nil {
		return meshcore.LocalStats{}, false, time.Time{}, fmt.Errorf("live stats: %w", err)
	}
	return stats, true, time.Now(), nil
}

func printMCStatus(e *env, st localbackend.Status, dev localbackend.DeviceStatus, stats meshcore.LocalStats, statsOK bool, statsAt time.Time) error {
	type statusJSON struct {
		Device     any `json:"device"`
		Radio      any `json:"radio,omitempty"`
		LocalState any `json:"local_state"`
		Backend    any `json:"backend"`
	}
	transport := dev.Transport
	if transport == "" {
		transport = st.URI
	}
	backendRunning := st.Running && st.Healthy
	if e.out.JSON {
		out := statusJSON{
			Device:     deviceStatusForOutput(dev, transport),
			Radio:      radioStatusForOutput(dev, stats, statsOK, statsAt),
			LocalState: localStateForOutput(localStateForPublicKey(dev.PublicKey)),
			Backend:    backendStatusForOutput(st, backendRunning),
		}
		return e.out.JSONValue(out)
	}

	return printStyledStatus(e, st, dev, stats, statsOK, statsAt)
}

func deviceStatusFromInfo(info meshcore.DeviceInfo, transport string, backendRunning bool) localbackend.DeviceStatus {
	return localbackend.DeviceStatus{
		Name:            info.Name,
		PublicKey:       info.PublicKey,
		Firmware:        info.FirmwareName,
		FirmwareVersion: info.FirmwareVersion,
		Protocol:        info.ProtocolVersion,
		Capabilities:    info.Capabilities.List(),
		Transport:       transport,
		RadioFreqKHz:    info.RadioFreqKHz,
		RadioBWKHz:      info.RadioBWKHz,
		RadioSF:         info.RadioSF,
		RadioCR:         info.RadioCR,
		TxPowerDBm:      info.TxPowerDBm,
		Latitude:        info.Latitude,
		Longitude:       info.Longitude,
	}
}

func deviceStatusForOutput(dev localbackend.DeviceStatus, transport string) map[string]any {
	return map[string]any{
		"public_key":       dev.PublicKey,
		"name":             dev.Name,
		"firmware":         dev.Firmware,
		"firmware_version": dev.FirmwareVersion,
		"protocol":         dev.Protocol,
		"transport":        transport,
		"capabilities":     dev.Capabilities,
	}
}

func radioStatusForOutput(dev localbackend.DeviceStatus, stats meshcore.LocalStats, statsOK bool, statsAt time.Time) any {
	hasModem := dev.RadioFreqKHz != 0 || dev.RadioBWKHz != 0 || dev.RadioSF != 0 || dev.RadioCR != 0 || dev.TxPowerDBm != 0
	if !hasModem && !statsOK {
		return nil
	}
	out := map[string]any{}
	if hasModem {
		out["modem"] = map[string]any{
			"frequency_khz": dev.RadioFreqKHz,
			"bandwidth_hz":  dev.RadioBWKHz,
			"spreading":     dev.RadioSF,
			"coding_rate":   dev.RadioCR,
			"tx_power_dbm":  dev.TxPowerDBm,
		}
	}
	if !statsOK {
		return out
	}
	if !statsAt.IsZero() {
		out["updated_at"] = statsAt
	}
	out["core"] = map[string]any{
		"battery_mv":  stats.Core.BatteryMV,
		"uptime_secs": stats.Core.UptimeSecs,
		"queue_len":   stats.Core.QueueLen,
		"error_flags": stats.Core.ErrorFlags,
	}
	out["signal"] = map[string]any{
		"rssi_dbm":        stats.Radio.LastRSSI,
		"snr_db":          stats.Radio.LastSNR,
		"noise_floor_dbm": stats.Radio.NoiseFloor,
	}
	out["packets"] = map[string]any{
		"received":  stats.Packets.Received,
		"sent":      stats.Packets.Sent,
		"rx_errors": stats.Packets.RecvErrors,
		"flood_rx":  stats.Packets.FloodRx,
		"direct_rx": stats.Packets.DirectRx,
		"flood_tx":  stats.Packets.FloodTx,
		"direct_tx": stats.Packets.DirectTx,
	}
	out["airtime"] = map[string]any{
		"rx_secs": stats.Radio.RxAirSecs,
		"tx_secs": stats.Radio.TxAirSecs,
	}
	return out
}

func cmdDoctor(ctx context.Context, e *env) error {
	type check struct {
		Name   string `json:"name"`
		Result string `json:"result"`
		OK     bool   `json:"ok"`
	}
	var checks []check
	add := func(name, result string, ok bool) {
		checks = append(checks, check{Name: name, Result: result, OK: ok})
		e.out.Human("%-34s %s\n", name, result)
	}

	uri, profile, err := resolveURI(e)
	if err != nil {
		add("Configuration", err.Error(), false)
		return finishDoctor(e, checks)
	}
	add("Configuration file", "ok", true)
	if profile != "" {
		add("Default profile", profile, true)
	}
	add("Endpoint", uri, true)

	if !e.args.has("direct") {
		if st, ok := backendStatus(ctx, e); ok {
			add("Local backend", fmt.Sprintf("running (pid %d)", st.PID), true)
			if st.Device.Available() {
				add("Companion radio", "reachable via backend", true)
				add("Protocol handshake", "already established", true)
				add("Firmware", strings.TrimSpace(st.Device.Firmware+" "+st.Device.FirmwareVersion), true)
				add("Protocol", orDash(st.Device.Protocol), true)
				return finishDoctor(e, checks)
			}
			client := backendClientForEnv(e)
			info, err := client.DeviceInfo(ctx)
			if err != nil {
				add("Companion radio", "backend error: "+err.Error(), false)
				return finishDoctor(e, checks)
			}
			add("Companion radio", "reachable via backend", true)
			add("Protocol handshake", "already established", true)
			add("Firmware", strings.TrimSpace(info.FirmwareName+" "+info.FirmwareVersion), true)
			add("Protocol", orDash(info.ProtocolVersion), true)
			return finishDoctor(e, checks)
		}
		add("Local backend", "not running", true)
	}

	client, err := meshcore.Dial(ctx, uri, e.dbg.DialOptions()...)
	if err != nil {
		add("Companion radio", "unreachable: "+err.Error(), false)
		return finishDoctor(e, checks)
	}
	defer client.Close()
	add("Companion radio", "reachable", true)
	add("Protocol handshake", "ok", true)

	info, _ := client.DeviceInfo(ctx)
	add("Firmware", strings.TrimSpace(info.FirmwareName+" "+info.FirmwareVersion), true)
	add("Protocol", orDash(info.ProtocolVersion), true)

	if t, err := client.DeviceTime(ctx); err == nil {
		add("Clock difference", clockDelta(t), true)
	}

	return finishDoctor(e, checks)
}

func finishDoctor(e *env, checks any) error {
	return e.out.JSONValue(checks)
}

func backendStatusForOutput(st localbackend.Status, running bool) map[string]any {
	if !running {
		return map[string]any{"running": false}
	}
	session := map[string]any{"state": st.State}
	if !st.StartedAt.IsZero() {
		session["connected_at"] = st.StartedAt
	}
	activity := map[string]any{
		"active": st.Radio.Active,
	}
	if st.Radio.Method != "" {
		activity["method"] = st.Radio.Method
	}
	if st.Radio.DurationMs > 0 {
		activity["duration_ms"] = st.Radio.DurationMs
	}
	if !st.Radio.LastAt.IsZero() {
		last := map[string]any{"at": st.Radio.LastAt}
		if st.Radio.LastMethod != "" {
			last["method"] = st.Radio.LastMethod
		}
		if st.Radio.LastDurationMs > 0 {
			last["duration_ms"] = st.Radio.LastDurationMs
		}
		activity["last_request"] = last
	}
	out := map[string]any{
		"running":    st.Running,
		"healthy":    st.Healthy,
		"state":      st.State,
		"pid":        st.PID,
		"uptime_sec": st.UptimeSec,
		"socket":     st.Socket,
		"session":    session,
		"activity":   activity,
		"requests": map[string]any{
			"completed": st.RequestsCompleted,
			"failed":    st.RequestsFailed,
			"pending":   st.QueuePending,
		},
		"clients": st.Clients,
		"version": st.Version,
	}
	if !st.StartedAt.IsZero() {
		out["started_at"] = st.StartedAt
	}
	if st.LastError != "" {
		out["last_error"] = st.LastError
		out["last_error_at"] = st.LastErrorAt
	}
	return out
}

func localStateForOutput(local ui.LocalStateInfo) map[string]any {
	out := map[string]any{
		"initialized": local.Initialized,
	}
	if !local.Initialized {
		return out
	}
	out["contacts"] = local.Contacts
	out["channels"] = local.Channels
	out["repeater_sessions"] = local.RepeaterSessions
	if !local.UpdatedAt.IsZero() {
		out["updated_at"] = local.UpdatedAt
	}
	return out
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func shortKey(k string) string {
	if len(k) > 12 {
		return k[:12] + "..."
	}
	return orDash(k)
}
