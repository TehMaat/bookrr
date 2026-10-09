package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// SnapshotTorrent is one torrent as reported by a qBittorrent client.
type SnapshotTorrent struct {
	Hash     string
	Name     string
	Size     int64
	Tags     string // already normalized
	Category string
	Tracker  string
	SavePath string
	State    string
	Progress float64
	Ratio    float64
	AddedOn  int64
}

// Removal describes a torrent that disappeared from a client.
type Removal struct {
	Hash       string
	ClientName string
	Tags       string
}

// ApplySnapshot replaces the locations of one client with the given torrents
// and returns the torrents that are no longer present on it.
func (s *Store) ApplySnapshot(ctx context.Context, client Client, torrents []SnapshotTorrent) ([]Removal, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	prev := map[string]string{}
	rows, err := tx.QueryContext(ctx, `SELECT hash, tags FROM locations WHERE client_id = ?`, client.ID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var h, tags string
		if err := rows.Scan(&h, &tags); err != nil {
			rows.Close()
			return nil, err
		}
		prev[h] = tags
	}
	rows.Close()

	ts := now()
	upsertTorrent, err := tx.PrepareContext(ctx, `
INSERT INTO torrents (hash, name, size, tags, category, tracker, first_seen_at, last_seen_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(hash) DO UPDATE SET
	name = excluded.name,
	size = excluded.size,
	tags = excluded.tags,
	category = excluded.category,
	tracker = CASE WHEN excluded.tracker != '' THEN excluded.tracker ELSE torrents.tracker END,
	last_seen_at = excluded.last_seen_at`)
	if err != nil {
		return nil, err
	}
	defer upsertTorrent.Close()
	upsertLocation, err := tx.PrepareContext(ctx, `
INSERT INTO locations (hash, client_id, save_path, state, progress, ratio, tags, category, added_on, seen_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(hash, client_id) DO UPDATE SET
	save_path = excluded.save_path, state = excluded.state, progress = excluded.progress, ratio = excluded.ratio,
	tags = excluded.tags, category = excluded.category, added_on = excluded.added_on, seen_at = excluded.seen_at`)
	if err != nil {
		return nil, err
	}
	defer upsertLocation.Close()

	for _, t := range torrents {
		if _, err := upsertTorrent.ExecContext(ctx, t.Hash, t.Name, t.Size, t.Tags, t.Category, t.Tracker, ts, ts, ts); err != nil {
			return nil, fmt.Errorf("torrent %s: %w", t.Hash, err)
		}
		if _, err := upsertLocation.ExecContext(ctx, t.Hash, client.ID, t.SavePath, t.State, t.Progress, t.Ratio, t.Tags, t.Category, t.AddedOn, ts); err != nil {
			return nil, fmt.Errorf("posizione %s: %w", t.Hash, err)
		}
		delete(prev, t.Hash)
	}

	removed := make([]Removal, 0, len(prev))
	for h, tags := range prev {
		if _, err := tx.ExecContext(ctx, `DELETE FROM locations WHERE hash = ? AND client_id = ?`, h, client.ID); err != nil {
			return nil, err
		}
		removed = append(removed, Removal{Hash: h, ClientName: client.Name, Tags: tags})
	}
	if _, err := tx.ExecContext(ctx, `UPDATE clients SET last_error = '', last_sync_at = ?, torrent_count = ? WHERE id = ?`, ts, len(torrents), client.ID); err != nil {
		return nil, err
	}
	return removed, tx.Commit()
}

// FinalizeSync runs after every client has been synced. It flags removed
// personal releases that are no longer on any client, closes alerts for
// torrents that came back and forgets torrents nobody cares about anymore.
// It returns the number of new alerts.
func (s *Store) FinalizeSync(ctx context.Context, removed []Removal, releaseTag string) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// Locations of disabled clients are stale: drop them.
	if _, err := tx.ExecContext(ctx, `DELETE FROM locations WHERE client_id IN (SELECT id FROM clients WHERE enabled = 0)`); err != nil {
		return 0, err
	}

	// For torrents on a client, the release tag is the source of truth.
	if _, err := tx.ExecContext(ctx, `
UPDATE torrents SET personal_release = EXISTS (
	SELECT 1 FROM locations l WHERE l.hash = torrents.hash
	AND instr(',' || lower(l.tags) || ',', ',' || lower(?) || ',') > 0)
WHERE hash IN (SELECT hash FROM locations)`, releaseTag); err != nil {
		return 0, err
	}

	if _, err := tx.ExecContext(ctx, `
UPDATE alerts SET resolved_at = ?, resolution = 'Di nuovo presente su un client'
WHERE resolved_at IS NULL AND hash IN (SELECT hash FROM locations)`, now()); err != nil {
		return 0, err
	}

	created := 0
	seen := map[string]bool{}
	for _, r := range removed {
		if seen[r.Hash] {
			continue
		}
		seen[r.Hash] = true
		ok, err := flagRemoval(ctx, tx, r, releaseTag, "sync")
		if err != nil {
			return 0, err
		}
		if ok {
			created++
		}
	}

	if _, err := tx.ExecContext(ctx, `
DELETE FROM torrents
WHERE manual = 0 AND personal_release = 0 AND adopted_by = '' AND notes = ''
	AND NOT EXISTS (SELECT 1 FROM locations l WHERE l.hash = torrents.hash)
	AND NOT EXISTS (SELECT 1 FROM archives a WHERE a.hash = torrents.hash)
	AND NOT EXISTS (SELECT 1 FROM alerts al WHERE al.hash = torrents.hash)`); err != nil {
		return 0, err
	}
	return created, tx.Commit()
}

type txLike interface {
	execer
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// flagRemoval opens an alert for a personal release that is no longer on any
// client. It reports whether an alert was created.
func flagRemoval(ctx context.Context, tx txLike, r Removal, releaseTag, source string) (bool, error) {
	var remaining int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM locations WHERE hash = ?`, r.Hash).Scan(&remaining); err != nil {
		return false, err
	}
	if remaining > 0 {
		return false, nil
	}
	var personal bool
	err := tx.QueryRowContext(ctx, `SELECT personal_release FROM torrents WHERE hash = ?`, r.Hash).Scan(&personal)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !personal && !HasTag(r.Tags, releaseTag) {
		return false, nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE torrents SET personal_release = 1 WHERE hash = ?`, r.Hash); err != nil {
		return false, err
	}
	exists, err := openAlertExists(ctx, tx, r.Hash)
	if err != nil || exists {
		return false, err
	}
	msg := "Rimosso dal client"
	if r.ClientName != "" {
		msg = "Rimosso da " + r.ClientName
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO alerts (hash, source, client_name, message, created_at) VALUES (?, ?, ?, ?, ?)`,
		r.Hash, source, r.ClientName, msg, now())
	return err == nil, err
}

// WebhookRemoval is the payload of a "torrent removed" notification.
type WebhookRemoval struct {
	Hash       string
	Name       string
	Tags       string // normalized, may be empty if the sender doesn't know
	Category   string
	Size       int64
	ClientName string
}

// HandleWebhookRemoval records a removal reported by a webhook. It returns
// whether an alert was created.
func (s *Store) HandleWebhookRemoval(ctx context.Context, w WebhookRemoval, releaseTag string) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	ts := now()
	name := w.Name
	if name == "" {
		name = w.Hash
	}
	// Create the torrent if bookrr never saw it; otherwise keep what we know.
	if _, err := tx.ExecContext(ctx, `
INSERT INTO torrents (hash, name, size, tags, category, first_seen_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(hash) DO NOTHING`, w.Hash, name, w.Size, w.Tags, w.Category, ts, ts); err != nil {
		return false, err
	}

	tags := w.Tags
	if w.ClientName != "" {
		var clientID int64
		err := tx.QueryRowContext(ctx, `SELECT id FROM clients WHERE name = ? COLLATE NOCASE`, w.ClientName).Scan(&clientID)
		switch {
		case err == nil:
			var locTags string
			if err := tx.QueryRowContext(ctx, `SELECT tags FROM locations WHERE hash = ? AND client_id = ?`, w.Hash, clientID).Scan(&locTags); err == nil && tags == "" {
				tags = locTags
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM locations WHERE hash = ? AND client_id = ?`, w.Hash, clientID); err != nil {
				return false, err
			}
		case !errors.Is(err, sql.ErrNoRows):
			return false, err
		}
	}
	if HasTag(tags, releaseTag) {
		if _, err := tx.ExecContext(ctx, `UPDATE torrents SET personal_release = 1 WHERE hash = ?`, w.Hash); err != nil {
			return false, err
		}
	}

	// If the torrent is still listed on a client (another client, or the
	// sender is unknown) no alert is opened: the next sync decides.
	created, err := flagRemoval(ctx, tx, Removal{Hash: w.Hash, ClientName: w.ClientName, Tags: tags}, releaseTag, "webhook")
	if err != nil {
		return false, err
	}
	return created, tx.Commit()
}
