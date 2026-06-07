package backend

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResetState(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	t.Setenv("XDG_RUNTIME_DIR", dir)

	mcDir := filepath.Join(dir, dirName)
	if err := os.MkdirAll(mcDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"backend.db", "backend.db-wal", "backend.log", sockName} {
		if err := os.WriteFile(filepath.Join(mcDir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	removed, err := ResetState()
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 4 {
		t.Fatalf("removed %d paths, want 4: %v", len(removed), removed)
	}
	for _, name := range []string{"backend.db", "backend.db-wal", "backend.log", sockName} {
		if _, err := os.Stat(filepath.Join(mcDir, name)); !os.IsNotExist(err) {
			t.Errorf("expected %s to be removed", name)
		}
	}
}
