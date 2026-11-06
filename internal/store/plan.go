package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// Statuses of a film of the release plan.
const (
	PlanTodo    = "todo"
	PlanDoing   = "doing"
	PlanDone    = "done"
	PlanDropped = "dropped"
)

// ValidPlanStatus reports whether s is a status of the plan.
func ValidPlanStatus(s string) bool {
	switch s {
	case PlanTodo, PlanDoing, PlanDone, PlanDropped:
		return true
	}
	return false
}

// PlanEntry is a film the user plans to release (see the release_plan table).
type PlanEntry struct {
	ID            int64  `json:"id"`
	TmdbID        int64  `json:"tmdbId"`
	ImdbID        int64  `json:"imdbId"`
	Title         string `json:"title"`
	OriginalTitle string `json:"originalTitle"`
	// "1999-03-31", "" when unknown.
	ReleaseDate string `json:"releaseDate"`
	PosterPath  string `json:"posterPath"`
	Status      string `json:"status"`
	// Where it was added from: "" by hand, "saga:collection:119",
	// "list:mdblist:user/slug"; SourceTitle is the name of the collection.
	Source      string `json:"source"`
	SourceTitle string `json:"sourceTitle"`
	Note        string `json:"note"`
	// The upload of the user that completed it, 0 if none.
	ReleaseID int64   `json:"releaseId"`
	CreatedAt string  `json:"createdAt"`
	DoneAt    *string `json:"doneAt"`
}

// Open reports whether the film is still to be released (todo or doing).
func (e PlanEntry) Open() bool { return e.Status == PlanTodo || e.Status == PlanDoing }

// Released reports whether the film was out on day (YYYY-MM-DD).
func (e PlanEntry) Released(day string) bool { return e.ReleaseDate != "" && e.ReleaseDate <= day }

// Year of release, "" if unknown.
func (e PlanEntry) Year() string {
	if len(e.ReleaseDate) >= 4 {
		return e.ReleaseDate[:4]
	}
	return ""
}

const planColumns = `id, tmdb_id, imdb_id, title, original_title, release_date, poster_path, status, source, source_title,
	note, release_id, created_at, done_at`

func scanPlan(row interface{ Scan(...any) error }) (PlanEntry, error) {
	var e PlanEntry
	var done sql.NullString
	err := row.Scan(&e.ID, &e.TmdbID, &e.ImdbID, &e.Title, &e.OriginalTitle, &e.ReleaseDate, &e.PosterPath, &e.Status,
		&e.Source, &e.SourceTitle, &e.Note, &e.ReleaseID, &e.CreatedAt, &done)
	if done.Valid {
		e.DoneAt = &done.String
	}
	return e, err
}

// PlanEntries returns the films of the plan, the last added first.
func (s *Store) PlanEntries(ctx context.Context) ([]PlanEntry, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+planColumns+` FROM release_plan ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PlanEntry{}
	for rows.Next() {
		e, err := scanPlan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// PlanEntry returns a film of the plan.
func (s *Store) PlanEntry(ctx context.Context, id int64) (PlanEntry, error) {
	e, err := scanPlan(s.db.QueryRowContext(ctx, `SELECT `+planColumns+` FROM release_plan WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return e, ErrNotFound
	}
	return e, err
}

// PlanEntryByTmdb returns the film of the plan with this TMDB id.
func (s *Store) PlanEntryByTmdb(ctx context.Context, tmdbID int64) (PlanEntry, error) {
	e, err := scanPlan(s.db.QueryRowContext(ctx, `SELECT `+planColumns+` FROM release_plan WHERE tmdb_id = ?`, tmdbID))
	if errors.Is(err, sql.ErrNoRows) {
		return e, ErrNotFound
	}
	return e, err
}

// AddPlanEntry adds a film to the plan, to do; ErrExists if it is there already.
func (s *Store) AddPlanEntry(ctx context.Context, e PlanEntry) (PlanEntry, error) {
	if e.TmdbID <= 0 {
		return e, errors.New("film senza id TMDB")
	}
	res, err := s.db.ExecContext(ctx, `
INSERT INTO release_plan (tmdb_id, imdb_id, title, original_title, release_date, poster_path, status, source, source_title, note, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.TmdbID, e.ImdbID, e.Title, e.OriginalTitle, e.ReleaseDate, e.PosterPath, PlanTodo, e.Source, e.SourceTitle,
		strings.TrimSpace(e.Note), now())
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return e, ErrExists
		}
		return e, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return e, err
	}
	return s.PlanEntry(ctx, id)
}

// PlanUpdate changes a film of the plan: the fields that are not nil.
type PlanUpdate struct {
	Status *string `json:"status"`
	Note   *string `json:"note"`
}

// UpdatePlanEntry changes status and note of a film of the plan. Done by
// hand, it is linked to no upload; back to an open status, it forgets
// the upload that completed it.
func (s *Store) UpdatePlanEntry(ctx context.Context, id int64, u PlanUpdate) (PlanEntry, error) {
	e, err := s.PlanEntry(ctx, id)
	if err != nil {
		return e, err
	}
	if u.Note != nil {
		e.Note = strings.TrimSpace(*u.Note)
	}
	if u.Status != nil && *u.Status != e.Status {
		if !ValidPlanStatus(*u.Status) {
			return e, errors.New("stato non valido")
		}
		e.Status = *u.Status
		e.DoneAt, e.ReleaseID = nil, 0
		if e.Status == PlanDone {
			ts := now()
			e.DoneAt = &ts
		}
	}
	_, err = s.db.ExecContext(ctx, `UPDATE release_plan SET status = ?, note = ?, done_at = ?, release_id = ? WHERE id = ?`,
		e.Status, e.Note, e.DoneAt, e.ReleaseID, id)
	if err != nil {
		return e, err
	}
	return s.PlanEntry(ctx, id)
}

// CompletePlanEntry marks an open film of the plan done by an upload of the user.
func (s *Store) CompletePlanEntry(ctx context.Context, id, releaseID int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE release_plan SET status = ?, release_id = ?, done_at = ? WHERE id = ? AND status IN (?, ?)`,
		PlanDone, releaseID, now(), id, PlanTodo, PlanDoing)
	return err
}

// DeletePlanEntry removes a film from the plan.
func (s *Store) DeletePlanEntry(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM release_plan WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
