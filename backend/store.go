package backend

import (
	"context"

	meshcore "github.com/meshcore-cz/meshcore-go"
)

// ContactCacheEntry is a replicated contact plus sync metadata.
type ContactCacheEntry struct {
	Contact  meshcore.Contact `json:"contact"`
	Device   string           `json:"device"`
	CachedAt string           `json:"cached_at"` // last replicated at (JSON field name kept for compatibility)
}

// ChannelCacheEntry is a replicated channel plus sync metadata.
type ChannelCacheEntry struct {
	Channel  meshcore.Channel `json:"channel"`
	Device   string           `json:"device"`
	CachedAt string           `json:"cached_at"` // last replicated at
}

// Store persists backend-local state such as the contact replica.
type Store interface {
	Close() error
	UpsertContacts(ctx context.Context, device string, contacts []meshcore.Contact) error
	ClearContacts(ctx context.Context, device string) error
	ContactLastMod(ctx context.Context, device string) (uint32, error)
	SetContactLastMod(ctx context.Context, device string, lastMod uint32) error
	UpsertContact(ctx context.Context, device string, contact meshcore.Contact) error
	Contacts(ctx context.Context, device string) ([]ContactCacheEntry, error)
	Contact(ctx context.Context, device, query string) (ContactCacheEntry, error)
	UpsertChannels(ctx context.Context, device string, channels []meshcore.Channel) error
	Channels(ctx context.Context, device string) ([]ChannelCacheEntry, error)
	Channel(ctx context.Context, device, query string) (ChannelCacheEntry, error)
	UpsertRepeaterSession(ctx context.Context, device string, session meshcore.RepeaterSession) error
	RepeaterSession(ctx context.Context, device, repeater string) (meshcore.RepeaterSession, error)
	ClearRepeaterSession(ctx context.Context, device, repeater string) error
}
