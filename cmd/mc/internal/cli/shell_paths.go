package cli

import (
	"os"
	"path/filepath"
)

func cliStateDir() string {
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return dir
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".local", "state")
	}
	return os.TempDir()
}

func shellHistoryPath() (string, error) {
	dir := filepath.Join(cliStateDir(), "mc")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "history"), nil
}
