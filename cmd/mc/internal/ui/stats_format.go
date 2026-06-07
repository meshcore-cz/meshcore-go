package ui

import (
	"fmt"
	"strconv"
	"time"

	meshcore "github.com/meshcore-cz/meshcore-go"
)

// DeviceStatsInfo is a compact stats snapshot for status output.
type DeviceStatsInfo struct {
	Available bool
	Core      meshcore.LocalStatsCore
	Radio     meshcore.LocalStatsRadio
	Packets   meshcore.LocalStatsPackets
	UpdatedAt time.Time
}

func DeviceStatsFromLocal(s meshcore.LocalStats, ok bool, updatedAt time.Time) DeviceStatsInfo {
	return DeviceStatsInfo{
		Available: ok,
		Core:      s.Core,
		Radio:     s.Radio,
		Packets:   s.Packets,
		UpdatedAt: updatedAt,
	}
}

func signalLabel(s DeviceStatsInfo) string {
	if !s.Available {
		return ""
	}
	parts := []string{}
	if s.Radio.LastRSSI != 0 {
		parts = append(parts, fmt.Sprintf("%d dBm RSSI", s.Radio.LastRSSI))
	}
	if s.Radio.LastSNR != 0 {
		parts = append(parts, fmt.Sprintf("%+.1f dB SNR", s.Radio.LastSNR))
	}
	if s.Radio.NoiseFloor != 0 {
		parts = append(parts, fmt.Sprintf("%d dBm noise", s.Radio.NoiseFloor))
	}
	return joinParts(parts)
}

func batteryLabel(s DeviceStatsInfo) string {
	if !s.Available || s.Core.BatteryMV == 0 {
		return ""
	}
	return fmt.Sprintf("%.2f V", float64(s.Core.BatteryMV)/1000)
}

func uptimeLabel(s DeviceStatsInfo) string {
	if !s.Available || s.Core.UptimeSecs == 0 {
		return ""
	}
	return FormatDurationSecs(s.Core.UptimeSecs)
}

func packetsLabel(s DeviceStatsInfo) string {
	if !s.Available {
		return ""
	}
	parts := []string{
		formatUint(s.Packets.Received) + " rx",
		formatUint(s.Packets.Sent) + " tx",
	}
	if s.Packets.RecvErrors > 0 {
		parts = append(parts, formatUint(s.Packets.RecvErrors)+" errors")
	}
	return joinParts(parts)
}

func airtimeLabel(s DeviceStatsInfo) string {
	if !s.Available {
		return ""
	}
	parts := []string{}
	if s.Radio.RxAirSecs > 0 {
		parts = append(parts, FormatDurationSecs(s.Radio.RxAirSecs)+" rx")
	}
	if s.Radio.TxAirSecs > 0 {
		parts = append(parts, FormatDurationSecs(s.Radio.TxAirSecs)+" tx")
	}
	return joinParts(parts)
}

func queueLabel(s DeviceStatsInfo) string {
	if !s.Available {
		return ""
	}
	if s.Core.QueueLen == 0 {
		return "0 pending"
	}
	return fmt.Sprintf("%d pending", s.Core.QueueLen)
}

func joinParts(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for _, p := range parts[1:] {
		out += " · " + p
	}
	return out
}

// FormatBandwidthHz formats a companion radio bandwidth value stored in Hz.
func FormatBandwidthHz(hz uint32) string {
	if hz == 0 {
		return ""
	}
	khz := float64(hz) / 1000
	if khz == float64(uint32(khz)) {
		return fmt.Sprintf("%.0f kHz", khz)
	}
	return fmt.Sprintf("%.1f kHz", khz)
}

// FormatDurationSecs formats seconds as a compact duration ("10h 57m", "8m 37s").
func FormatDurationSecs(secs uint32) string {
	d := time.Duration(secs) * time.Second
	days := d / (24 * time.Hour)
	d -= days * 24 * time.Hour
	hours := d / time.Hour
	d -= hours * time.Hour
	mins := d / time.Minute
	d -= mins * time.Minute
	seconds := d / time.Second

	if days > 0 {
		if hours > 0 {
			return fmt.Sprintf("%dd %dh", days, hours)
		}
		return fmt.Sprintf("%dd", days)
	}
	if hours > 0 {
		if mins > 0 {
			return fmt.Sprintf("%dh %dm", hours, mins)
		}
		return fmt.Sprintf("%dh", hours)
	}
	if mins > 0 {
		if seconds > 0 {
			return fmt.Sprintf("%dm %ds", mins, seconds)
		}
		return fmt.Sprintf("%dm", mins)
	}
	return fmt.Sprintf("%ds", seconds)
}

func formatUint(n uint32) string {
	s := strconv.FormatUint(uint64(n), 10)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
