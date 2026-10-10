package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// ManualHashPrefix marks the placeholder hash of a torrent added by hand
// without its info hash.
const ManualHashPrefix = "manual-"

func IsManualHash(hash string) bool { return strings.HasPrefix(hash, ManualHashPrefix) }

// HashLookup is a torrent whose real info hash is still unknown.
type HashLookup struct {
	Hash string
	Name string
	Size int64
}

// PendingHashLookups returns torrents with a placeholder hash that were
// never looked up, or whose last lookup failed before retryBefore. Never
// tried ones come first.
func (s *Store) PendingHashLookups(ctx context.Context, retryBefore time.Time, limit int) ([]HashLookup, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT hash, name, size FROM torrents
WHERE substr(hash, 1, length(?)) = ? AND (hash_lookup_at IS NULL OR hash_lookup_at < ?)
ORDER BY hash_lookup_at IS NOT NULL, hash_lookup_at, first_seen_at
LIMIT ?`, ManualHashPrefix, ManualHashPrefix, retryBefore.UTC().Format(time.RFC3339), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []HashLookup{}
	for rows.Next() {
		var h HashLookup
		if err := rows.Scan(&h.Hash, &h.Name, &h.Size); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// SetHashLookupError records why the hash could not be found. A final
// failure (not found, ambiguous) also records the attempt, so the torrent
// is not looked up again until the retry delay has passed; a temporary one
// (tracker unreachable) leaves it pending.
func (s *Store) SetHashLookupError(ctx context.Context, hash, msg string, final bool) error {
	q := `UPDATE torrents SET hash_lookup_error = ? WHERE hash = ?`
	args := []any{msg, hash}
	if final {
		q = `UPDATE torrents SET hash_lookup_error = ?, hash_lookup_at = ? WHERE hash = ?`
		args = []any{msg, now(), hash}
	}
	_, err := s.db.ExecContext(ctx, q, args...)
	return err
}

// TrackerTorrent is what the tracker told about a torrent found there.
type TrackerTorrent struct {
	Hash string
	URL  string // page of the torrent on the tracker
	File []byte // .torrent file, if it could be downloaded
}

// ReplaceHash gives the torrent stored under a placeholder hash its real
// info hash, moving its archives, adoptions and alerts along, and stores
// what the tracker told about it. If bookrr already knows the real hash
// (e.g. the torrent is still on a client), the placeholder is merged into
// it: the existing torrent keeps its data and gains what only the
// placeholder had. It reports whether a merge happened.
func (s *Store) ReplaceHash(ctx context.Context, oldHash string, found TrackerTorrent) (bool, error) {
	newHash := found.Hash
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	var old struct {
		size     int64
		tags     string
		personal bool
		notes    string
	}
	err = tx.QueryRowContext(ctx, `SELECT size, tags, personal_release, notes FROM torrents WHERE hash = ?`, oldHash).
		Scan(&old.size, &old.tags, &old.personal, &old.notes)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, err
	}
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM torrents WHERE hash = ?`, newHash).Scan(&exists); err != nil {
		return false, err
	}
	merged := exists > 0
	ts := now()
	if merged {
		_, err = tx.ExecContext(ctx, `
UPDATE torrents SET
	size = CASE WHEN size = 0 THEN ? ELSE size END,
	tags = CASE WHEN tags = '' THEN ? ELSE tags END,
	personal_release = MAX(personal_release, ?),
	notes = CASE WHEN notes = '' THEN ? WHEN ? = '' OR notes = ? THEN notes ELSE notes || char(10) || ? END,
	hash_lookup_at = ?, hash_lookup_error = '', tracker_url = ?, updated_at = ?
WHERE hash = ?`, old.size, old.tags, boolInt(old.personal), old.notes, old.notes, old.notes, old.notes, ts, found.URL, ts, newHash)
	} else {
		_, err = tx.ExecContext(ctx, `
INSERT INTO torrents (hash, name, size, tags, category, tracker, personal_release, manual, notes, first_seen_at, last_seen_at, updated_at, hash_lookup_at, hash_lookup_error, tracker_url)
SELECT ?, name, size, tags, category, tracker, personal_release, manual, notes, first_seen_at, last_seen_at, ?, ?, '', ?
FROM torrents WHERE hash = ?`, newHash, ts, ts, found.URL, oldHash)
	}
	if err != nil {
		return false, err
	}
	// The child tables reference torrents(hash) without ON UPDATE CASCADE:
	// move the rows, then drop the placeholder. Rows that would duplicate
	// one the existing torrent already has stay behind and go with it.
	for _, q := range []string{
		`UPDATE archives SET hash = ? WHERE hash = ?`,
		`UPDATE OR IGNORE adoptions SET hash = ? WHERE hash = ?`,
		`UPDATE alerts SET hash = ? WHERE hash = ?`,
		`UPDATE OR IGNORE locations SET hash = ? WHERE hash = ?`,
		`UPDATE OR IGNORE torrent_files SET hash = ? WHERE hash = ?`,
	} {
		if _, err := tx.ExecContext(ctx, q, newHash, oldHash); err != nil {
			return false, err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM torrents WHERE hash = ?`, oldHash); err != nil {
		return false, err
	}
	if len(found.File) > 0 {
		if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO torrent_files (hash, data, created_at) VALUES (?, ?, ?)`,
			newHash, found.File, ts); err != nil {
			return false, err
		}
	}
	if merged {
		// An archive or adoption coming from the placeholder answers an open
		// "where did it go?" alert of a torrent no longer on any client.
		var onClient int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM locations WHERE hash = ?`, newHash).Scan(&onClient); err != nil {
			return false, err
		}
		if onClient == 0 {
			desc, err := knownDestination(ctx, tx, newHash)
			if err != nil {
				return false, err
			}
			if desc != "" {
				if err := resolveOpenAlerts(ctx, tx, newHash, desc); err != nil {
					return false, err
				}
			}
		}
	}
	return merged, tx.Commit()
}

// TorrentFile returns the .torrent file saved for the torrent.
func (s *Store) TorrentFile(ctx context.Context, hash string) ([]byte, error) {
	var data []byte
	err := s.db.QueryRowContext(ctx, `SELECT data FROM torrent_files WHERE hash = ?`, hash).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return data, err
}
