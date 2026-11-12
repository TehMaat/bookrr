package store

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"
)

// Statuses of the pair of a merge film on Wayfinderr, besides its own
// (WAITING, PAIRED, ERROR).
const (
	// Wayfinderr no longer has the plan: not planned again by itself.
	MergeGone = "GONE"
	// The user removed the plan: not planned again by itself.
	MergeRemoved = "REMOVED"
)

// PlanMerge is the pair a merge film of the plan (two .torrent files) is
// planned as on the Wayfinderr of a client (see the plan_merges table).
type PlanMerge struct {
	PlanID int64 `json:"-"`
	// The two torrents planned, "a,b" (see MergeHashes).
	Hashes     string `json:"-"`
	ClientID   int64  `json:"clientId"`
	ClientName string `json:"clientName"`
	// The plan on Wayfinderr, "" if not planned yet (Error says why).
	RemoteID string `json:"planId"`
	// WAITING, PAIRED, ERROR as Wayfinderr says; GONE, REMOVED; "" not planned.
	Status string `json:"status"`
	// The pair Wayfinderr created, once PAIRED.
	PairID string `json:"pairId"`
	// What Wayfinderr says of an ERROR.
	Message string `json:"message"`
	// Why the last try of bookrr failed (planning, reading, removing).
	Error     string  `json:"error"`
	PlannedAt *string `json:"plannedAt"`
	CheckedAt *string `json:"checkedAt"`
	UpdatedAt string  `json:"updatedAt"`
}

// Planned reports whether the pair is on Wayfinderr and still open there
// (waiting for its files, or in error).
func (m PlanMerge) Planned() bool {
	return m.RemoteID != "" && (m.Status == "WAITING" || m.Status == "ERROR")
}

// MergeHashes is the pair of torrents of a merge film, "a,b" sorted; "" when
// the film does not have two.
func MergeHashes(ts []PlanTorrent) string {
	if len(ts) != MaxPlanTorrents {
		return ""
	}
	h := []string{ts[0].Hash, ts[1].Hash}
	sort.Strings(h)
	return strings.Join(h, ",")
}

const mergeColumns = `pm.plan_id, pm.hashes, pm.client_id, COALESCE(c.name, ''), pm.remote_id, pm.status, pm.pair_id, pm.message,
	pm.error, pm.planned_at, pm.checked_at, pm.updated_at`

func scanMerge(row interface{ Scan(...any) error }) (PlanMerge, error) {
	var m PlanMerge
	var planned, checked sql.NullString
	err := row.Scan(&m.PlanID, &m.Hashes, &m.ClientID, &m.ClientName, &m.RemoteID, &m.Status, &m.PairID, &m.Message,
		&m.Error, &planned, &checked, &m.UpdatedAt)
	m.PlannedAt, m.CheckedAt = nullStr(planned), nullStr(checked)
	return m, err
}

// PlanMerges returns the pairs of the merge films, by film.
func (s *Store) PlanMerges(ctx context.Context) (map[int64]PlanMerge, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+mergeColumns+` FROM plan_merges pm LEFT JOIN clients c ON c.id = pm.client_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]PlanMerge{}
	for rows.Next() {
		m, err := scanMerge(rows)
		if err != nil {
			return nil, err
		}
		out[m.PlanID] = m
	}
	return out, rows.Err()
}

// PlanMergeOf returns the pair of a merge film (ErrNotFound if none).
func (s *Store) PlanMergeOf(ctx context.Context, planID int64) (PlanMerge, error) {
	m, err := scanMerge(s.db.QueryRowContext(ctx, `SELECT `+mergeColumns+`
FROM plan_merges pm LEFT JOIN clients c ON c.id = pm.client_id WHERE pm.plan_id = ?`, planID))
	if errors.Is(err, sql.ErrNoRows) {
		return m, ErrNotFound
	}
	return m, err
}

// SavePlanMerge writes the pair of a merge film, replacing the one there.
func (s *Store) SavePlanMerge(ctx context.Context, m PlanMerge) (PlanMerge, error) {
	_, err := s.db.ExecContext(ctx, `
INSERT OR REPLACE INTO plan_merges (plan_id, hashes, client_id, remote_id, status, pair_id, message, error, planned_at,
	checked_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, m.PlanID, m.Hashes, m.ClientID, m.RemoteID, m.Status, m.PairID, m.Message, m.Error,
		m.PlannedAt, m.CheckedAt, now())
	if err != nil {
		return PlanMerge{}, err
	}
	return s.PlanMergeOf(ctx, m.PlanID)
}

// DeletePlanMerge forgets the pair of a merge film.
func (s *Store) DeletePlanMerge(ctx context.Context, planID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM plan_merges WHERE plan_id = ?`, planID)
	return err
}
