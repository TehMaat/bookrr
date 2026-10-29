package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// TrackerUpload is one of the user's uploads as listed by the tracker.
type TrackerUpload struct {
	ID              int64
	Name            string
	Category        string
	Type            string
	TypeID          int64
	Resolution      string
	ResolutionID    int64
	Size            int64
	Seeders         int64
	Leechers        int64
	TimesCompleted  int64
	TmdbID          int64
	ImdbID          int64
	TvdbID          int64
	PersonalRelease bool
	CreatedAt       string
	DetailsLink     string
}

// TrackerRelease is an upload of the user on the tracker, with the
// torrent of bookrr it is linked to (same info hash), if any.
type TrackerRelease struct {
	ID              int64   `json:"id"`
	Name            string  `json:"name"`
	Category        string  `json:"category"`
	Type            string  `json:"type"`
	TypeID          int64   `json:"typeId"`
	Resolution      string  `json:"resolution"`
	ResolutionID    int64   `json:"resolutionId"`
	Size            int64   `json:"size"`
	Seeders         int64   `json:"seeders"`
	Leechers        int64   `json:"leechers"`
	TimesCompleted  int64   `json:"timesCompleted"`
	TmdbID          int64   `json:"tmdbId"`
	ImdbID          int64   `json:"imdbId"`
	TvdbID          int64   `json:"tvdbId"`
	PersonalRelease bool    `json:"personalRelease"`
	CreatedAt       string  `json:"createdAt"`
	DetailsLink     string  `json:"detailsLink"`
	Hash            string  `json:"hash"`
	HashError       string  `json:"hashError"`
	HasTorrentFile  bool    `json:"hasTorrentFile"`
	SeenAt          string  `json:"seenAt"`
	RemovedAt       *string `json:"removedAt"`
	UpdatedAt       string  `json:"updatedAt"`
	// Hash of the linked torrent of bookrr, empty if there is none.
	TorrentHash string `json:"torrentHash"`
	// Status of the linked torrent, or ReleaseUntracked.
	Status string `json:"status"`
	// How it stands against the archive rule; nil when it is on no client.
	Archivable *Archivability `json:"archivable"`
}

// ReleaseUntracked is the status of an upload bookrr knows no torrent for.
const ReleaseUntracked = "untracked"

const releasesSyncedKey = "tracker_releases_synced_at"

// SaveTrackerReleases records the complete list of the user's uploads:
// counters and details are refreshed, the hash already computed is kept,
// and uploads that are no longer listed are marked as removed from the
// tracker (they stay in the history). It returns how many are new.
func (s *Store) SaveTrackerReleases(ctx context.Context, ups []TrackerUpload) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	ts := now()
	var before int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM tracker_releases`).Scan(&before); err != nil {
		return 0, err
	}
	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO tracker_releases (id, name, category, type, type_id, resolution, resolution_id, size, seeders, leechers, times_completed,
	tmdb_id, imdb_id, tvdb_id, personal_release, created_at, details_link, seen_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET
	name = excluded.name, category = excluded.category, type = excluded.type, type_id = excluded.type_id,
	resolution = excluded.resolution, resolution_id = excluded.resolution_id, size = excluded.size,
	seeders = excluded.seeders, leechers = excluded.leechers, times_completed = excluded.times_completed,
	tmdb_id = excluded.tmdb_id, imdb_id = excluded.imdb_id, tvdb_id = excluded.tvdb_id,
	personal_release = excluded.personal_release,
	created_at = CASE WHEN excluded.created_at != '' THEN excluded.created_at ELSE tracker_releases.created_at END,
	details_link = excluded.details_link, seen_at = excluded.seen_at, removed_at = NULL, updated_at = excluded.updated_at`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	for _, u := range ups {
		if _, err := stmt.ExecContext(ctx, u.ID, u.Name, u.Category, u.Type, u.TypeID, u.Resolution, u.ResolutionID, u.Size,
			u.Seeders, u.Leechers, u.TimesCompleted, u.TmdbID, u.ImdbID, u.TvdbID, boolInt(u.PersonalRelease),
			u.CreatedAt, u.DetailsLink, ts, ts); err != nil {
			return 0, err
		}
	}
	listed := make(map[int64]bool, len(ups))
	for _, u := range ups {
		listed[u.ID] = true
	}
	var gone []int64
	rows, err := tx.QueryContext(ctx, `SELECT id FROM tracker_releases WHERE removed_at IS NULL`)
	if err != nil {
		return 0, err
	}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		if !listed[id] {
			gone = append(gone, id)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, id := range gone {
		if _, err := tx.ExecContext(ctx, `UPDATE tracker_releases SET removed_at = ? WHERE id = ?`, ts, id); err != nil {
			return 0, err
		}
	}
	var after int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM tracker_releases`).Scan(&after); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT (key) DO UPDATE SET value = excluded.value`,
		releasesSyncedKey, ts); err != nil {
		return 0, err
	}
	return after - before, tx.Commit()
}

// TrackerReleasesSyncedAt is when the list of uploads was last read in full.
func (s *Store) TrackerReleasesSyncedAt(ctx context.Context) (*string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, releasesSyncedKey).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) || v == "" {
		return nil, nil
	}
	return &v, err
}

// TrackerReleasesWithoutHash returns the uploads still listed on the
// tracker whose .torrent file was never downloaded successfully.
func (s *Store) TrackerReleasesWithoutHash(ctx context.Context) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM tracker_releases WHERE hash = '' AND removed_at IS NULL ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// SetTrackerReleaseHash records the info hash (and .torrent file) of an
// upload. The hash never changes, so the file is downloaded only once.
func (s *Store) SetTrackerReleaseHash(ctx context.Context, id int64, hash string, file []byte) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE tracker_releases SET hash = ?, hash_error = '', updated_at = ? WHERE id = ?`,
		strings.ToLower(hash), now(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	if len(file) > 0 {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO tracker_release_files (release_id, data) VALUES (?, ?) ON CONFLICT (release_id) DO UPDATE SET data = excluded.data`,
			id, file); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return s.LinkTrackerReleases(ctx)
}

// SetTrackerReleaseHashError records why the hash of an upload is unknown.
func (s *Store) SetTrackerReleaseHashError(ctx context.Context, id int64, msg string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE tracker_releases SET hash_error = ? WHERE id = ?`, msg, id)
	return err
}

// LinkTrackerReleases gives the torrents of bookrr that match an upload by
// hash what only the tracker knows: the .torrent file, the page link and,
// for torrents added by hand without it, the size.
func (s *Store) LinkTrackerReleases(ctx context.Context) error {
	ts := now()
	if _, err := s.db.ExecContext(ctx, `
UPDATE torrents SET size = (SELECT MAX(r.size) FROM tracker_releases r WHERE r.hash = torrents.hash)
WHERE size = 0 AND EXISTS (SELECT 1 FROM tracker_releases r WHERE r.hash = torrents.hash AND r.size > 0)`); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `
INSERT OR IGNORE INTO torrent_files (hash, data, created_at)
SELECT r.hash, f.data, ? FROM tracker_releases r
	JOIN tracker_release_files f ON f.release_id = r.id
	JOIN torrents t ON t.hash = r.hash
WHERE r.hash != '' AND NOT EXISTS (SELECT 1 FROM torrent_files x WHERE x.hash = r.hash)`, ts); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `
UPDATE torrents SET tracker_url = (
	SELECT r.details_link FROM tracker_releases r WHERE r.hash = torrents.hash AND r.details_link != '' ORDER BY r.removed_at IS NOT NULL, r.id DESC LIMIT 1)
WHERE tracker_url = '' AND EXISTS (SELECT 1 FROM tracker_releases r WHERE r.hash = torrents.hash AND r.details_link != '')`)
	return err
}

// ReleaseCandidate is an upload whose hash is known, for matching the
// torrents added without one by name.
type ReleaseCandidate struct {
	ID          int64
	Name        string
	Size        int64
	Hash        string
	DetailsLink string
	Removed     bool
}

func (s *Store) ReleaseCandidates(ctx context.Context) ([]ReleaseCandidate, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, size, hash, details_link, removed_at IS NOT NULL FROM tracker_releases WHERE hash != '' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ReleaseCandidate{}
	for rows.Next() {
		var c ReleaseCandidate
		if err := rows.Scan(&c.ID, &c.Name, &c.Size, &c.Hash, &c.DetailsLink, &c.Removed); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ManualTorrents returns every torrent that still has a placeholder hash.
func (s *Store) ManualTorrents(ctx context.Context) ([]HashLookup, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT hash, name, size FROM torrents WHERE substr(hash, 1, length(?)) = ? ORDER BY first_seen_at`, ManualHashPrefix, ManualHashPrefix)
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

const releaseCols = `r.id, r.name, r.category, r.type, r.type_id, r.resolution, r.resolution_id, r.size, r.seeders, r.leechers,
	r.times_completed, r.tmdb_id, r.imdb_id, r.tvdb_id, r.personal_release, r.created_at, r.details_link, r.hash, r.hash_error,
	EXISTS (SELECT 1 FROM tracker_release_files f WHERE f.release_id = r.id), r.seen_at, r.removed_at, r.updated_at, r.archivable_since, COALESCE(t.hash, ''), COALESCE(t.never_archive, 0),
	CASE
		WHEN t.hash IS NULL THEN '` + ReleaseUntracked + `'
		WHEN EXISTS (SELECT 1 FROM locations l WHERE l.hash = t.hash) THEN '` + StatusClient + `'
		WHEN EXISTS (SELECT 1 FROM alerts a WHERE a.hash = t.hash AND a.resolved_at IS NULL) THEN '` + StatusMissing + `'
		WHEN EXISTS (SELECT 1 FROM archives a WHERE a.hash = t.hash) THEN '` + StatusArchived + `'
		WHEN EXISTS (SELECT 1 FROM adoptions a WHERE a.hash = t.hash) THEN '` + StatusAdopted + `'
		ELSE '` + StatusUnknown + `'
	END`

// ListTrackerReleases returns the uploads, newest first, each with the
// status of the torrent of bookrr it is linked to (the same status as in
// the list of torrents).
func (s *Store) ListTrackerReleases(ctx context.Context) ([]TrackerRelease, error) {
	return s.loadReleases(ctx, "")
}

func (s *Store) GetTrackerRelease(ctx context.Context, id int64) (TrackerRelease, error) {
	rs, err := s.loadReleases(ctx, " WHERE r.id = ?", id)
	if err != nil {
		return TrackerRelease{}, err
	}
	if len(rs) == 0 {
		return TrackerRelease{}, ErrNotFound
	}
	return rs[0], nil
}

func (s *Store) loadReleases(ctx context.Context, where string, args ...any) ([]TrackerRelease, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+releaseCols+`
FROM tracker_releases r LEFT JOIN torrents t ON r.hash != '' AND t.hash = r.hash`+where+`
ORDER BY r.created_at DESC, r.id DESC`, args...)
	if err != nil {
		return nil, err
	}
	out := []TrackerRelease{}
	var since []*string
	var never []bool
	for rows.Next() {
		var r TrackerRelease
		var keep bool
		var removed, archivable sql.NullString
		if err := rows.Scan(&r.ID, &r.Name, &r.Category, &r.Type, &r.TypeID, &r.Resolution, &r.ResolutionID, &r.Size,
			&r.Seeders, &r.Leechers, &r.TimesCompleted, &r.TmdbID, &r.ImdbID, &r.TvdbID, &r.PersonalRelease, &r.CreatedAt,
			&r.DetailsLink, &r.Hash, &r.HashError, &r.HasTorrentFile, &r.SeenAt, &removed, &r.UpdatedAt, &archivable,
			&r.TorrentHash, &keep, &r.Status); err != nil {
			rows.Close()
			return nil, err
		}
		r.RemovedAt = nullStr(removed)
		out = append(out, r)
		since = append(since, nullStr(archivable))
		never = append(never, keep)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	locs, err := s.seedLocations(ctx)
	if err != nil {
		return nil, err
	}
	rule, err := s.archiveRule(ctx)
	if err != nil {
		return nil, err
	}
	now := s.clock()
	for i, r := range out {
		if r.TorrentHash == "" {
			continue
		}
		c := releaseCounters{id: r.ID, hash: r.Hash, seeders: r.Seeders, completed: r.TimesCompleted, seenAt: r.SeenAt,
			removed: r.RemovedAt != nil, since: since[i]}
		out[i].Archivable = assessArchivable(c, locs[r.TorrentHash], rule, now).neverArchive(never[i])
	}
	return out, nil
}

var ErrNoReleaseHash = errors.New("l'hash di questa release non è ancora stato calcolato: riprova dopo il prossimo aggiornamento")

// AddTrackerReleases creates in bookrr the torrents of uploads it does not
// know yet, as personal releases with their hash, name, size, page link and
// .torrent file, so the user can then record where they are. With no ids
// every unknown upload whose hash is known is added. It returns how many
// torrents were created.
func (s *Store) AddTrackerReleases(ctx context.Context, ids []int64) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if len(ids) == 0 {
		rows, err := tx.QueryContext(ctx, `
SELECT id FROM tracker_releases r WHERE hash != '' AND NOT EXISTS (SELECT 1 FROM torrents t WHERE t.hash = r.hash) ORDER BY id`)
		if err != nil {
			return 0, err
		}
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return 0, err
			}
			ids = append(ids, id)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return 0, err
		}
	}
	ts := now()
	added := 0
	for _, id := range ids {
		var name, hash, link string
		var size int64
		var file []byte
		err := tx.QueryRowContext(ctx, `
SELECT r.name, r.size, r.hash, r.details_link, f.data
FROM tracker_releases r LEFT JOIN tracker_release_files f ON f.release_id = r.id WHERE r.id = ?`, id).
			Scan(&name, &size, &hash, &link, &file)
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrNotFound
		}
		if err != nil {
			return 0, err
		}
		if hash == "" {
			return 0, ErrNoReleaseHash
		}
		res, err := tx.ExecContext(ctx, `
INSERT INTO torrents (hash, name, size, personal_release, manual, first_seen_at, updated_at, hash_lookup_at, tracker_url)
VALUES (?, ?, ?, 1, 1, ?, ?, ?, ?) ON CONFLICT (hash) DO NOTHING`, hash, name, size, ts, ts, ts, link)
		if err != nil {
			return 0, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			if len(ids) == 1 {
				return 0, ErrExists
			}
			continue
		}
		added++
		if len(file) > 0 {
			if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO torrent_files (hash, data, created_at) VALUES (?, ?, ?)`, hash, file, ts); err != nil {
				return 0, err
			}
		}
	}
	return added, tx.Commit()
}

// NotOnTracker is a personal release of bookrr missing from the uploads.
type NotOnTracker struct {
	Hash    string
	Name    string
	Tracker string
}

// PersonalReleasesNotOnTracker returns the personal releases of bookrr
// whose hash is not among the uploads still on the tracker: deleted or
// replaced there, uploaded elsewhere, or added without their hash.
func (s *Store) PersonalReleasesNotOnTracker(ctx context.Context) ([]NotOnTracker, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT hash, name, tracker FROM torrents t WHERE personal_release = 1
	AND NOT EXISTS (SELECT 1 FROM tracker_releases r WHERE r.hash = t.hash AND r.removed_at IS NULL)
ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []NotOnTracker{}
	for rows.Next() {
		var n NotOnTracker
		if err := rows.Scan(&n.Hash, &n.Name, &n.Tracker); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}
