package backend

import (
	"context"
	"path/filepath"
	"testing"

	meshcore "github.com/meshcore-cz/meshcore-go"
)

func TestSQLiteStoreUpsertContactNilOutPath(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenSQLiteStore(filepath.Join(dir, "backend.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	contacts := make([]meshcore.Contact, 1300)
	for i := range contacts {
		contacts[i] = meshcore.Contact{
			Name:      "node",
			PublicKey: "aa",
			Type:      meshcore.ContactRepeater,
			OutPath:   nil,
		}
	}
	if err := store.UpsertContacts(ctx, "ble://test", contacts); err != nil {
		t.Fatal(err)
	}
}
