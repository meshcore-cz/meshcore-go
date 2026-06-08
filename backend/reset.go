package backend

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResetState removes all device-local state databases, optional capture
// databases, and the backend log and socket. The backend daemon should be
// stopped before calling this.
func ResetState() ([]string, error) {
	var removed []string

	for _, dir := range []string{StateDevicesDir(), CapturesDir()} {
		dbs, err := removeDatabasesIn(dir)
		if err != nil {
			return removed, err
		}
		removed = append(removed, dbs...)
	}

	for _, path := range []string{LogPath(), SocketPath()} {
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

// removeDatabasesIn deletes every *.db (and -wal/-shm sidecar) in dir.
func removeDatabasesIn(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var removed []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !(strings.HasSuffix(name, ".db") || strings.HasSuffix(name, ".db-wal") || strings.HasSuffix(name, ".db-shm")) {
			continue
		}
		path := filepath.Join(dir, name)
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
