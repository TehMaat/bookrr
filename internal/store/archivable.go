package store

import (
	"context"
	"database/sql"
	"time"
)

// An upload of the user can be taken off the clients and archived when
// others seed it: the tracker's seeders include the user, once for every
// client seeding it, so the others are the seeders minus the clients of
// bookrr that hold it complete and are seeding it.

// seedingStates are the qBittorrent states in which a complete torrent is
// announced to the tracker as a seeder (stalled too: it just has no peers
// right now). Paused, stopped, queued or checking torrents are not.
var seedingStates = map[string]bool{"uploading": true, "stalledUP": true, "forcedUP": true}

// Seeding reports whether a client counts among the seeders of a torrent.
func Seeding(state string, progress float64) bool {
	return progress >= 1 && seedingStates[state]
}

// ArchiveRule is when an upload is archivable: at least MinOthers other
// seeders and MinCompleted downloads, for at least Days days in a row.
type ArchiveRule struct {
	MinOthers    int64 `json:"minOthers"`
	MinCompleted int64 `json:"minCompleted"`
	Days         int64 `json:"days"`
}

func (st Settings) ArchiveRule() ArchiveRule {
	return ArchiveRule{MinOthers: st.ArchiveMinSeeders, MinCompleted: st.ArchiveMinCompleted, Days: st.ArchiveDays}
}

// Archivability is how an upload still on a client stands against the
// archive rule, with the numbers it is based on.
type Archivability struct {
	ReleaseID int64 `json:"releaseId"`
	// Seeders on the tracker, the user included.
	Seeders int64 `json:"seeders"`
	// Clients of bookrr seeding it complete.
	Mine           int64 `json:"mine"`
	Others         int64 `json:"others"`
	TimesCompleted int64 `json:"timesCompleted"`
	// Last time the tracker gave these counters.
	TrackerAt string `json:"trackerAt"`
	// The thresholds are met now.
	Met bool `json:"met"`
	// Since when they are met without a break; nil when they are not.
	Since *string `json:"since"`
	// Whole days since Since.
	Days int64 `json:"days"`
	// Met for at least the days of the rule.
	Archivable bool `json:"archivable"`
	// When it becomes archivable if the thresholds stay met.
	ReadyAt *string     `json:"readyAt"`
	Rule    ArchiveRule `json:"rule"`
	// The user keeps it on the clients: never archivable.
	NeverArchive bool `json:"neverArchive"`
}

// releaseCounters is what the tracker says of an upload.
type releaseCounters struct {
	id        int64
	hash      string
	seeders   int64
	leechers  int64
	completed int64
	seenAt    string
	removed   bool
	since     *string // stored archivable_since
}

// seedLoc is a copy of a torrent on a client.
type seedLoc struct {
	state    string
	progress float64
}

const day = 24 * time.Hour

// assessArchivable applies the rule to an upload. Uploads no longer on the
// tracker or on no client (already archived or adopted) are not assessed:
// it returns nil. since is when the thresholds were first seen met; when
// they are met but since is unknown, they are met from now.
func assessArchivable(r releaseCounters, locs []seedLoc, rule ArchiveRule, now time.Time) *Archivability {
	if r.removed || len(locs) == 0 {
		return nil
	}
	a := &Archivability{ReleaseID: r.id, Seeders: r.seeders, TimesCompleted: r.completed, TrackerAt: r.seenAt, Rule: rule}
	for _, l := range locs {
		if Seeding(l.state, l.progress) {
			a.Mine++
		}
	}
	a.Others = max(r.seeders-a.Mine, 0)
	a.Met = a.Others >= rule.MinOthers && a.TimesCompleted >= rule.MinCompleted
	if !a.Met {
		return a
	}
	since := now
	if r.since != nil {
		if t, err := time.Parse(time.RFC3339, *r.since); err == nil && !t.After(now) {
			since = t
		}
	}
	s := since.UTC().Format(time.RFC3339)
	a.Since = &s
	a.Days = int64(now.Sub(since) / day)
	ready := since.Add(time.Duration(rule.Days) * day)
	if a.Archivable = !now.Before(ready); !a.Archivable {
		r := ready.UTC().Format(time.RFC3339)
		a.ReadyAt = &r
	}
	return a
}

// neverArchive applies the user's choice to keep an upload on the clients
// (see SetNeverArchive): it is never archivable, while the days the rule is
// met are still counted, so it is right again if the choice is undone.
func (a *Archivability) neverArchive(never bool) *Archivability {
	if a != nil && never {
		a.NeverArchive = true
		a.Archivable = false
		a.ReadyAt = nil
	}
	return a
}

// SetNeverArchive marks torrents as never to archive, or lets them be
// archivable again. It returns how many torrents it found.
func (s *Store) SetNeverArchive(ctx context.Context, hashes []string, never bool) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	n := 0
	for _, h := range hashes {
		res, err := tx.ExecContext(ctx, `UPDATE torrents SET never_archive = ? WHERE hash = ?`, boolInt(never), h)
		if err != nil {
			return 0, err
		}
		if c, _ := res.RowsAffected(); c > 0 {
			n++
		}
	}
	return n, tx.Commit()
}

// Peers is what the tracker counts of an upload.
type Peers struct {
	ReleaseID      int64 `json:"releaseId"`
	Seeders        int64 `json:"seeders"`
	Leechers       int64 `json:"leechers"`
	TimesCompleted int64 `json:"timesCompleted"`
	// Last time the tracker gave these counters.
	TrackerAt string `json:"trackerAt"`
}

func (r releaseCounters) peers() *Peers {
	return &Peers{ReleaseID: r.id, Seeders: r.seeders, Leechers: r.leechers, TimesCompleted: r.completed, TrackerAt: r.seenAt}
}

const releaseCounterCols = `id, hash, seeders, leechers, times_completed, seen_at, removed_at IS NOT NULL, archivable_since`

func scanCounters(rows *sql.Rows) (releaseCounters, error) {
	var r releaseCounters
	var since sql.NullString
	err := rows.Scan(&r.id, &r.hash, &r.seeders, &r.leechers, &r.completed, &r.seenAt, &r.removed, &since)
	r.since = nullStr(since)
	return r, err
}

// liveReleases returns the uploads still on the tracker by hash (the most
// recent one if the same file was uploaded twice); hash "" = all of them.
func (s *Store) liveReleases(ctx context.Context, hash string) (map[string]releaseCounters, error) {
	where, args := "", []any{}
	if hash != "" {
		where, args = " AND hash = ?", []any{hash}
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+releaseCounterCols+` FROM tracker_releases
WHERE hash != '' AND removed_at IS NULL`+where+` ORDER BY id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]releaseCounters{}
	for rows.Next() {
		r, err := scanCounters(rows)
		if err != nil {
			return nil, err
		}
		out[r.hash] = r
	}
	return out, rows.Err()
}

// seedLocations returns the copies on the clients of every torrent.
func (s *Store) seedLocations(ctx context.Context) (map[string][]seedLoc, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT hash, state, progress FROM locations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]seedLoc{}
	for rows.Next() {
		var h string
		var l seedLoc
		if err := rows.Scan(&h, &l.state, &l.progress); err != nil {
			return nil, err
		}
		out[h] = append(out[h], l)
	}
	return out, rows.Err()
}

func (s *Store) archiveRule(ctx context.Context) (ArchiveRule, error) {
	st, err := s.GetSettings(ctx)
	return st.ArchiveRule(), err
}

// RefreshArchivable records since when each upload meets the archive rule
// and forgets it as soon as it does not. It runs whenever the counters of
// the tracker, the clients or the rule change.
func (s *Store) RefreshArchivable(ctx context.Context) error {
	rule, err := s.archiveRule(ctx)
	if err != nil {
		return err
	}
	locs, err := s.seedLocations(ctx)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT `+releaseCounterCols+` FROM tracker_releases`)
	if err != nil {
		return err
	}
	type change struct {
		id    int64
		since *string
	}
	var changes []change
	now := s.clock()
	for rows.Next() {
		r, err := scanCounters(rows)
		if err != nil {
			rows.Close()
			return err
		}
		var want *string
		if r.hash != "" {
			if a := assessArchivable(r, locs[r.hash], rule, now); a != nil && a.Met {
				want = a.Since
			}
		}
		if !sameStr(want, r.since) {
			changes = append(changes, change{r.id, want})
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, c := range changes {
		if _, err := tx.ExecContext(ctx, `UPDATE tracker_releases SET archivable_since = ? WHERE id = ?`, c.since, c.id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func sameStr(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
