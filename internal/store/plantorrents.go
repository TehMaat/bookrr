package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// MaxPlanTorrents is how many .torrent files a film of the plan takes: two
// when the release is a merge of two sources.
const MaxPlanTorrents = 2

// MaxPlanTorrentLabel is the longest label of a .torrent of the plan.
const MaxPlanTorrentLabel = 100

var (
	ErrPlanTorrentsFull = errors.New("the film has two .torrent files already: replace or remove one")
	ErrPlanTorrentDup   = errors.New("this .torrent is already loaded on the film")
	// The file was dropped once the film was released: it is on Wayfinderr.
	ErrPlanTorrentDropped = errors.New("the .torrent file is on Wayfinderr only")
)

// PlanTorrent is a .torrent file loaded on a film of the plan (see the
// plan_torrents table), with where it is on the clients now.
type PlanTorrent struct {
	PlanID   int64 `json:"-"`
	Position int   `json:"position"`
	// v1 info hash, lowercase hex.
	Hash  string `json:"hash"`
	Name  string `json:"name"`
	Size  int64  `json:"size"`
	Label string `json:"label"`
	// The client it was last sent to, 0 and "" if never.
	SentClientID   int64   `json:"sentClientId"`
	SentClientName string  `json:"sentClientName"`
	SentAt         *string `json:"sentAt"`
	CreatedAt      string  `json:"createdAt"`
	// Where it is on the clients, as their last read found it.
	Downloads []PlanDownload `json:"downloads"`
	// Sent, but the client read after the sending does not have it: it was
	// removed (or never added).
	Gone bool `json:"gone"`
	// Sent to the Wayfinderr of this client as the source .torrent of the
	// film (0 and nil if not yet); WayfinderrError is why the last try failed.
	WayfinderrClientID   int64   `json:"wayfinderrClientId"`
	WayfinderrClientName string  `json:"wayfinderrClientName"`
	WayfinderrAt         *string `json:"wayfinderrAt"`
	WayfinderrError      string  `json:"wayfinderrError"`
	// The film was released: bookrr dropped the file, it is on Wayfinderr.
	Dropped bool `json:"dropped"`
}

// Complete reports whether the torrent is downloaded whole on a client.
func (t PlanTorrent) Complete() bool {
	for _, d := range t.Downloads {
		if d.Progress >= 1 {
			return true
		}
	}
	return false
}

// PlanDownload is a .torrent of the plan on a client.
type PlanDownload struct {
	ClientID   int64   `json:"clientId"`
	ClientName string  `json:"clientName"`
	State      string  `json:"state"`
	Progress   float64 `json:"progress"`
	SeenAt     string  `json:"seenAt"`
}

// PlanTorrentInput is a .torrent file read by the caller (see unit3d.ReadTorrent).
type PlanTorrentInput struct {
	Hash  string
	Name  string
	Size  int64
	Label string
	Data  []byte
}

// PlanTorrents returns the .torrent files of the films of the plan, by film.
func (s *Store) PlanTorrents(ctx context.Context) (map[int64][]PlanTorrent, error) {
	return s.planTorrents(ctx, 0)
}

// PlanTorrentsOf returns the .torrent files of a film of the plan, by position.
func (s *Store) PlanTorrentsOf(ctx context.Context, planID int64) ([]PlanTorrent, error) {
	byPlan, err := s.planTorrents(ctx, planID)
	if err != nil {
		return nil, err
	}
	if byPlan[planID] == nil {
		return []PlanTorrent{}, nil
	}
	return byPlan[planID], nil
}

// planTorrents reads the torrents of the film planID, or of every film with 0.
func (s *Store) planTorrents(ctx context.Context, planID int64) (map[int64][]PlanTorrent, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT pt.plan_id, pt.position, pt.hash, pt.name, pt.size, pt.label, pt.sent_client_id, COALESCE(c.name, ''),
	pt.sent_at, pt.created_at,
	COALESCE(pt.sent_at IS NOT NULL AND c.enabled = 1 AND c.last_sync_at > pt.sent_at
		AND NOT EXISTS (SELECT 1 FROM locations l WHERE l.hash = pt.hash AND l.client_id = pt.sent_client_id), 0),
	pt.wayfinderr_client_id, COALESCE(wc.name, ''), pt.wayfinderr_at, pt.wayfinderr_error, pt.dropped_at IS NOT NULL
FROM plan_torrents pt LEFT JOIN clients c ON c.id = pt.sent_client_id
	LEFT JOIN clients wc ON wc.id = pt.wayfinderr_client_id
WHERE ? = 0 OR pt.plan_id = ?
ORDER BY pt.plan_id, pt.position`, planID, planID)
	if err != nil {
		return nil, err
	}
	out := map[int64][]PlanTorrent{}
	hashes := map[string]bool{}
	for rows.Next() {
		var t PlanTorrent
		var sent, wayfinderr sql.NullString
		if err := rows.Scan(&t.PlanID, &t.Position, &t.Hash, &t.Name, &t.Size, &t.Label, &t.SentClientID, &t.SentClientName,
			&sent, &t.CreatedAt, &t.Gone, &t.WayfinderrClientID, &t.WayfinderrClientName, &wayfinderr, &t.WayfinderrError,
			&t.Dropped); err != nil {
			rows.Close()
			return nil, err
		}
		t.SentAt, t.WayfinderrAt = nullStr(sent), nullStr(wayfinderr)
		t.Downloads = []PlanDownload{}
		out[t.PlanID] = append(out[t.PlanID], t)
		hashes[t.Hash] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil || len(hashes) == 0 {
		return out, err
	}

	// Where they are on the clients.
	downloads := map[string][]PlanDownload{}
	lrows, err := s.db.QueryContext(ctx, `
SELECT l.hash, l.client_id, c.name, l.state, l.progress, l.seen_at
FROM locations l JOIN clients c ON c.id = l.client_id
WHERE l.hash IN (SELECT hash FROM plan_torrents WHERE ? = 0 OR plan_id = ?)
ORDER BY c.name COLLATE NOCASE`, planID, planID)
	if err != nil {
		return nil, err
	}
	defer lrows.Close()
	for lrows.Next() {
		var h string
		var d PlanDownload
		if err := lrows.Scan(&h, &d.ClientID, &d.ClientName, &d.State, &d.Progress, &d.SeenAt); err != nil {
			return nil, err
		}
		downloads[h] = append(downloads[h], d)
	}
	if err := lrows.Err(); err != nil {
		return nil, err
	}
	for id, ts := range out {
		for i := range ts {
			if d := downloads[ts[i].Hash]; d != nil {
				ts[i].Downloads = d
			}
		}
		out[id] = ts
	}
	return out, nil
}

// PlanTorrent returns a .torrent of a film of the plan.
func (s *Store) PlanTorrent(ctx context.Context, planID int64, position int) (PlanTorrent, error) {
	ts, err := s.PlanTorrentsOf(ctx, planID)
	if err != nil {
		return PlanTorrent{}, err
	}
	for _, t := range ts {
		if t.Position == position {
			return t, nil
		}
	}
	return PlanTorrent{}, ErrNotFound
}

// PlanTorrentFile returns the .torrent file of a film of the plan;
// ErrPlanTorrentDropped once it is on Wayfinderr only.
func (s *Store) PlanTorrentFile(ctx context.Context, planID int64, position int) ([]byte, error) {
	var data []byte
	var dropped bool
	err := s.db.QueryRowContext(ctx, `SELECT data, dropped_at IS NOT NULL FROM plan_torrents WHERE plan_id = ? AND position = ?`,
		planID, position).Scan(&data, &dropped)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err == nil && (dropped || len(data) == 0) {
		return nil, ErrPlanTorrentDropped
	}
	return data, err
}

// SetPlanTorrent loads a .torrent file on a film of the plan: at position
// (1 or 2) replacing the one there, or with 0 at the first free one. A
// film downloaded already goes back in progress, and is made downloaded
// again by itself when the new torrent is complete too.
func (s *Store) SetPlanTorrent(ctx context.Context, planID int64, position int, in PlanTorrentInput) (PlanTorrent, error) {
	in.Label = strings.TrimSpace(in.Label)
	if len(in.Label) > MaxPlanTorrentLabel {
		return PlanTorrent{}, errors.New("label too long")
	}
	if in.Hash == "" || len(in.Data) == 0 {
		return PlanTorrent{}, errors.New("empty .torrent file")
	}
	if position < 0 || position > MaxPlanTorrents {
		return PlanTorrent{}, errors.New("invalid position")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PlanTorrent{}, err
	}
	defer tx.Rollback()
	var status string
	err = tx.QueryRowContext(ctx, `SELECT status FROM release_plan WHERE id = ?`, planID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return PlanTorrent{}, ErrNotFound
	}
	if err != nil {
		return PlanTorrent{}, err
	}
	used := map[int]string{}
	rows, err := tx.QueryContext(ctx, `SELECT position, hash FROM plan_torrents WHERE plan_id = ?`, planID)
	if err != nil {
		return PlanTorrent{}, err
	}
	for rows.Next() {
		var p int
		var h string
		if err := rows.Scan(&p, &h); err != nil {
			rows.Close()
			return PlanTorrent{}, err
		}
		used[p] = h
	}
	rows.Close()
	if position == 0 {
		for p := 1; p <= MaxPlanTorrents; p++ {
			if _, ok := used[p]; !ok {
				position = p
				break
			}
		}
		if position == 0 {
			return PlanTorrent{}, ErrPlanTorrentsFull
		}
	}
	for p, h := range used {
		if p != position && h == in.Hash {
			return PlanTorrent{}, ErrPlanTorrentDup
		}
	}
	if used[position] == in.Hash {
		// The same file again: only the label changes (and the file is back
		// if it was dropped).
		if _, err := tx.ExecContext(ctx, `UPDATE plan_torrents SET label = ?, data = ?, dropped_at = NULL
WHERE plan_id = ? AND position = ?`, in.Label, in.Data, planID, position); err != nil {
			return PlanTorrent{}, err
		}
	} else {
		if _, err := tx.ExecContext(ctx, `
INSERT OR REPLACE INTO plan_torrents (plan_id, position, hash, name, size, label, created_at, data)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, planID, position, in.Hash, in.Name, in.Size, in.Label, now(), in.Data); err != nil {
			return PlanTorrent{}, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE release_plan SET downloaded_at = NULL,
	status = CASE WHEN status = ? THEN ? ELSE status END WHERE id = ?`, PlanDownloaded, PlanDoing, planID); err != nil {
			return PlanTorrent{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return PlanTorrent{}, err
	}
	return s.PlanTorrent(ctx, planID, position)
}

// SetPlanTorrentLabel changes the label of a .torrent of the plan.
func (s *Store) SetPlanTorrentLabel(ctx context.Context, planID int64, position int, label string) (PlanTorrent, error) {
	label = strings.TrimSpace(label)
	if len(label) > MaxPlanTorrentLabel {
		return PlanTorrent{}, errors.New("label too long")
	}
	res, err := s.db.ExecContext(ctx, `UPDATE plan_torrents SET label = ? WHERE plan_id = ? AND position = ?`, label, planID, position)
	if err != nil {
		return PlanTorrent{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return PlanTorrent{}, ErrNotFound
	}
	return s.PlanTorrent(ctx, planID, position)
}

// DeletePlanTorrent removes a .torrent from a film of the plan (not from
// the clients).
func (s *Store) DeletePlanTorrent(ctx context.Context, planID int64, position int) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM plan_torrents WHERE plan_id = ? AND position = ?`, planID, position)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// MarkPlanTorrentSent records that a .torrent of the plan was sent to a
// client: the film is in progress (if it was to do) and is made downloaded
// again by itself once complete.
func (s *Store) MarkPlanTorrentSent(ctx context.Context, planID int64, position int, clientID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE plan_torrents SET sent_client_id = ?, sent_at = ? WHERE plan_id = ? AND position = ?`,
		clientID, now(), planID, position)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `UPDATE release_plan SET downloaded_at = NULL,
	status = CASE WHEN status = ? THEN ? ELSE status END WHERE id = ?`, PlanTodo, PlanDoing, planID); err != nil {
		return err
	}
	return tx.Commit()
}

// PromoteDownloadedPlan makes downloaded the films to do or in progress
// whose .torrent files are all complete on a client, unless they were
// downloaded already since they were last sent (the user put them back).
// It returns how many.
func (s *Store) PromoteDownloadedPlan(ctx context.Context) (int, error) {
	res, err := s.db.ExecContext(ctx, `
UPDATE release_plan SET status = ?, downloaded_at = ?
WHERE status IN (?, ?) AND downloaded_at IS NULL
	AND EXISTS (SELECT 1 FROM plan_torrents pt WHERE pt.plan_id = release_plan.id)
	AND NOT EXISTS (SELECT 1 FROM plan_torrents pt WHERE pt.plan_id = release_plan.id
		AND NOT EXISTS (SELECT 1 FROM locations l WHERE l.hash = pt.hash AND l.progress >= 1))`,
		PlanDownloaded, now(), PlanTodo, PlanDoing)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	return int(n), err
}

// WayfinderrSend is a .torrent of the plan to send to the Wayfinderr of a
// client, as the source .torrent of its film.
type WayfinderrSend struct {
	PlanID   int64
	Position int
	Hash     string
	Name     string
	Label    string
	TmdbID   int64
	Data     []byte
	Client   Client
}

// PlanTorrentsForWayfinderr returns the .torrent files of the open films of
// the plan complete on a client with a Wayfinderr and not sent to one yet,
// each with that client: the one it was sent to when it is complete there,
// else the first by name.
func (s *Store) PlanTorrentsForWayfinderr(ctx context.Context) ([]WayfinderrSend, error) {
	// One row per client with a Wayfinderr it is complete on, the client it
	// was sent to first.
	rows, err := s.db.QueryContext(ctx, `
SELECT pt.plan_id, pt.position, pt.hash, pt.name, pt.label, rp.tmdb_id, c.id
FROM plan_torrents pt JOIN release_plan rp ON rp.id = pt.plan_id
	JOIN locations l ON l.hash = pt.hash AND l.progress >= 1
	JOIN clients c ON c.id = l.client_id AND c.enabled = 1 AND c.wayfinderr_url != '' AND c.wayfinderr_api_key != ''
WHERE pt.wayfinderr_at IS NULL AND pt.dropped_at IS NULL AND rp.status IN (?, ?, ?)
ORDER BY pt.plan_id, pt.position, c.id = pt.sent_client_id DESC, c.name COLLATE NOCASE`, PlanTodo, PlanDoing, PlanDownloaded)
	if err != nil {
		return nil, err
	}
	var out []WayfinderrSend
	for rows.Next() {
		var w WayfinderrSend
		if err := rows.Scan(&w.PlanID, &w.Position, &w.Hash, &w.Name, &w.Label, &w.TmdbID, &w.Client.ID); err != nil {
			rows.Close()
			return nil, err
		}
		if n := len(out); n > 0 && out[n-1].PlanID == w.PlanID && out[n-1].Position == w.Position {
			continue
		}
		out = append(out, w)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		c, err := s.GetClient(ctx, out[i].Client.ID)
		if err != nil {
			return nil, err
		}
		out[i].Client = c
		if out[i].Data, err = s.PlanTorrentFile(ctx, out[i].PlanID, out[i].Position); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// SetPlanTorrentWayfinderr records how sending a .torrent of the plan to the
// Wayfinderr of a client went: sent with a nil error, else why it failed.
func (s *Store) SetPlanTorrentWayfinderr(ctx context.Context, planID int64, position int, clientID int64, sendErr error) error {
	var res sql.Result
	var err error
	if sendErr != nil {
		res, err = s.db.ExecContext(ctx, `UPDATE plan_torrents SET wayfinderr_error = ? WHERE plan_id = ? AND position = ?`,
			sendErr.Error(), planID, position)
	} else {
		res, err = s.db.ExecContext(ctx, `UPDATE plan_torrents SET wayfinderr_client_id = ?, wayfinderr_at = ?, wayfinderr_error = ''
WHERE plan_id = ? AND position = ?`, clientID, now(), planID, position)
	}
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// WayfinderrDrop is a .torrent of the plan whose file can go: its film was
// released, and it is on the Wayfinderr of Client.
type WayfinderrDrop struct {
	PlanID   int64
	Position int
	Hash     string
	Name     string
	Client   Client
}

// PlanTorrentsToDrop returns the .torrent files still kept of the films the
// user released (made done by an upload found on the tracker, not by hand)
// that were sent to a Wayfinderr, with the client of that Wayfinderr.
func (s *Store) PlanTorrentsToDrop(ctx context.Context) ([]WayfinderrDrop, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT pt.plan_id, pt.position, pt.hash, pt.name, pt.wayfinderr_client_id
FROM plan_torrents pt JOIN release_plan rp ON rp.id = pt.plan_id
WHERE rp.status = ? AND rp.release_id != 0 AND pt.wayfinderr_at IS NOT NULL AND pt.dropped_at IS NULL
ORDER BY pt.plan_id, pt.position`, PlanDone)
	if err != nil {
		return nil, err
	}
	var out []WayfinderrDrop
	for rows.Next() {
		var d WayfinderrDrop
		if err := rows.Scan(&d.PlanID, &d.Position, &d.Hash, &d.Name, &d.Client.ID); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, d)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	kept := out[:0]
	for _, d := range out {
		c, err := s.GetClient(ctx, d.Client.ID)
		if errors.Is(err, ErrNotFound) {
			// The client is gone, and with it the way to its Wayfinderr: kept.
			continue
		}
		if err != nil {
			return nil, err
		}
		d.Client = c
		kept = append(kept, d)
	}
	return kept, nil
}

// DropPlanTorrentFile drops the file of a .torrent of the plan, which is on
// Wayfinderr: the row stays, with its name, hash and label.
func (s *Store) DropPlanTorrentFile(ctx context.Context, planID int64, position int) error {
	res, err := s.db.ExecContext(ctx, `UPDATE plan_torrents SET data = X'', dropped_at = ? WHERE plan_id = ? AND position = ?`,
		now(), planID, position)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// WayfinderrOf returns the client whose Wayfinderr has a .torrent of the
// plan (ErrNotFound if it was never sent, or the client is gone).
func (s *Store) WayfinderrOf(ctx context.Context, planID int64, position int) (Client, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT wayfinderr_client_id FROM plan_torrents WHERE plan_id = ? AND position = ?
	AND wayfinderr_at IS NOT NULL`, planID, position).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return Client{}, ErrNotFound
	}
	if err != nil {
		return Client{}, err
	}
	return s.GetClient(ctx, id)
}
