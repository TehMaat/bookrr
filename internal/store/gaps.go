package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// GapGroup is the result of a check for what is missing from one
// collection of the user's uploads (a series, later a saga), stored as
// JSON: the store does not need to know its shape.
type GapGroup struct {
	Key       string
	Title     string
	Missing   int
	Data      json.RawMessage
	CheckedAt string
}

func gapsCheckedKey(kind string) string { return "gaps_" + kind + "_checked_at" }

// SaveGapGroups replaces every result of this kind with those of a
// complete check, and records when it ended.
func (s *Store) SaveGapGroups(ctx context.Context, kind string, groups []GapGroup) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	ts := now()
	if _, err := tx.ExecContext(ctx, `DELETE FROM gap_groups WHERE kind = ?`, kind); err != nil {
		return err
	}
	for _, g := range groups {
		at := g.CheckedAt
		if at == "" {
			at = ts
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO gap_groups (kind, key, title, missing, data, checked_at) VALUES (?, ?, ?, ?, ?, ?)`,
			kind, g.Key, g.Title, g.Missing, string(g.Data), at); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT (key) DO UPDATE SET value = excluded.value`,
		gapsCheckedKey(kind), ts); err != nil {
		return err
	}
	return tx.Commit()
}

// GapGroups returns the results of this kind by title.
func (s *Store) GapGroups(ctx context.Context, kind string) ([]GapGroup, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key, title, missing, data, checked_at FROM gap_groups WHERE kind = ? ORDER BY title COLLATE NOCASE, key`, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GapGroup{}
	for rows.Next() {
		var g GapGroup
		var data string
		if err := rows.Scan(&g.Key, &g.Title, &g.Missing, &data, &g.CheckedAt); err != nil {
			return nil, err
		}
		g.Data = json.RawMessage(data)
		out = append(out, g)
	}
	return out, rows.Err()
}

// GapsCheckedAt is when the last complete check of this kind ended.
func (s *Store) GapsCheckedAt(ctx context.Context, kind string) (*string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, gapsCheckedKey(kind)).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) || v == "" {
		return nil, nil
	}
	return &v, err
}

// ActiveTrackerUploads returns the user's uploads still on the tracker.
func (s *Store) ActiveTrackerUploads(ctx context.Context) ([]TrackerUpload, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, category, type, type_id, resolution, resolution_id, size, seeders, leechers, times_completed,
	tmdb_id, imdb_id, tvdb_id, personal_release, created_at, details_link
FROM tracker_releases WHERE removed_at IS NULL ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TrackerUpload{}
	for rows.Next() {
		var u TrackerUpload
		if err := rows.Scan(&u.ID, &u.Name, &u.Category, &u.Type, &u.TypeID, &u.Resolution, &u.ResolutionID, &u.Size,
			&u.Seeders, &u.Leechers, &u.TimesCompleted, &u.TmdbID, &u.ImdbID, &u.TvdbID, &u.PersonalRelease,
			&u.CreatedAt, &u.DetailsLink); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
