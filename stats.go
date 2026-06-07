package meshcore

import (
	"context"

	"github.com/meshcore-cz/meshcore-go/protocol"
	"github.com/meshcore-cz/meshcore-go/protocol/companion"
)

// LocalStats is a snapshot of local device, radio and packet counters.
type LocalStats struct {
	Core    LocalStatsCore    `json:"core"`
	Radio   LocalStatsRadio   `json:"radio"`
	Packets LocalStatsPackets `json:"packets"`
}

// LocalStatsCore carries device-level counters.
type LocalStatsCore struct {
	BatteryMV  uint16 `json:"battery_mv"`
	UptimeSecs uint32 `json:"uptime_secs"`
	QueueLen   byte   `json:"queue_len"`
	ErrorFlags uint16 `json:"error_flags"`
}

// LocalStatsRadio carries radio configuration, signal and airtime counters.
type LocalStatsRadio struct {
	FrequencyKHz uint32  `json:"frequency_khz"`
	BandwidthKHz uint32  `json:"bandwidth_khz"`
	Spreading    byte    `json:"spreading"`
	CodingRate   byte    `json:"coding_rate"`
	TxPowerDBm   byte    `json:"tx_power_dbm"`
	NoiseFloor   int16   `json:"noise_floor_dbm"`
	LastRSSI     int     `json:"last_rssi_dbm"`
	LastSNR      float64 `json:"last_snr_db"`
	RxAirSecs    uint32  `json:"rx_air_secs"`
	TxAirSecs    uint32  `json:"tx_air_secs"`
}

// LocalStatsPackets carries packet counters split by total and route type.
type LocalStatsPackets struct {
	Received   uint32 `json:"received"`
	Sent       uint32 `json:"sent"`
	RecvErrors uint32 `json:"rx_errors"`
	FloodRx    uint32 `json:"flood_rx"`
	DirectRx   uint32 `json:"direct_rx"`
	FloodTx    uint32 `json:"flood_tx"`
	DirectTx   uint32 `json:"direct_tx"`
}

// Stats queries the local radio statistics exposed by MeshCore CMD_GET_STATS.
func (c *Client) Stats(ctx context.Context) (LocalStats, error) {
	info, _ := c.DeviceInfo(ctx)
	core, err := c.statsCore(ctx)
	if err != nil {
		return LocalStats{}, err
	}
	radio, err := c.statsRadio(ctx)
	if err != nil {
		return LocalStats{}, err
	}
	packets, err := c.statsPackets(ctx)
	if err != nil {
		return LocalStats{}, err
	}
	return LocalStats{
		Core: LocalStatsCore{
			BatteryMV:  core.BatteryMV,
			UptimeSecs: core.UptimeSecs,
			QueueLen:   core.QueueLen,
			ErrorFlags: core.ErrorFlags,
		},
		Radio: LocalStatsRadio{
			FrequencyKHz: info.RadioFreqKHz,
			BandwidthKHz: info.RadioBWKHz,
			Spreading:    info.RadioSF,
			CodingRate:   info.RadioCR,
			TxPowerDBm:   info.TxPowerDBm,
			NoiseFloor:   radio.NoiseFloor,
			LastRSSI:     radio.LastRSSI,
			LastSNR:      radio.LastSNR,
			RxAirSecs:    radio.RxAirSecs,
			TxAirSecs:    radio.TxAirSecs,
		},
		Packets: LocalStatsPackets{
			Received:   packets.Received,
			Sent:       packets.Sent,
			RecvErrors: packets.RecvErrors,
			FloodRx:    packets.FloodRx,
			DirectRx:   packets.DirectRx,
			FloodTx:    packets.FloodTx,
			DirectTx:   packets.DirectTx,
		},
	}, nil
}

func (c *Client) statsCore(ctx context.Context) (companion.StatsCore, error) {
	resp, err := c.requestStats(ctx, companion.StatsTypeCore)
	if err != nil {
		return companion.StatsCore{}, err
	}
	if resp.Core == nil {
		return companion.StatsCore{}, protocol.ErrUnexpectedResponse
	}
	return *resp.Core, nil
}

func (c *Client) statsRadio(ctx context.Context) (companion.StatsRadio, error) {
	resp, err := c.requestStats(ctx, companion.StatsTypeRadio)
	if err != nil {
		return companion.StatsRadio{}, err
	}
	if resp.Radio == nil {
		return companion.StatsRadio{}, protocol.ErrUnexpectedResponse
	}
	return *resp.Radio, nil
}

func (c *Client) statsPackets(ctx context.Context) (companion.StatsPackets, error) {
	resp, err := c.requestStats(ctx, companion.StatsTypePackets)
	if err != nil {
		return companion.StatsPackets{}, err
	}
	if resp.Packets == nil {
		return companion.StatsPackets{}, protocol.ErrUnexpectedResponse
	}
	return *resp.Packets, nil
}

func (c *Client) requestStats(ctx context.Context, typ byte) (companion.StatsResponse, error) {
	msg, err := c.request(ctx, companion.GetStats{Type: typ})
	if err != nil {
		return companion.StatsResponse{}, err
	}
	resp, ok := msg.(companion.StatsResponse)
	if !ok || resp.Type != typ {
		return companion.StatsResponse{}, protocol.ErrUnexpectedResponse
	}
	return resp, nil
}
