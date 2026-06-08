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

	// Per-device state and capture databases.
	for _, d := range []string{StateDevicesDir(), CapturesDir()} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	stateFiles := []string{
		filepath.Join(StateDevicesDir(), "aaaaaaaaaaaaaaaa.db"),
		filepath.Join(StateDevicesDir(), "aaaaaaaaaaaaaaaa.db-wal"),
		filepath.Join(CapturesDir(), "aaaaaaaaaaaaaaaa.db"),
	}
	// Log and socket.
	if err := os.MkdirAll(filepath.Dir(LogPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	allFiles := append([]string{}, stateFiles...)
	allFiles = append(allFiles, LogPath(), SocketPath())
	for _, p := range allFiles {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	removed, err := ResetState()
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != len(allFiles) {
		t.Fatalf("removed %d paths, want %d: %v", len(removed), len(allFiles), removed)
	}
	for _, p := range allFiles {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("expected %s to be removed", p)
		}
	}
}
