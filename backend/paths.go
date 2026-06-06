// Package backend implements a local MeshCore connection backend over IPC.
package backend

import (
	"os"
	"path/filepath"
)

const (
	dirName  = "mcr"
	sockName = "backend.sock"
	logName  = "backend.log"
)

// SocketPath returns the Unix socket path used by the local backend.
func SocketPath() string {
	if dir := os.Getenv("MCR_BACKEND_SOCKET"); dir != "" {
		return dir
	}
	return filepath.Join(runtimeDir(), dirName, sockName)
}

// LogPath returns the default log path for the background backend process.
func LogPath() string {
	return filepath.Join(stateDir(), dirName, logName)
}

func runtimeDir() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return dir
	}
	if dir, err := os.UserCacheDir(); err == nil {
		return dir
	}
	return os.TempDir()
}

func stateDir() string {
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return dir
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".local", "state")
	}
	return os.TempDir()
}
