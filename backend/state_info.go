package backend

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// StateSummary describes one device's local-state database for `mc state`.
type StateSummary struct {
	Path             string    `json:"path"`
	Prefix           string    `json:"prefix"`
	PublicKey        string    `json:"public_key"`
	CreatedAt        time.Time `json:"created_at,omitempty"`
	SchemaVersion    string    `json:"schema_version,omitempty"`
	Contacts         int       `json:"contacts"`
	Channels         int       `json:"channels"`
	RepeaterSessions int       `json:"repeater_sessions"`
	ModTime          time.Time `json:"updated_at"`
	SizeBytes        int64     `json:"size_bytes"`
}

// ListStateSummaries returns a summary of every per-device local-state database.
func ListStateSummaries() ([]StateSummary, error) {
	dir := StateDevicesDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []StateSummary
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".db") {
			continue
		}
		sum, err := ReadStateSummary(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		out = append(out, sum)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Prefix < out[j].Prefix })
	return out, nil
}

// ReadStateSummary opens a local-state database read-only and reports its
// identity and row counts. It does not validate or alter identity.
func ReadStateSummary(path string) (StateSummary, error) {
	sum := StateSummary{
		Path:   path,
		Prefix: strings.TrimSuffix(filepath.Base(path), ".db"),
	}
	if fi, err := os.Stat(path); err == nil {
		sum.ModTime = fi.ModTime()
		sum.SizeBytes = fi.Size()
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return sum, err
	}
	defer db.Close()

	ctx := context.Background()
	meta := func(key string) string {
		var v string
		_ = db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = ?`, key).Scan(&v)
		return v
	}
	sum.PublicKey = meta(metaPublicKey)
	sum.SchemaVersion = meta(metaSchemaVersion)
	if created := meta(metaCreatedAt); created != "" {
		if t, err := time.Parse(time.RFC3339Nano, created); err == nil {
			sum.CreatedAt = t
		}
	}
	count := func(table string) int {
		var n int
		_ = db.QueryRowContext(ctx, `SELECT count(*) FROM `+table).Scan(&n)
		return n
	}
	sum.Contacts = count("contacts")
	sum.Channels = count("channels")
	sum.RepeaterSessions = count("repeater_sessions")
	return sum, nil
}

// PurgeState deletes a device's local-state database (and its sidecar files). It
// returns the paths that were removed.
func PurgeState(path string) ([]string, error) {
	var removed []string
	for _, p := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.Remove(p); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return removed, err
		}
		removed = append(removed, p)
	}
	return removed, nil
}
