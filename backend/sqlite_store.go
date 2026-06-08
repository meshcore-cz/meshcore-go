package backend

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	meshcore "github.com/meshcore-cz/meshcore-go"
	_ "modernc.org/sqlite"
)

const stateSchemaVersion = "1"

const (
	metaPublicKey      = "public_key"
	metaCreatedAt      = "created_at"
	metaSchemaVersion  = "schema_version"
	metaContactLastMod = "contact_last_mod"
)

// ErrIdentityMismatch is returned when a device's local-state database belongs
// to a different public key than the one being opened.
var ErrIdentityMismatch = errors.New("device identity mismatch")

// SQLiteStateStore is one device's persistent local state. Each store is a
// single database file scoped to one device public key; there is no device
// column. The database is device-local state, not a cache or replica.
type SQLiteStateStore struct {
	db        *sql.DB
	publicKey string
}

// OpenStateStore opens (creating if needed) the local-state database for a
// device public key at StateDBPath(publicKey). The full key is recorded in the
// meta table on creation and validated on every open: if the file already
// belongs to a different key, ErrIdentityMismatch is returned and no state is
// reused.
func OpenStateStore(publicKey string) (*SQLiteStateStore, error) {
	key := normalizePublicKey(publicKey)
	if !looksLikePublicKey(key) {
		return nil, fmt.Errorf("invalid device public key %q", publicKey)
	}
	path := StateDBPath(key)
	if err := os.MkdirAll(StateDevicesDir(), 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	s := &SQLiteStateStore{db: db, publicKey: key}
	if err := s.init(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.ensureIdentity(context.Background(), key); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// PublicKey returns the full device public key this store belongs to.
func (s *SQLiteStateStore) PublicKey() string { return s.publicKey }

func (s *SQLiteStateStore) Close() error { return s.db.Close() }

func (s *SQLiteStateStore) init(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS contacts (
	public_key TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	type TEXT NOT NULL,
	has_path INTEGER NOT NULL,
	latitude REAL NOT NULL,
	longitude REAL NOT NULL,
	last_advert TEXT NOT NULL,
	stored_at TEXT NOT NULL,
	out_path_enc INTEGER NOT NULL DEFAULT 255,
	out_path BLOB NOT NULL DEFAULT X''
);
CREATE INDEX IF NOT EXISTS idx_contacts_name ON contacts(name);
CREATE TABLE IF NOT EXISTS channels (
	idx INTEGER PRIMARY KEY,
	name TEXT NOT NULL,
	stored_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS repeater_sessions (
	repeater TEXT PRIMARY KEY,
	public_key TEXT NOT NULL,
	logged_in_at TEXT NOT NULL,
	expires_at TEXT NOT NULL,
	permissions INTEGER NOT NULL,
	tag INTEGER NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS meta (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
`)
	return err
}

// ensureIdentity records the device public key on first use and validates it on
// every subsequent open.
func (s *SQLiteStateStore) ensureIdentity(ctx context.Context, key string) error {
	stored, err := s.metaGet(ctx, metaPublicKey)
	if err != nil {
		return err
	}
	if stored == "" {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if err := s.metaSet(ctx, metaPublicKey, key); err != nil {
			return err
		}
		if err := s.metaSet(ctx, metaCreatedAt, now); err != nil {
			return err
		}
		return s.metaSet(ctx, metaSchemaVersion, stateSchemaVersion)
	}
	if normalizePublicKey(stored) != key {
		return fmt.Errorf("%w: database %s belongs to %s, not %s", ErrIdentityMismatch, StateDBPath(key), stored, key)
	}
	return nil
}

func (s *SQLiteStateStore) metaGet(ctx context.Context, key string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return value, nil
}

func (s *SQLiteStateStore) metaSet(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO meta(key, value) VALUES (?, ?)
ON CONFLICT(key) DO UPDATE SET value=excluded.value
`, key, value)
	return err
}

func (s *SQLiteStateStore) UpsertContacts(ctx context.Context, contacts []meshcore.Contact) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO contacts(public_key, name, type, has_path, latitude, longitude, last_advert, stored_at, out_path_enc, out_path)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(public_key) DO UPDATE SET
	name=excluded.name,
	type=excluded.type,
	has_path=excluded.has_path,
	latitude=excluded.latitude,
	longitude=excluded.longitude,
	last_advert=excluded.last_advert,
	stored_at=excluded.stored_at,
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
		if _, err := stmt.ExecContext(ctx, key, ct.Name, string(ct.Type), hasPath, ct.Latitude, ct.Longitude, lastAdvert, now, int(ct.OutPathEnc), outPath); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLiteStateStore) UpsertContact(ctx context.Context, contact meshcore.Contact) error {
	return s.UpsertContacts(ctx, []meshcore.Contact{contact})
}

func (s *SQLiteStateStore) Contacts(ctx context.Context) ([]ContactStateEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT public_key, name, type, has_path, latitude, longitude, last_advert, stored_at, out_path_enc, out_path
FROM contacts
ORDER BY lower(name)
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ContactStateEntry
	for rows.Next() {
		entry, err := scanContact(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

func (s *SQLiteStateStore) Contact(ctx context.Context, query string) (ContactStateEntry, error) {
	contacts, err := s.Contacts(ctx)
	if err != nil {
		return ContactStateEntry{}, err
	}
	q := strings.ToLower(strings.TrimSpace(query))
	var partial []ContactStateEntry
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
		return ContactStateEntry{}, sql.ErrNoRows
	default:
		return ContactStateEntry{}, fmt.Errorf("stored contact %q is ambiguous (%d matches)", query, len(partial))
	}
}

// UpsertChannels replaces the stored channel set. Channels are slot-based, so a
// full sync clears stale slots (renamed or removed channels) before inserting
// the current set.
func (s *SQLiteStateStore) UpsertChannels(ctx context.Context, channels []meshcore.Channel) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM channels`); err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO channels(idx, name, stored_at)
VALUES (?, ?, ?)
ON CONFLICT(idx) DO UPDATE SET
	name=excluded.name,
	stored_at=excluded.stored_at
`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, ch := range channels {
		if _, err := stmt.ExecContext(ctx, ch.Index, ch.Name, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLiteStateStore) Channels(ctx context.Context) ([]ChannelStateEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT idx, name, stored_at
FROM channels
ORDER BY idx
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ChannelStateEntry
	for rows.Next() {
		var ch meshcore.Channel
		var storedAt string
		if err := rows.Scan(&ch.Index, &ch.Name, &storedAt); err != nil {
			return nil, err
		}
		out = append(out, ChannelStateEntry{Channel: ch, StoredAt: storedAt})
	}
	return out, rows.Err()
}

func (s *SQLiteStateStore) Channel(ctx context.Context, query string) (ChannelStateEntry, error) {
	channels, err := s.Channels(ctx)
	if err != nil {
		return ChannelStateEntry{}, err
	}
	q := strings.TrimPrefix(strings.TrimSpace(query), "#")
	if idx, err := strconv.Atoi(q); err == nil {
		for _, entry := range channels {
			if entry.Channel.Index == idx {
				return entry, nil
			}
		}
		return ChannelStateEntry{}, sql.ErrNoRows
	}
	for _, entry := range channels {
		if strings.EqualFold(entry.Channel.Name, q) {
			return entry, nil
		}
	}
	return ChannelStateEntry{}, sql.ErrNoRows
}

type contactScanner interface {
	Scan(dest ...any) error
}

func scanContact(rows contactScanner) (ContactStateEntry, error) {
	var ct meshcore.Contact
	var typ string
	var hasPath int
	var outPathEnc int
	var outPath []byte
	var lastAdvert, storedAt string
	if err := rows.Scan(&ct.PublicKey, &ct.Name, &typ, &hasPath, &ct.Latitude, &ct.Longitude, &lastAdvert, &storedAt, &outPathEnc, &outPath); err != nil {
		return ContactStateEntry{}, err
	}
	ct.Type = meshcore.ContactType(typ)
	ct.HasPath = hasPath != 0
	ct.OutPathEnc = byte(outPathEnc)
	ct.OutPath = append([]byte(nil), outPath...)
	if lastAdvert != "" {
		t, err := time.Parse(time.RFC3339Nano, lastAdvert)
		if err != nil {
			return ContactStateEntry{}, err
		}
		ct.LastAdvert = t
	}
	return ContactStateEntry{Contact: ct, StoredAt: storedAt}, nil
}

func IsNotFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}

func (s *SQLiteStateStore) UpsertRepeaterSession(ctx context.Context, session meshcore.RepeaterSession) error {
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
INSERT INTO repeater_sessions(repeater, public_key, logged_in_at, expires_at, permissions, tag, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(repeater) DO UPDATE SET
	public_key=excluded.public_key,
	logged_in_at=excluded.logged_in_at,
	expires_at=excluded.expires_at,
	permissions=excluded.permissions,
	tag=excluded.tag,
	updated_at=excluded.updated_at
`, session.Repeater, session.PublicKey, loggedInAt, expiresAt, int(session.Permissions), session.Tag, now)
	return err
}

func (s *SQLiteStateStore) RepeaterSession(ctx context.Context, repeater string) (meshcore.RepeaterSession, error) {
	var sess meshcore.RepeaterSession
	var loggedInAt, expiresAt string
	var permissions int
	err := s.db.QueryRowContext(ctx, `
SELECT repeater, public_key, logged_in_at, expires_at, permissions, tag
FROM repeater_sessions
WHERE repeater = ?
`, repeater).Scan(&sess.Repeater, &sess.PublicKey, &loggedInAt, &expiresAt, &permissions, &sess.Tag)
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

func (s *SQLiteStateStore) ClearRepeaterSession(ctx context.Context, repeater string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM repeater_sessions WHERE repeater = ?`, repeater)
	return err
}

func (s *SQLiteStateStore) ClearContacts(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM contacts`)
	return err
}

func (s *SQLiteStateStore) ContactLastMod(ctx context.Context) (uint32, error) {
	value, err := s.metaGet(ctx, metaContactLastMod)
	if err != nil {
		return 0, err
	}
	if value == "" {
		return 0, nil
	}
	n, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("parsing contact last mod: %w", err)
	}
	return uint32(n), nil
}

func (s *SQLiteStateStore) SetContactLastMod(ctx context.Context, lastMod uint32) error {
	return s.metaSet(ctx, metaContactLastMod, strconv.FormatUint(uint64(lastMod), 10))
}

func looksLikePublicKey(key string) bool {
	if len(key) < statePrefixLen {
		return false
	}
	for _, c := range key {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
