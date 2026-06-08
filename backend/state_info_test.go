package backend

import (
	"context"
	"strings"
	"testing"

	meshcore "github.com/meshcore-cz/meshcore-go"
)

func TestStateSummaries(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	key := strings.Repeat("ab", 32) // 64 hex chars

	store, err := OpenStateStore(key)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := store.UpsertContacts(ctx, []meshcore.Contact{
		{Name: "alice", PublicKey: strings.Repeat("11", 32)},
		{Name: "bob", PublicKey: strings.Repeat("22", 32)},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertChannels(ctx, []meshcore.Channel{{Index: 0, Name: "public"}}); err != nil {
		t.Fatal(err)
	}
	store.Close()

	summaries, err := ListStateSummaries()
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 {
		t.Fatalf("summaries = %d, want 1", len(summaries))
	}
	got := summaries[0]
	if got.PublicKey != key {
		t.Fatalf("PublicKey = %q, want %q", got.PublicKey, key)
	}
	if got.Prefix != StatePrefix(key) {
		t.Fatalf("Prefix = %q, want %q", got.Prefix, StatePrefix(key))
	}
	if got.Contacts != 2 {
		t.Fatalf("Contacts = %d, want 2", got.Contacts)
	}
	if got.Channels != 1 {
		t.Fatalf("Channels = %d, want 1", got.Channels)
	}
	if got.CreatedAt.IsZero() {
		t.Fatal("CreatedAt is zero")
	}

	// Purge removes the database.
	removed, err := PurgeState(got.Path)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) == 0 {
		t.Fatal("PurgeState removed nothing")
	}
	after, err := ListStateSummaries()
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 0 {
		t.Fatalf("after purge: %d summaries, want 0", len(after))
	}
}
