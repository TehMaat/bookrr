package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

func (s *Store) ListAlerts(ctx context.Context, openOnly bool, hash string) ([]Alert, error) {
	conds, args := []string{}, []any{}
	if openOnly {
		conds = append(conds, "a.resolved_at IS NULL")
	}
	if hash != "" {
		conds = append(conds, "a.hash = ?")
		args = append(args, hash)
	}
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT a.id, a.hash, t.name, a.source, a.client_name, a.message, a.created_at, a.resolved_at, a.resolution
FROM alerts a JOIN torrents t ON t.hash = a.hash`+where+` ORDER BY a.created_at DESC, a.id DESC LIMIT 500`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Alert{}
	for rows.Next() {
		var a Alert
		var resolved sql.NullString
		if err := rows.Scan(&a.ID, &a.Hash, &a.TorrentName, &a.Source, &a.ClientName, &a.Message, &a.CreatedAt, &resolved, &a.Resolution); err != nil {
			return nil, err
		}
		a.ResolvedAt = nullStr(resolved)
		out = append(out, a)
	}
	return out, rows.Err()
}

type ResolveInput struct {
	// Resolution is a free-text description of where the torrent went.
	Resolution string        `json:"resolution"`
	Archive    *ArchiveInput `json:"archive"`
	AdoptedBy  *string       `json:"adoptedBy"`
}

// ResolveAlert records where a removed torrent went and closes every open
// alert for the same torrent.
func (s *Store) ResolveAlert(ctx context.Context, id int64, in ResolveInput) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var hash string
	err = tx.QueryRowContext(ctx, `SELECT hash FROM alerts WHERE id = ?`, id).Scan(&hash)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if in.Archive != nil {
		if err := addArchive(ctx, tx, hash, *in.Archive); err != nil {
			return err
		}
	}
	if in.AdoptedBy != nil {
		if _, err := tx.ExecContext(ctx, `UPDATE torrents SET adopted_by = ?, updated_at = ? WHERE hash = ?`,
			strings.TrimSpace(*in.AdoptedBy), now(), hash); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE alerts SET resolved_at = ?, resolution = ? WHERE hash = ? AND resolved_at IS NULL`,
		now(), in.Resolution, hash); err != nil {
		return err
	}
	return tx.Commit()
}

func openAlertExists(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, hash string) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM alerts WHERE hash = ? AND resolved_at IS NULL`, hash).Scan(&n)
	return n > 0, err
}
