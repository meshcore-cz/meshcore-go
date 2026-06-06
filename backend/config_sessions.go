package backend

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	meshcore "github.com/meshcore-cz/meshcore-go"
)

type fileRepeaterSession struct {
	Name       string    `json:"name"`
	PublicKey  string    `json:"public_key"`
	LoggedInAt time.Time `json:"logged_in_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}

func configSessionsDir() string {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "mc", "sessions")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "mc", "sessions")
}

func loadConfigRepeaterSession(query string) (meshcore.RepeaterSession, bool) {
	dir := configSessionsDir()
	if dir == "" {
		return meshcore.RepeaterSession{}, false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return meshcore.RepeaterSession{}, false
	}
	query = strings.TrimSpace(query)
	queryKey := normalizePublicKey(query)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var sess fileRepeaterSession
		if err := json.Unmarshal(data, &sess); err != nil {
			continue
		}
		if !configSessionActive(sess) {
			continue
		}
		if query == "" ||
			strings.EqualFold(sess.Name, query) ||
			normalizePublicKey(sess.PublicKey) == queryKey ||
			(queryKey != "" && strings.HasPrefix(normalizePublicKey(sess.PublicKey), queryKey)) {
			return meshcore.RepeaterSession{
				Repeater:   sess.Name,
				PublicKey:  sess.PublicKey,
				LoggedInAt: sess.LoggedInAt,
				ExpiresAt:  sess.ExpiresAt,
			}, true
		}
	}
	return meshcore.RepeaterSession{}, false
}

func configSessionActive(sess fileRepeaterSession) bool {
	return !sess.ExpiresAt.IsZero() && time.Now().Before(sess.ExpiresAt)
}

func normalizePublicKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}
