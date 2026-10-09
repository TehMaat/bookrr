package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

type Adopter struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Contact      string `json:"contact"`
	Notes        string `json:"notes"`
	CreatedAt    string `json:"createdAt"`
	AdoptedCount int    `json:"adoptedCount"`
	AdoptedSize  int64  `json:"adoptedSize"`
}

// Adoption links a torrent to someone who adopted it.
type Adoption struct {
	ID          int64  `json:"id"`
	AdopterID   int64  `json:"adopterId"`
	AdopterName string `json:"adopterName"`
	Notes       string `json:"notes"`
	CreatedAt   string `json:"createdAt"`
	hash        string
}

var ErrAlreadyAdopted = errors.New("il torrent è già adottato da questo utente")

const adopterSelect = `
SELECT ad.id, ad.name, ad.contact, ad.notes, ad.created_at, COUNT(a.id), COALESCE(SUM(t.size), 0)
FROM adopters ad
LEFT JOIN adoptions a ON a.adopter_id = ad.id
LEFT JOIN torrents t ON t.hash = a.hash`

func scanAdopter(row interface{ Scan(...any) error }) (Adopter, error) {
	var a Adopter
	err := row.Scan(&a.ID, &a.Name, &a.Contact, &a.Notes, &a.CreatedAt, &a.AdoptedCount, &a.AdoptedSize)
	return a, err
}

func (s *Store) ListAdopters(ctx context.Context) ([]Adopter, error) {
	rows, err := s.db.QueryContext(ctx, adopterSelect+` GROUP BY ad.id ORDER BY ad.name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Adopter{}
	for rows.Next() {
		a, err := scanAdopter(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) GetAdopter(ctx context.Context, id int64) (Adopter, error) {
	a, err := scanAdopter(s.db.QueryRowContext(ctx, adopterSelect+` WHERE ad.id = ? GROUP BY ad.id`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}

func (s *Store) CreateAdopter(ctx context.Context, a Adopter) (Adopter, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO adopters (name, contact, notes, created_at) VALUES (?, ?, ?, ?)`,
		a.Name, a.Contact, a.Notes, now())
	if err != nil {
		return a, err
	}
	id, _ := res.LastInsertId()
	return s.GetAdopter(ctx, id)
}

func (s *Store) UpdateAdopter(ctx context.Context, a Adopter) (Adopter, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE adopters SET name = ?, contact = ?, notes = ? WHERE id = ?`,
		a.Name, a.Contact, a.Notes, a.ID)
	if err != nil {
		return a, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return a, ErrNotFound
	}
	return s.GetAdopter(ctx, a.ID)
}

// DeleteAdopter removes the adopter and every adoption that references it.
func (s *Store) DeleteAdopter(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM adopters WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) listAdoptions(ctx context.Context, hash string) ([]Adoption, error) {
	where, args := "", []any{}
	if hash != "" {
		where, args = " WHERE a.hash = ?", []any{hash}
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT a.id, a.hash, a.adopter_id, ad.name, a.notes, a.created_at
FROM adoptions a JOIN adopters ad ON ad.id = a.adopter_id`+where+` ORDER BY a.created_at, a.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Adoption{}
	for rows.Next() {
		var a Adoption
		if err := rows.Scan(&a.ID, &a.hash, &a.AdopterID, &a.AdopterName, &a.Notes, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

type AdoptionInput struct {
	AdopterID int64  `json:"adopterId"`
	Notes     string `json:"notes"`
}

// AddAdoption records that someone adopted the torrent. Like a move to a
// disk, it answers any open "where did it go?" alert.
func (s *Store) AddAdoption(ctx context.Context, hash string, in AdoptionInput) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := addAdoption(ctx, tx, hash, in); err != nil {
		return err
	}
	desc, err := describeAdoption(ctx, tx, in)
	if err != nil {
		return err
	}
	if err := resolveOpenAlerts(ctx, tx, hash, desc); err != nil {
		return err
	}
	return tx.Commit()
}

func addAdoption(ctx context.Context, db execer, hash string, in AdoptionInput) error {
	_, err := db.ExecContext(ctx, `INSERT INTO adoptions (hash, adopter_id, notes, created_at) VALUES (?, ?, ?, ?)`,
		hash, in.AdopterID, strings.TrimSpace(in.Notes), now())
	switch {
	case err == nil:
		return nil
	case strings.Contains(err.Error(), "UNIQUE"):
		return ErrAlreadyAdopted
	case strings.Contains(err.Error(), "FOREIGN KEY"):
		return ErrNotFound
	}
	return err
}

func describeAdoption(ctx context.Context, q txLike, in AdoptionInput) (string, error) {
	var name string
	if err := q.QueryRowContext(ctx, `SELECT name FROM adopters WHERE id = ?`, in.AdopterID).Scan(&name); err != nil {
		return "", err
	}
	return "Adottato da " + name, nil
}

func (s *Store) DeleteAdoption(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM adoptions WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
