package backend

import (
	"context"
	"time"

	meshcore "github.com/meshcore-cz/meshcore-go"
)

// Message direction and delivery-status constants used in the messages table.
const (
	MessageIn  = "in"
	MessageOut = "out"

	MessageDirect  = "direct"
	MessageChannel = "channel"

	// Outgoing delivery statuses.
	StatusQueued      = "queued"
	StatusSent        = "sent"
	StatusDelivered   = "delivered"
	StatusUnconfirmed = "unconfirmed"
	StatusFailed      = "failed"

	// Incoming messages carry this status.
	StatusReceived = "received"
)

// MessageRecord is one stored direct or channel message. The database is scoped
// to a single local device (one file per device public key), so messages are
// namespaced by device without a per-row device column.
type MessageRecord struct {
	ID             int64     `json:"id"`
	Direction      string    `json:"direction"`      // MessageIn | MessageOut
	Kind           string    `json:"kind"`           // MessageDirect | MessageChannel
	Peer           string    `json:"peer,omitempty"` // contact public key/prefix (direct)
	PeerName       string    `json:"peer_name,omitempty"`
	Channel        string    `json:"channel,omitempty"` // channel identifier (channel)
	Text           string    `json:"text"`
	TxtType        byte      `json:"txt_type,omitempty"`
	Timestamp      time.Time `json:"timestamp"`
	SNR            float64   `json:"snr,omitempty"`
	Read           bool      `json:"read"`
	Status         string    `json:"status,omitempty"`
	AckCode        string    `json:"ack_code,omitempty"`
	AcknowledgedAt time.Time `json:"acknowledged_at,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// MessageFilter narrows a Messages query. The zero value returns all messages,
// newest last.
type MessageFilter struct {
	Direction  string // "" for any, else MessageIn/MessageOut
	UnreadOnly bool
	Limit      int // 0 = no limit
}

// ContactStateEntry is a stored contact plus the time it was last written. The
// database is device-local state, not a cache: it may be stale, incomplete, or
// locally enriched.
type ContactStateEntry struct {
	Contact  meshcore.Contact `json:"contact"`
	StoredAt string           `json:"stored_at"`
}

// ChannelStateEntry is a stored channel plus the time it was last written.
type ChannelStateEntry struct {
	Channel  meshcore.Channel `json:"channel"`
	StoredAt string           `json:"stored_at"`
}

// Store persists one device's local state. Each store is already scoped to a
// single device (one database file per device public key), so methods take no
// device argument.
type Store interface {
	Close() error
	UpsertContacts(ctx context.Context, contacts []meshcore.Contact) error
	ClearContacts(ctx context.Context) error
	ContactLastMod(ctx context.Context) (uint32, error)
	SetContactLastMod(ctx context.Context, lastMod uint32) error
	UpsertContact(ctx context.Context, contact meshcore.Contact) error
	Contacts(ctx context.Context) ([]ContactStateEntry, error)
	Contact(ctx context.Context, query string) (ContactStateEntry, error)
	UpsertChannels(ctx context.Context, channels []meshcore.Channel) error
	Channels(ctx context.Context) ([]ChannelStateEntry, error)
	Channel(ctx context.Context, query string) (ChannelStateEntry, error)
	UpsertRepeaterSession(ctx context.Context, session meshcore.RepeaterSession) error
	RepeaterSession(ctx context.Context, repeater string) (meshcore.RepeaterSession, error)
	ClearRepeaterSession(ctx context.Context, repeater string) error
	InsertMessage(ctx context.Context, rec *MessageRecord) error
	SetMessageStatus(ctx context.Context, id int64, status, ackCode string) error
	SetMessageStatusByAck(ctx context.Context, ackCode, status string) error
	Messages(ctx context.Context, filter MessageFilter) ([]MessageRecord, error)
	MarkMessagesRead(ctx context.Context, ids []int64) error
}
