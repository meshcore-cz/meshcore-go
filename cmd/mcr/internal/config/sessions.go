package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// RepeaterSession is on-disk metadata from the last successful repeater login.
// It is a hint only; the companion radio's connection table is authoritative.
type RepeaterSession struct {
	Name        string    `json:"name"`
	PublicKey   string    `json:"public_key"`
	LoggedInAt  time.Time `json:"logged_in_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	Permissions byte      `json:"permissions"`
	Tag         int32     `json:"tag"`
}

// Active reports whether the cached session has not passed its expiry hint.
func (s RepeaterSession) Active() bool {
	return !s.ExpiresAt.IsZero() && time.Now().Before(s.ExpiresAt)
}

// SessionsDir returns the directory for cached repeater session files.
func SessionsDir() (string, error) {
	path, err := Path()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(path), "sessions"), nil
}

func repeaterSessionPath(publicKey string) (string, error) {
	dir, err := SessionsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, NormalizePublicKey(publicKey)+".json"), nil
}

// LoadRepeaterSession reads a cached session for the given public key.
func LoadRepeaterSession(publicKey string) (RepeaterSession, bool, error) {
	path, err := repeaterSessionPath(publicKey)
	if err != nil {
		return RepeaterSession{}, false, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return RepeaterSession{}, false, nil
	}
	if err != nil {
		return RepeaterSession{}, false, err
	}
	var sess RepeaterSession
	if err := json.Unmarshal(data, &sess); err != nil {
		return RepeaterSession{}, false, err
	}
	return sess, true, nil
}

// SaveRepeaterSession writes a cached session file.
func SaveRepeaterSession(sess RepeaterSession) error {
	path, err := repeaterSessionPath(sess.PublicKey)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// CachedRepeaterSession returns a non-expired on-disk session for a repeater
// name, public key, or key prefix.
func CachedRepeaterSession(cfg *Config, query string) (RepeaterSession, bool) {
	if key, _, ok := cfg.MatchRepeater(query); ok {
		if sess, ok, err := LoadRepeaterSession(key); err == nil && ok && sess.Active() {
			return sess, true
		}
	}
	if cfg.CurrentRepeater != "" {
		if sess, ok, err := LoadRepeaterSession(cfg.CurrentRepeater); err == nil && ok && sess.Active() {
			if query == "" || sess.Name == query {
				return sess, true
			}
		}
	}
	return RepeaterSession{}, false
}

// RemoveRepeaterSession deletes a cached session file.
func RemoveRepeaterSession(publicKey string) error {
	path, err := repeaterSessionPath(publicKey)
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
