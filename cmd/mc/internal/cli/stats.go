package cli

import (
	"context"
	"fmt"
	"strconv"
	"time"

	meshcore "github.com/meshcore-cz/meshcore-go"
	"github.com/meshcore-cz/meshcore-go/cmd/mc/internal/ui"
)

func cmdStats(ctx context.Context, e *env) error {
	backend, err := openBackend(ctx, e)
	if err != nil {
		return err
	}
	defer backend.Close()

	stats, err := backend.Stats(ctx)
	if err != nil {
		return err
	}
	if e.out.JSON {
		return e.out.JSONValue(stats)
	}
	printStats(e, stats)
	return nil
}

func printStats(e *env, s meshcore.LocalStats) {
	e.out.Human("Core\n")
	statsLine(e, "Battery", fmt.Sprintf("%.2f V", float64(s.Core.BatteryMV)/1000))
	statsLine(e, "Uptime", formatStatsDuration(s.Core.UptimeSecs))
	statsLine(e, "Queue", strconv.Itoa(int(s.Core.QueueLen)))
	statsLine(e, "Errors", formatErrorFlags(s.Core.ErrorFlags))

	e.out.Human("\nRadio\n")
	statsLine(e, "Frequency", formatFrequencyMHz(s.Radio.FrequencyKHz))
	statsLine(e, "Bandwidth", orDashBandwidth(s.Radio.BandwidthKHz))
	statsLine(e, "Spreading", formatSpreading(s.Radio.Spreading))
	statsLine(e, "Coding rate", formatCodingRate(s.Radio.CodingRate))
	statsLine(e, "TX power", formatTxPower(s.Radio.TxPowerDBm))
	statsLine(e, "Noise floor", fmt.Sprintf("%d dBm", s.Radio.NoiseFloor))
	statsLine(e, "Last RSSI", fmt.Sprintf("%d dBm", s.Radio.LastRSSI))
	statsLine(e, "Last SNR", fmt.Sprintf("%+.1f dB", s.Radio.LastSNR))
	statsLine(e, "Airtime RX", formatStatsDuration(s.Radio.RxAirSecs))
	statsLine(e, "Airtime TX", formatStatsDuration(s.Radio.TxAirSecs))

	e.out.Human("\nPackets\n")
	statsLine(e, "Received", formatUint(s.Packets.Received))
	statsLine(e, "Sent", formatUint(s.Packets.Sent))
	statsLine(e, "RX errors", formatUint(s.Packets.RecvErrors))
	statsLine(e, "Flood RX", formatUint(s.Packets.FloodRx))
	statsLine(e, "Direct RX", formatUint(s.Packets.DirectRx))
	statsLine(e, "Flood TX", formatUint(s.Packets.FloodTx))
	statsLine(e, "Direct TX", formatUint(s.Packets.DirectTx))
}

func statsLine(e *env, label, value string) {
	e.out.Human("  %-14s %s\n", label+":", value)
}

func formatErrorFlags(flags uint16) string {
	if flags == 0 {
		return "none"
	}
	return fmt.Sprintf("0x%04x", flags)
}

func orDashBandwidth(hz uint32) string {
	if v := ui.FormatBandwidthHz(hz); v != "" {
		return v
	}
	return "-"
}

func formatFrequencyMHz(khz uint32) string {
	if khz == 0 {
		return "-"
	}
	return fmt.Sprintf("%.3f MHz", float64(khz)/1000)
}

func formatSpreading(sf byte) string {
	if sf == 0 {
		return "-"
	}
	return fmt.Sprintf("SF%d", sf)
}

func formatCodingRate(cr byte) string {
	if cr == 0 {
		return "-"
	}
	return fmt.Sprintf("4/%d", cr)
}

func formatTxPower(dbm byte) string {
	return fmt.Sprintf("%d dBm", dbm)
}

func formatUint(n uint32) string {
	s := strconv.FormatUint(uint64(n), 10)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func formatStatsDuration(secs uint32) string {
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
