package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
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

	// Filled from a SMART report (see SaveDiskSmart), never by the disk form.
	Firmware     string  `json:"firmware"`
	Health       string  `json:"health"`
	PowerOnHours int64   `json:"powerOnHours"`
	SmartAt      *string `json:"smartAt"`
}

const diskSelect = `
SELECT d.id, d.label, d.kind, d.serial, d.model, d.capacity, d.place, d.notes, d.created_at,
	COUNT(a.id), COALESCE(SUM(t.size), 0), d.firmware, d.health, d.power_on_hours, d.smart_at
FROM disks d
LEFT JOIN archives a ON a.disk_id = d.id
LEFT JOIN torrents t ON t.hash = a.hash`

func scanDisk(row interface{ Scan(...any) error }) (Disk, error) {
	var d Disk
	var smartAt sql.NullString
	err := row.Scan(&d.ID, &d.Label, &d.Kind, &d.Serial, &d.Model, &d.Capacity, &d.Place, &d.Notes, &d.CreatedAt, &d.ArchiveCount, &d.ArchivedSize,
		&d.Firmware, &d.Health, &d.PowerOnHours, &smartAt)
	d.SmartAt = nullStr(smartAt)
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

// FindDiskBySerial returns the disk with the given serial number, ignoring
// case and surrounding spaces.
func (s *Store) FindDiskBySerial(ctx context.Context, serial string) (Disk, error) {
	serial = strings.TrimSpace(serial)
	if serial == "" {
		return Disk{}, ErrNotFound
	}
	d, err := scanDisk(s.db.QueryRowContext(ctx, diskSelect+` WHERE trim(d.serial) = ? COLLATE NOCASE GROUP BY d.id ORDER BY d.id LIMIT 1`, serial))
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

// SaveDiskSmart creates (ID 0) or fully updates a disk, including the fields
// read from a SMART report, and marks the report as read now.
func (s *Store) SaveDiskSmart(ctx context.Context, d Disk) (Disk, error) {
	if d.ID == 0 {
		res, err := s.db.ExecContext(ctx,
			`INSERT INTO disks (label, kind, serial, model, capacity, place, notes, firmware, health, power_on_hours, smart_at, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			d.Label, d.Kind, d.Serial, d.Model, d.Capacity, d.Place, d.Notes, d.Firmware, d.Health, d.PowerOnHours, now(), now())
		if err != nil {
			return d, err
		}
		id, _ := res.LastInsertId()
		return s.GetDisk(ctx, id)
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE disks SET label = ?, kind = ?, serial = ?, model = ?, capacity = ?, place = ?, notes = ?,
			firmware = ?, health = ?, power_on_hours = ?, smart_at = ? WHERE id = ?`,
		d.Label, d.Kind, d.Serial, d.Model, d.Capacity, d.Place, d.Notes, d.Firmware, d.Health, d.PowerOnHours, now(), d.ID)
	if err != nil {
		return d, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return d, ErrNotFound
	}
	return s.GetDisk(ctx, d.ID)
}
