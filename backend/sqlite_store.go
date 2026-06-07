package backend

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	meshcore "github.com/meshcore-cz/meshcore-go"
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
CREATE TABLE IF NOT EXISTS channels (
	device TEXT NOT NULL,
	idx INTEGER NOT NULL,
	name TEXT NOT NULL,
	cached_at TEXT NOT NULL,
	PRIMARY KEY (device, idx)
);
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
CREATE TABLE IF NOT EXISTS device_meta (
	device TEXT NOT NULL,
	key TEXT NOT NULL,
	value TEXT NOT NULL,
	PRIMARY KEY (device, key)
);
`)
	if err != nil {
		return err
	}
	return s.migrateContacts(ctx)
}

func (s *SQLiteStore) migrateContacts(ctx context.Context) error {
	for _, stmt := range []string{
		`ALTER TABLE contacts ADD COLUMN out_path_enc INTEGER NOT NULL DEFAULT 255`,
		`ALTER TABLE contacts ADD COLUMN out_path BLOB NOT NULL DEFAULT X''`,
	} {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			if !strings.Contains(err.Error(), "duplicate column") {
				return err
			}
		}
	}
	return nil
}

func (s *SQLiteStore) UpsertContacts(ctx context.Context, device string, contacts []meshcore.Contact) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO contacts(device, public_key, name, type, has_path, latitude, longitude, last_advert, cached_at, out_path_enc, out_path)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(device, public_key) DO UPDATE SET
	name=excluded.name,
	type=excluded.type,
	has_path=excluded.has_path,
	latitude=excluded.latitude,
	longitude=excluded.longitude,
	last_advert=excluded.last_advert,
	cached_at=excluded.cached_at,
	out_path_enc=excluded.out_path_enc,
	out_path=excluded.out_path
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
		outPath := ct.OutPath
		if outPath == nil {
			outPath = []byte{}
		}
		if _, err := stmt.ExecContext(ctx, device, key, ct.Name, string(ct.Type), hasPath, ct.Latitude, ct.Longitude, lastAdvert, now, int(ct.OutPathEnc), outPath); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLiteStore) UpsertContact(ctx context.Context, device string, contact meshcore.Contact) error {
	return s.UpsertContacts(ctx, device, []meshcore.Contact{contact})
}

func (s *SQLiteStore) Contacts(ctx context.Context, device string) ([]ContactCacheEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT public_key, name, type, has_path, latitude, longitude, last_advert, cached_at, out_path_enc, out_path
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

// UpsertChannels replaces the replicated channel set for device. Channels are
// slot-based, so a full sync clears stale slots (renamed or removed channels)
// before inserting the current set.
func (s *SQLiteStore) UpsertChannels(ctx context.Context, device string, channels []meshcore.Channel) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM channels WHERE device = ?`, device); err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO channels(device, idx, name, cached_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(device, idx) DO UPDATE SET
	name=excluded.name,
	cached_at=excluded.cached_at
`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, ch := range channels {
		if _, err := stmt.ExecContext(ctx, device, ch.Index, ch.Name, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLiteStore) Channels(ctx context.Context, device string) ([]ChannelCacheEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT idx, name, cached_at
FROM channels
WHERE device = ?
ORDER BY idx
`, device)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ChannelCacheEntry
	for rows.Next() {
		var ch meshcore.Channel
		var cachedAt string
		if err := rows.Scan(&ch.Index, &ch.Name, &cachedAt); err != nil {
			return nil, err
		}
		out = append(out, ChannelCacheEntry{Channel: ch, Device: device, CachedAt: cachedAt})
	}
	return out, rows.Err()
}

func (s *SQLiteStore) Channel(ctx context.Context, device, query string) (ChannelCacheEntry, error) {
	channels, err := s.Channels(ctx, device)
	if err != nil {
		return ChannelCacheEntry{}, err
	}
	q := strings.TrimPrefix(strings.TrimSpace(query), "#")
	if idx, err := strconv.Atoi(q); err == nil {
		for _, entry := range channels {
			if entry.Channel.Index == idx {
				return entry, nil
			}
		}
		return ChannelCacheEntry{}, sql.ErrNoRows
	}
	for _, entry := range channels {
		if strings.EqualFold(entry.Channel.Name, q) {
			return entry, nil
		}
	}
	return ChannelCacheEntry{}, sql.ErrNoRows
}

type contactScanner interface {
	Scan(dest ...any) error
}

func scanContact(rows contactScanner, device string) (ContactCacheEntry, error) {
	var ct meshcore.Contact
	var typ string
	var hasPath int
	var outPathEnc int
	var outPath []byte
	var lastAdvert, cachedAt string
	if err := rows.Scan(&ct.PublicKey, &ct.Name, &typ, &hasPath, &ct.Latitude, &ct.Longitude, &lastAdvert, &cachedAt, &outPathEnc, &outPath); err != nil {
		return ContactCacheEntry{}, err
	}
	ct.Type = meshcore.ContactType(typ)
	ct.HasPath = hasPath != 0
	ct.OutPathEnc = byte(outPathEnc)
	ct.OutPath = append([]byte(nil), outPath...)
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

const contactLastModKey = "contact_last_mod"

func (s *SQLiteStore) ClearContacts(ctx context.Context, device string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM contacts WHERE device = ?`, device)
	return err
}

func (s *SQLiteStore) ContactLastMod(ctx context.Context, device string) (uint32, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM device_meta WHERE device = ? AND key = ?`, device, contactLastModKey).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	n, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("parsing contact last mod: %w", err)
	}
	return uint32(n), nil
}

func (s *SQLiteStore) SetContactLastMod(ctx context.Context, device string, lastMod uint32) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO device_meta(device, key, value) VALUES (?, ?, ?)
ON CONFLICT(device, key) DO UPDATE SET value=excluded.value
`, device, contactLastModKey, strconv.FormatUint(uint64(lastMod), 10))
	return err
}
