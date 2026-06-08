package backend

import (
	"context"

	meshcore "github.com/meshcore-cz/meshcore-go"
)

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
}
