package backend

import (
	"encoding/json"
	"time"
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

type statusResult struct {
	Running   bool           `json:"running"`
	Healthy   bool           `json:"healthy"`
	State     string         `json:"state"`
	URI       string         `json:"uri"`
	Transport string         `json:"transport"`
	PID       int            `json:"pid"`
	LastSeen  time.Time      `json:"last_seen,omitempty"`
	LastError string         `json:"last_error,omitempty"`
	Bridges   []BridgeStatus `json:"bridges,omitempty"`
	Contacts  contactStatus  `json:"contacts,omitempty"`
}

type contactStatus struct {
	Syncing  bool      `json:"syncing"`
	Count    int       `json:"count"`
	SyncedAt time.Time `json:"synced_at,omitempty"`
	Error    string    `json:"error,omitempty"`
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
}

type channelSendParams struct {
	Channel string `json:"channel"`
	Text    string `json:"text"`
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
