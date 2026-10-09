package store

import (
	"context"
	"database/sql"
	"errors"
)

type Disk struct {
	ID           int64  `json:"id"`
	Label        string `json:"label"`
	Kind         string `json:"kind"`
	Serial       string `json:"serial"`
	Model        string `json:"model"`
	Capacity     int64  `json:"capacity"`
	Place        string `json:"place"`
	Notes        string `json:"notes"`
	CreatedAt    string `json:"createdAt"`
	ArchiveCount int    `json:"archiveCount"`
	ArchivedSize int64  `json:"archivedSize"`
}

const diskSelect = `
SELECT d.id, d.label, d.kind, d.serial, d.model, d.capacity, d.place, d.notes, d.created_at,
	COUNT(a.id), COALESCE(SUM(t.size), 0)
FROM disks d
LEFT JOIN archives a ON a.disk_id = d.id
LEFT JOIN torrents t ON t.hash = a.hash`

func scanDisk(row interface{ Scan(...any) error }) (Disk, error) {
	var d Disk
	err := row.Scan(&d.ID, &d.Label, &d.Kind, &d.Serial, &d.Model, &d.Capacity, &d.Place, &d.Notes, &d.CreatedAt, &d.ArchiveCount, &d.ArchivedSize)
	return d, err
}

func (s *Store) ListDisks(ctx context.Context) ([]Disk, error) {
	rows, err := s.db.QueryContext(ctx, diskSelect+` GROUP BY d.id ORDER BY d.label COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Disk{}
	for rows.Next() {
		d, err := scanDisk(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) GetDisk(ctx context.Context, id int64) (Disk, error) {
	d, err := scanDisk(s.db.QueryRowContext(ctx, diskSelect+` WHERE d.id = ? GROUP BY d.id`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return d, ErrNotFound
	}
	return d, err
}

func (s *Store) CreateDisk(ctx context.Context, d Disk) (Disk, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO disks (label, kind, serial, model, capacity, place, notes, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		d.Label, d.Kind, d.Serial, d.Model, d.Capacity, d.Place, d.Notes, now())
	if err != nil {
		return d, err
	}
	id, _ := res.LastInsertId()
	return s.GetDisk(ctx, id)
}

func (s *Store) UpdateDisk(ctx context.Context, d Disk) (Disk, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE disks SET label = ?, kind = ?, serial = ?, model = ?, capacity = ?, place = ?, notes = ? WHERE id = ?`,
		d.Label, d.Kind, d.Serial, d.Model, d.Capacity, d.Place, d.Notes, d.ID)
	if err != nil {
		return d, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return d, ErrNotFound
	}
	return s.GetDisk(ctx, d.ID)
}

func (s *Store) DeleteDisk(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM disks WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
