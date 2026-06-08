// Package backend implements a local MeshCore connection backend over IPC.
package backend

import (
	"os"
	"path/filepath"
)

const (
	dirName  = "mc"
	sockName = "backend.sock"
	logName  = "backend.log"

	devicesDir  = "devices"
	capturesDir = "captures"

	// statePrefixLen is how many hex characters of a device public key are used
	// in the on-disk filename. The full key is stored and validated in the DB
	// meta table; this is only to keep filenames short and human-scannable.
	statePrefixLen = 16
)

// StateDevicesDir returns the directory holding per-device local-state
// databases (~/.local/state/mc/devices).
func StateDevicesDir() string {
	return filepath.Join(stateDir(), dirName, devicesDir)
}

// CapturesDir returns the directory holding optional observer packet-history
// databases (~/.local/state/mc/captures).
func CapturesDir() string {
	return filepath.Join(stateDir(), dirName, capturesDir)
}

// StatePrefix returns the filesystem prefix for a device public key: the first
// statePrefixLen hex characters, lowercased.
func StatePrefix(publicKey string) string {
	key := normalizePublicKey(publicKey)
	if len(key) > statePrefixLen {
		key = key[:statePrefixLen]
	}
	return key
}

// StateDBPath returns the per-device local-state database path for a public key.
func StateDBPath(publicKey string) string {
	return filepath.Join(StateDevicesDir(), StatePrefix(publicKey)+".db")
}

// CaptureDBPath returns the optional packet-history database path for a public
// key, kept separate from device local state.
func CaptureDBPath(publicKey string) string {
	return filepath.Join(CapturesDir(), StatePrefix(publicKey)+".db")
}

// SocketPath returns the Unix socket path used by the local backend.
func SocketPath() string {
	if dir := os.Getenv("MC_BACKEND_SOCKET"); dir != "" {
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
