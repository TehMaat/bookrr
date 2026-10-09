// Package store persists bookrr data in SQLite.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("non trovato")

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	dsn := "file:" + path + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// A single connection keeps SQLite writes serialized and pragmas consistent.
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

var migrations = []string{
	`
CREATE TABLE clients (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL UNIQUE,
	url TEXT NOT NULL,
	username TEXT NOT NULL DEFAULT '',
	password TEXT NOT NULL DEFAULT '',
	skip_tls_verify INTEGER NOT NULL DEFAULT 0,
	enabled INTEGER NOT NULL DEFAULT 1,
	last_sync_at TEXT,
	last_error TEXT NOT NULL DEFAULT '',
	torrent_count INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL
);
CREATE TABLE torrents (
	hash TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	size INTEGER NOT NULL DEFAULT 0,
	tags TEXT NOT NULL DEFAULT '',
	category TEXT NOT NULL DEFAULT '',
	tracker TEXT NOT NULL DEFAULT '',
	personal_release INTEGER NOT NULL DEFAULT 0,
	manual INTEGER NOT NULL DEFAULT 0,
	adopted_by TEXT NOT NULL DEFAULT '',
	notes TEXT NOT NULL DEFAULT '',
	first_seen_at TEXT NOT NULL,
	last_seen_at TEXT,
	updated_at TEXT NOT NULL
);
CREATE TABLE locations (
	hash TEXT NOT NULL REFERENCES torrents(hash) ON DELETE CASCADE,
	client_id INTEGER NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
	save_path TEXT NOT NULL DEFAULT '',
	state TEXT NOT NULL DEFAULT '',
	progress REAL NOT NULL DEFAULT 0,
	ratio REAL NOT NULL DEFAULT 0,
	tags TEXT NOT NULL DEFAULT '',
	category TEXT NOT NULL DEFAULT '',
	added_on INTEGER NOT NULL DEFAULT 0,
	seen_at TEXT NOT NULL,
	PRIMARY KEY (hash, client_id)
);
CREATE INDEX locations_client ON locations(client_id);
CREATE TABLE disks (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	label TEXT NOT NULL,
	kind TEXT NOT NULL DEFAULT 'HDD',
	serial TEXT NOT NULL DEFAULT '',
	model TEXT NOT NULL DEFAULT '',
	capacity INTEGER NOT NULL DEFAULT 0,
	place TEXT NOT NULL DEFAULT '',
	notes TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL
);
CREATE TABLE archives (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	hash TEXT NOT NULL REFERENCES torrents(hash) ON DELETE CASCADE,
	disk_id INTEGER REFERENCES disks(id) ON DELETE SET NULL,
	path TEXT NOT NULL DEFAULT '',
	notes TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL
);
CREATE INDEX archives_hash ON archives(hash);
CREATE TABLE alerts (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	hash TEXT NOT NULL REFERENCES torrents(hash) ON DELETE CASCADE,
	source TEXT NOT NULL,
	client_name TEXT NOT NULL DEFAULT '',
	message TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	resolved_at TEXT,
	resolution TEXT NOT NULL DEFAULT ''
);
CREATE INDEX alerts_hash ON alerts(hash);
`,
	`
CREATE TABLE adopters (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL UNIQUE COLLATE NOCASE,
	contact TEXT NOT NULL DEFAULT '',
	notes TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL
);
CREATE TABLE adoptions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	hash TEXT NOT NULL REFERENCES torrents(hash) ON DELETE CASCADE,
	adopter_id INTEGER NOT NULL REFERENCES adopters(id) ON DELETE CASCADE,
	notes TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	UNIQUE (hash, adopter_id)
);
CREATE INDEX adoptions_adopter ON adoptions(adopter_id);
INSERT OR IGNORE INTO adopters (name, created_at)
	SELECT trim(adopted_by), MIN(updated_at) FROM torrents WHERE trim(adopted_by) != '' GROUP BY trim(adopted_by) COLLATE NOCASE;
INSERT OR IGNORE INTO adoptions (hash, adopter_id, created_at)
	SELECT t.hash, a.id, t.updated_at FROM torrents t JOIN adopters a ON a.name = trim(t.adopted_by);
ALTER TABLE torrents DROP COLUMN adopted_by;
`,
}

func (s *Store) migrate(ctx context.Context) error {
	var version int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	for i := version; i < len(migrations); i++ {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migrazione %d: %w", i+1, err)
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", i+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func now() string { return time.Now().UTC().Format(time.RFC3339) }

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// NormalizeTags turns qBittorrent's "a, b" tag list into a trimmed, comma-joined string.
func NormalizeTags(raw string) string {
	return strings.Join(SplitTags(raw), ",")
}

func SplitTags(raw string) []string {
	out := []string{}
	for _, t := range strings.Split(raw, ",") {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// HasTag reports whether the tag list contains tag, ignoring case.
func HasTag(raw, tag string) bool {
	for _, t := range SplitTags(raw) {
		if strings.EqualFold(t, tag) {
			return true
		}
	}
	return false
}

func nullStr(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	return &ns.String
}
