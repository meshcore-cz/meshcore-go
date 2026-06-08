package backend

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	meshcore "github.com/meshcore-cz/meshcore-go"
)

func testKey(fill byte) string {
	return strings.Repeat(string(fill), 64)
}

func TestStateStoreContactsRoundTrip(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	store, err := OpenStateStore(testKey('a'))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	contacts := make([]meshcore.Contact, 1300)
	for i := range contacts {
		contacts[i] = meshcore.Contact{
			Name:      "node",
			PublicKey: fmt.Sprintf("%064x", i),
			Type:      meshcore.ContactRepeater,
			OutPath:   nil, // exercise nil out-path handling
		}
	}
	if err := store.UpsertContacts(ctx, contacts); err != nil {
		t.Fatal(err)
	}
	got, err := store.Contacts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(contacts) {
		t.Fatalf("contacts = %d, want %d", len(got), len(contacts))
	}
}

func TestStateStoreIdentityRecorded(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	key := testKey('a')
	store, err := OpenStateStore(key)
	if err != nil {
		t.Fatal(err)
	}
	if store.PublicKey() != key {
		t.Fatalf("PublicKey() = %q, want %q", store.PublicKey(), key)
	}
	store.Close()

	// Reopening the same key reuses the database.
	store2, err := OpenStateStore(key)
	if err != nil {
		t.Fatalf("reopen same key: %v", err)
	}
	store2.Close()
}

func TestStateStoreIdentityMismatch(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	// Two keys share a 16-hex filename prefix but differ in the full key, so
	// they map to the same database file.
	keyA := strings.Repeat("a", 16) + strings.Repeat("b", 48)
	keyB := strings.Repeat("a", 16) + strings.Repeat("c", 48)
	if StateDBPath(keyA) != StateDBPath(keyB) {
		t.Fatalf("expected shared path for prefix collision")
	}

	store, err := OpenStateStore(keyA)
	if err != nil {
		t.Fatal(err)
	}
	store.Close()

	_, err = OpenStateStore(keyB)
	if !errors.Is(err, ErrIdentityMismatch) {
		t.Fatalf("OpenStateStore(keyB) error = %v, want ErrIdentityMismatch", err)
	}
}

func TestStateStoreRejectsInvalidKey(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	if _, err := OpenStateStore("not-hex"); err == nil {
		t.Fatal("expected error for invalid public key")
	}
}
