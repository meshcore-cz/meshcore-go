package backend

import (
	"context"

	meshcore "github.com/meshcore-dev/meshcore-go"
)

// ContactCacheEntry is a cached contact plus cache metadata.
type ContactCacheEntry struct {
	Contact  meshcore.Contact `json:"contact"`
	Device   string           `json:"device"`
	CachedAt string           `json:"cached_at"`
}

// Store persists backend-local state such as contact caches.
type Store interface {
	Close() error
	UpsertContacts(ctx context.Context, device string, contacts []meshcore.Contact) error
	UpsertContact(ctx context.Context, device string, contact meshcore.Contact) error
	Contacts(ctx context.Context, device string) ([]ContactCacheEntry, error)
	Contact(ctx context.Context, device, query string) (ContactCacheEntry, error)
	UpsertRepeaterSession(ctx context.Context, device string, session meshcore.RepeaterSession) error
	RepeaterSession(ctx context.Context, device, repeater string) (meshcore.RepeaterSession, error)
	ClearRepeaterSession(ctx context.Context, device, repeater string) error
}
