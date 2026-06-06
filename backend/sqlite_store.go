package backend

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	meshcore "github.com/meshcore-dev/meshcore-go"
	_ "modernc.org/sqlite"
)

// DBPath returns the default SQLite path for backend-local state.
func DBPath() string {
	return filepath.Join(stateDir(), dirName, "backend.db")
}

type SQLiteStore struct {
	db *sql.DB
}

func OpenSQLiteStore(path string) (*SQLiteStore, error) {
	if path == "" {
		path = DBPath()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	s := &SQLiteStore{db: db}
	if err := s.init(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) init(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS contacts (
	device TEXT NOT NULL,
	public_key TEXT NOT NULL,
	name TEXT NOT NULL,
	type TEXT NOT NULL,
	has_path INTEGER NOT NULL,
	latitude REAL NOT NULL,
	longitude REAL NOT NULL,
	last_advert TEXT NOT NULL,
	cached_at TEXT NOT NULL,
	PRIMARY KEY (device, public_key)
);
CREATE INDEX IF NOT EXISTS idx_contacts_device_name ON contacts(device, name);
CREATE TABLE IF NOT EXISTS repeater_sessions (
	device TEXT NOT NULL,
	repeater TEXT NOT NULL,
	public_key TEXT NOT NULL,
	logged_in_at TEXT NOT NULL,
	expires_at TEXT NOT NULL,
	permissions INTEGER NOT NULL,
	tag INTEGER NOT NULL,
	updated_at TEXT NOT NULL,
	PRIMARY KEY (device, repeater)
);
`)
	return err
}

func (s *SQLiteStore) UpsertContacts(ctx context.Context, device string, contacts []meshcore.Contact) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO contacts(device, public_key, name, type, has_path, latitude, longitude, last_advert, cached_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(device, public_key) DO UPDATE SET
	name=excluded.name,
	type=excluded.type,
	has_path=excluded.has_path,
	latitude=excluded.latitude,
	longitude=excluded.longitude,
	last_advert=excluded.last_advert,
	cached_at=excluded.cached_at
`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, ct := range contacts {
		key := ct.PublicKey
		if key == "" {
			key = "name:" + strings.ToLower(ct.Name)
		}
		lastAdvert := ""
		if !ct.LastAdvert.IsZero() {
			lastAdvert = ct.LastAdvert.UTC().Format(time.RFC3339Nano)
		}
		hasPath := 0
		if ct.HasPath {
			hasPath = 1
		}
		if _, err := stmt.ExecContext(ctx, device, key, ct.Name, string(ct.Type), hasPath, ct.Latitude, ct.Longitude, lastAdvert, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLiteStore) Contacts(ctx context.Context, device string) ([]ContactCacheEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT public_key, name, type, has_path, latitude, longitude, last_advert, cached_at
FROM contacts
WHERE device = ?
ORDER BY lower(name)
`, device)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ContactCacheEntry
	for rows.Next() {
		entry, err := scanContact(rows, device)
		if err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) Contact(ctx context.Context, device, query string) (ContactCacheEntry, error) {
	contacts, err := s.Contacts(ctx, device)
	if err != nil {
		return ContactCacheEntry{}, err
	}
	q := strings.ToLower(strings.TrimSpace(query))
	var partial []ContactCacheEntry
	for _, entry := range contacts {
		ct := entry.Contact
		name := strings.ToLower(ct.Name)
		key := strings.ToLower(ct.PublicKey)
		if name == q || key == q {
			return entry, nil
		}
		if strings.Contains(name, q) || strings.HasPrefix(key, q) {
			partial = append(partial, entry)
		}
	}
	switch len(partial) {
	case 1:
		return partial[0], nil
	case 0:
		return ContactCacheEntry{}, sql.ErrNoRows
	default:
		return ContactCacheEntry{}, fmt.Errorf("cached contact %q is ambiguous (%d matches)", query, len(partial))
	}
}

type contactScanner interface {
	Scan(dest ...any) error
}

func scanContact(rows contactScanner, device string) (ContactCacheEntry, error) {
	var ct meshcore.Contact
	var typ string
	var hasPath int
	var lastAdvert, cachedAt string
	if err := rows.Scan(&ct.PublicKey, &ct.Name, &typ, &hasPath, &ct.Latitude, &ct.Longitude, &lastAdvert, &cachedAt); err != nil {
		return ContactCacheEntry{}, err
	}
	ct.Type = meshcore.ContactType(typ)
	ct.HasPath = hasPath != 0
	if lastAdvert != "" {
		t, err := time.Parse(time.RFC3339Nano, lastAdvert)
		if err != nil {
			return ContactCacheEntry{}, err
		}
		ct.LastAdvert = t
	}
	return ContactCacheEntry{Contact: ct, Device: device, CachedAt: cachedAt}, nil
}

func IsNotFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}

func (s *SQLiteStore) UpsertRepeaterSession(ctx context.Context, device string, session meshcore.RepeaterSession) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	loggedInAt := ""
	if !session.LoggedInAt.IsZero() {
		loggedInAt = session.LoggedInAt.UTC().Format(time.RFC3339Nano)
	}
	expiresAt := ""
	if !session.ExpiresAt.IsZero() {
		expiresAt = session.ExpiresAt.UTC().Format(time.RFC3339Nano)
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO repeater_sessions(device, repeater, public_key, logged_in_at, expires_at, permissions, tag, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(device, repeater) DO UPDATE SET
	public_key=excluded.public_key,
	logged_in_at=excluded.logged_in_at,
	expires_at=excluded.expires_at,
	permissions=excluded.permissions,
	tag=excluded.tag,
	updated_at=excluded.updated_at
`, device, session.Repeater, session.PublicKey, loggedInAt, expiresAt, int(session.Permissions), session.Tag, now)
	return err
}

func (s *SQLiteStore) RepeaterSession(ctx context.Context, device, repeater string) (meshcore.RepeaterSession, error) {
	var sess meshcore.RepeaterSession
	var loggedInAt, expiresAt string
	var permissions int
	err := s.db.QueryRowContext(ctx, `
SELECT repeater, public_key, logged_in_at, expires_at, permissions, tag
FROM repeater_sessions
WHERE device = ? AND repeater = ?
`, device, repeater).Scan(&sess.Repeater, &sess.PublicKey, &loggedInAt, &expiresAt, &permissions, &sess.Tag)
	if err != nil {
		return meshcore.RepeaterSession{}, err
	}
	if loggedInAt != "" {
		t, err := time.Parse(time.RFC3339Nano, loggedInAt)
		if err != nil {
			return meshcore.RepeaterSession{}, err
		}
		sess.LoggedInAt = t
	}
	if expiresAt != "" {
		t, err := time.Parse(time.RFC3339Nano, expiresAt)
		if err != nil {
			return meshcore.RepeaterSession{}, err
		}
		sess.ExpiresAt = t
	}
	sess.Permissions = byte(permissions)
	return sess, nil
}

func (s *SQLiteStore) ClearRepeaterSession(ctx context.Context, device, repeater string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM repeater_sessions WHERE device = ? AND repeater = ?`, device, repeater)
	return err
}
