package companion

import (
	"encoding/binary"
	"fmt"
	"strings"
)

// RepeaterStatsSize is sizeof(RepeaterStats) in MeshCore repeater firmware.
const RepeaterStatsSize = 56

// RepeaterStats is the binary payload returned by REQ_TYPE_GET_STATUS (0x01).
type RepeaterStats struct {
	BattMilliVolts     uint16
	CurrTxQueueLen     uint16
	NoiseFloor         int16
	LastRSSI           int16
	NPacketsRecv       uint32
	NPacketsSent       uint32
	TotalAirTimeSecs   uint32
	TotalUpTimeSecs    uint32
	NSentFlood         uint32
	NSentDirect        uint32
	NRecvFlood         uint32
	NRecvDirect        uint32
	ErrEvents          uint16
	LastSNR            int16 // scaled x4 in firmware
	NDirectDups        uint16
	NFloodDups         uint16
	TotalRxAirTimeSecs uint32
	NRecvErrors        uint32
}

// ParseRepeaterStats decodes a binary GET_STATUS payload.
func ParseRepeaterStats(b []byte) (RepeaterStats, bool) {
	if len(b) < RepeaterStatsSize {
		return RepeaterStats{}, false
	}
	return RepeaterStats{
		BattMilliVolts:     binary.LittleEndian.Uint16(b[0:2]),
		CurrTxQueueLen:     binary.LittleEndian.Uint16(b[2:4]),
		NoiseFloor:         int16(binary.LittleEndian.Uint16(b[4:6])),
		LastRSSI:           int16(binary.LittleEndian.Uint16(b[6:8])),
		NPacketsRecv:       binary.LittleEndian.Uint32(b[8:12]),
		NPacketsSent:       binary.LittleEndian.Uint32(b[12:16]),
		TotalAirTimeSecs:   binary.LittleEndian.Uint32(b[16:20]),
		TotalUpTimeSecs:    binary.LittleEndian.Uint32(b[20:24]),
		NSentFlood:         binary.LittleEndian.Uint32(b[24:28]),
		NSentDirect:        binary.LittleEndian.Uint32(b[28:32]),
		NRecvFlood:         binary.LittleEndian.Uint32(b[32:36]),
		NRecvDirect:        binary.LittleEndian.Uint32(b[36:40]),
		ErrEvents:          binary.LittleEndian.Uint16(b[40:42]),
		LastSNR:            int16(binary.LittleEndian.Uint16(b[42:44])),
		NDirectDups:        binary.LittleEndian.Uint16(b[44:46]),
		NFloodDups:         binary.LittleEndian.Uint16(b[46:48]),
		TotalRxAirTimeSecs: binary.LittleEndian.Uint32(b[48:52]),
		NRecvErrors:        binary.LittleEndian.Uint32(b[52:56]),
	}, true
}

// RepeaterStatsJSON is the structured stats object for machine-readable output.
type RepeaterStatsJSON struct {
	Core struct {
		BatteryMV  uint16 `json:"battery_mv"`
		UptimeSecs uint32 `json:"uptime_secs"`
		Errors     uint16 `json:"errors"`
		QueueLen   uint16 `json:"queue_len"`
	} `json:"core"`
	Radio struct {
		NoiseFloor int16   `json:"noise_floor"`
		LastRSSI   int16   `json:"last_rssi"`
		LastSNR    float64 `json:"last_snr"`
		TxAirSecs  uint32  `json:"tx_air_secs"`
		RxAirSecs  uint32  `json:"rx_air_secs"`
	} `json:"radio"`
	Packets struct {
		Recv       uint32 `json:"recv"`
		Sent       uint32 `json:"sent"`
		FloodTx    uint32 `json:"flood_tx"`
		DirectTx   uint32 `json:"direct_tx"`
		FloodRx    uint32 `json:"flood_rx"`
		DirectRx   uint32 `json:"direct_rx"`
		RecvErrors uint32 `json:"recv_errors"`
		DirectDups uint16 `json:"direct_dups"`
		FloodDups  uint16 `json:"flood_dups"`
	} `json:"packets"`
}

// JSONValue returns structured stats for --json output.
func (s RepeaterStats) JSONValue() RepeaterStatsJSON {
	var out RepeaterStatsJSON
	out.Core.BatteryMV = s.BattMilliVolts
	out.Core.UptimeSecs = s.TotalUpTimeSecs
	out.Core.Errors = s.ErrEvents
	out.Core.QueueLen = s.CurrTxQueueLen
	out.Radio.NoiseFloor = s.NoiseFloor
	out.Radio.LastRSSI = s.LastRSSI
	out.Radio.LastSNR = float64(s.LastSNR) / 4.0
	out.Radio.TxAirSecs = s.TotalAirTimeSecs
	out.Radio.RxAirSecs = s.TotalRxAirTimeSecs
	out.Packets.Recv = s.NPacketsRecv
	out.Packets.Sent = s.NPacketsSent
	out.Packets.FloodTx = s.NSentFlood
	out.Packets.DirectTx = s.NSentDirect
	out.Packets.FloodRx = s.NRecvFlood
	out.Packets.DirectRx = s.NRecvDirect
	out.Packets.RecvErrors = s.NRecvErrors
	out.Packets.DirectDups = s.NDirectDups
	out.Packets.FloodDups = s.NFloodDups
	return out
}

// Human formats stats for terminal output.
func (s RepeaterStats) Human() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Core\n")
	fmt.Fprintf(&b, "  Battery:      %.2f V\n", float64(s.BattMilliVolts)/1000)
	fmt.Fprintf(&b, "  Uptime:       %s\n", formatDuration(s.TotalUpTimeSecs))
	fmt.Fprintf(&b, "  TX queue:     %d\n", s.CurrTxQueueLen)
	fmt.Fprintf(&b, "  Errors:       %d\n", s.ErrEvents)
	fmt.Fprintf(&b, "\nRadio\n")
	fmt.Fprintf(&b, "  Noise floor:  %d dBm\n", s.NoiseFloor)
	fmt.Fprintf(&b, "  Last RSSI:    %d dBm\n", s.LastRSSI)
	fmt.Fprintf(&b, "  Last SNR:     %+.1f dB\n", float64(s.LastSNR)/4.0)
	fmt.Fprintf(&b, "  TX air time:  %s\n", formatDuration(s.TotalAirTimeSecs))
	fmt.Fprintf(&b, "  RX air time:  %s\n", formatDuration(s.TotalRxAirTimeSecs))
	fmt.Fprintf(&b, "\nPackets\n")
	fmt.Fprintf(&b, "  Received:     %d\n", s.NPacketsRecv)
	fmt.Fprintf(&b, "  Sent:         %d\n", s.NPacketsSent)
	fmt.Fprintf(&b, "  Flood TX:     %d\n", s.NSentFlood)
	fmt.Fprintf(&b, "  Direct TX:    %d\n", s.NSentDirect)
	fmt.Fprintf(&b, "  Flood RX:     %d\n", s.NRecvFlood)
	fmt.Fprintf(&b, "  Direct RX:    %d\n", s.NRecvDirect)
	fmt.Fprintf(&b, "  RX errors:    %d\n", s.NRecvErrors)
	fmt.Fprintf(&b, "  Direct dups:  %d\n", s.NDirectDups)
	fmt.Fprintf(&b, "  Flood dups:   %d\n", s.NFloodDups)
	return strings.TrimRight(b.String(), "\n")
}

func formatDuration(secs uint32) string {
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	d := secs / 86400
	secs %= 86400
	h := secs / 3600
	secs %= 3600
	m := secs / 60
	var parts []string
	if d > 0 {
		parts = append(parts, fmt.Sprintf("%dd", d))
	}
	if h > 0 {
		parts = append(parts, fmt.Sprintf("%dh", h))
	}
	if m > 0 {
		parts = append(parts, fmt.Sprintf("%dm", m))
	}
	if len(parts) == 0 {
		return fmt.Sprintf("%ds", secs)
	}
	return strings.Join(parts, " ")
}

// decodeStatusData parses PUSH status_data. Repeaters return binary stats;
// sensors may return plain text.
func decodeStatusData(b []byte) (*RepeaterStats, string) {
	if stats, ok := ParseRepeaterStats(b); ok {
		return &stats, ""
	}
	return nil, trimString(b)
}
