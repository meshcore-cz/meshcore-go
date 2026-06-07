package backend

import (
	"fmt"
	"os"
)

// ResetState removes the local backend replica database and related state
// files. The backend process should be stopped before calling this.
func ResetState() ([]string, error) {
	paths := []string{
		DBPath(),
		DBPath() + "-wal",
		DBPath() + "-shm",
		LogPath(),
		SocketPath(),
	}

	var removed []string
	for _, path := range paths {
		if err := os.Remove(path); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return removed, fmt.Errorf("removing %s: %w", path, err)
		}
		removed = append(removed, path)
	}
	return removed, nil
}
