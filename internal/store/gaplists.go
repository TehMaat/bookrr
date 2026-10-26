package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// GapList is a collection the user follows beyond the sagas: a list of
// films on MDBList (see the gap_lists table).
type GapList struct {
	ID        int64  `json:"id"`
	Source    string `json:"source"`
	Path      string `json:"path"`
	Title     string `json:"title"`
	CreatedAt string `json:"createdAt"`
}

// Key is the key of the collection's results and choices (gap_groups,
// gap_marks): "mdblist:user/slug", the same if it is removed and added again.
func (l GapList) Key() string { return l.Source + ":" + l.Path }

// GapLists returns the collections by title.
func (s *Store) GapLists(ctx context.Context) ([]GapList, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, source, path, title, created_at FROM gap_lists ORDER BY title COLLATE NOCASE, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GapList{}
	for rows.Next() {
		var l GapList
		if err := rows.Scan(&l.ID, &l.Source, &l.Path, &l.Title, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// AddGapList adds a collection; ErrExists if it is there already.
func (s *Store) AddGapList(ctx context.Context, l GapList) (GapList, error) {
	l.CreatedAt = now()
	res, err := s.db.ExecContext(ctx, `INSERT INTO gap_lists (source, path, title, created_at) VALUES (?, ?, ?, ?)`,
		l.Source, l.Path, l.Title, l.CreatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return l, fmt.Errorf("collezione %w", ErrExists)
		}
		return l, err
	}
	l.ID, _ = res.LastInsertId()
	return l, nil
}

// DeleteGapList removes a collection with its results and the user's
// choices on it.
func (s *Store) DeleteGapList(ctx context.Context, id int64, kind string) (GapList, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return GapList{}, err
	}
	defer tx.Rollback()
	l := GapList{ID: id}
	err = tx.QueryRowContext(ctx, `SELECT source, path, title, created_at FROM gap_lists WHERE id = ?`, id).
		Scan(&l.Source, &l.Path, &l.Title, &l.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return l, ErrNotFound
	}
	if err != nil {
		return l, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM gap_lists WHERE id = ?`, id); err != nil {
		return l, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM gap_groups WHERE kind = ? AND key = ?`, kind, l.Key()); err != nil {
		return l, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM gap_marks WHERE kind = ? AND group_key = ?`, kind, l.Key()); err != nil {
		return l, err
	}
	return l, tx.Commit()
}
