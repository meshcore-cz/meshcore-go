package backend

import (
	"encoding/json"
	"time"

	meshcore "github.com/meshcore-cz/meshcore-go"
)

type request struct {
	ID     uint64          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type response struct {
	ID     uint64          `json:"id"`
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

type deviceStatusSnapshot struct {
	Name            string   `json:"name,omitempty"`
	PublicKey       string   `json:"public_key,omitempty"`
	Firmware        string   `json:"firmware,omitempty"`
	FirmwareVersion string   `json:"firmware_version,omitempty"`
	Protocol        string   `json:"protocol,omitempty"`
	Capabilities    []string `json:"capabilities,omitempty"`
	RadioFreqKHz    uint32   `json:"radio_freq_khz,omitempty"`
	RadioBWKHz      uint32   `json:"radio_bw_khz,omitempty"`
	RadioSF         byte     `json:"radio_sf,omitempty"`
	RadioCR         byte     `json:"radio_cr,omitempty"`
	TxPowerDBm      byte     `json:"tx_power_dbm,omitempty"`
}

type statusResult struct {
	Running   bool                  `json:"running"`
	Healthy   bool                  `json:"healthy"`
	State     string                `json:"state"`
	URI       string                `json:"uri"`
	Transport string                `json:"transport"`
	PID       int                   `json:"pid"`
	LastSeen  time.Time             `json:"last_seen,omitempty"`
	LastError string                `json:"last_error,omitempty"`
	Bridges   []BridgeStatus        `json:"bridges,omitempty"`
	Contacts  contactStatus         `json:"contacts,omitempty"`
	Channels  channelStatus         `json:"channels,omitempty"`
	Device    *deviceStatusSnapshot `json:"device,omitempty"`
	Stats     *meshcore.LocalStats  `json:"stats,omitempty"`
	StatsAt   time.Time             `json:"stats_at,omitempty"`
	Radio     radioStatus           `json:"radio,omitempty"`
}

type contactStatus struct {
	Syncing      bool      `json:"syncing"`
	SyncReceived int       `json:"sync_received,omitempty"`
	SyncTotal    int       `json:"sync_total,omitempty"`
	Count        int       `json:"count"`
	SyncedAt     time.Time `json:"synced_at,omitempty"`
	Error        string    `json:"error,omitempty"`
}

type channelStatus struct {
	Syncing  bool      `json:"syncing"`
	Count    int       `json:"count"`
	SyncedAt time.Time `json:"synced_at,omitempty"`
	Error    string    `json:"error,omitempty"`
}

type radioStatus struct {
	Active     bool      `json:"active"`
	Idle       bool      `json:"idle"`
	Method     string    `json:"method,omitempty"`
	Since      time.Time `json:"since,omitempty"`
	DurationMs     int64     `json:"duration_ms,omitempty"`
	LastAt         time.Time `json:"last_at,omitempty"`
	LastMethod     string    `json:"last_method,omitempty"`
	LastDurationMs int64     `json:"last_duration_ms,omitempty"`
}

type sendTextParams struct {
	Recipient string `json:"recipient"`
	Text      string `json:"text"`
}

type waitAckParams struct {
	Receipt json.RawMessage `json:"receipt"`
}

type queryParams struct {
	Query string `json:"query"`
}

type contactsParams struct {
	Cached  bool `json:"cached"`
	Refresh bool `json:"refresh"`
	Wait    bool `json:"wait"`
	Full    bool `json:"full"`
}

type ContactRefreshResult struct {
	Started bool `json:"started"`
	Running bool `json:"running"`
}

type channelsParams struct {
	Refresh bool `json:"refresh"`
}

type statsParams struct {
	Refresh bool `json:"refresh"`
}

type channelSendParams struct {
	Channel string `json:"channel"`
	Text    string `json:"text"`
}

type advertParams struct {
	Flood bool `json:"flood"`
}

type discoverParams struct {
	Filter     byte `json:"filter,omitempty"`
	PrefixOnly bool `json:"prefix_only,omitempty"`
	TimeoutMs  int  `json:"timeout_ms,omitempty"`
}

type repeaterLoginParams struct {
	Repeater string `json:"repeater"`
	Password string `json:"password"`
}

type repeaterHasConnectionResult struct {
	Active bool `json:"active"`
}

type repeaterExecParams struct {
	Repeater string `json:"repeater"`
	Command  string `json:"command"`
}

type rawParams struct {
	Payload []byte `json:"payload"`
}

// RawResult is a JSON-friendly representation of a raw companion response.
type RawResult struct {
	Type    string `json:"type"`
	Code    byte   `json:"code,omitempty"`
	Push    bool   `json:"push,omitempty"`
	Payload []byte `json:"payload,omitempty"`
	Decoded string `json:"decoded,omitempty"`
}

// Event is a JSON-friendly event emitted by the local backend stream.
type Event struct {
	Type      string    `json:"type"`
	From      string    `json:"from,omitempty"`
	Channel   string    `json:"channel,omitempty"`
	Text      string    `json:"text,omitempty"`
	Timestamp time.Time `json:"timestamp,omitempty"`
	Code      string    `json:"code,omitempty"`
	RTTMillis int64     `json:"rtt_ms,omitempty"`
	Name      string    `json:"name,omitempty"`
	PublicKey string    `json:"public_key,omitempty"`
	Error     string    `json:"error,omitempty"`
}
