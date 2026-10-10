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

	// Set when the archive is an S3 bucket that bookrr reads.
	S3 *DiskS3 `json:"s3"`
}

// DiskS3 is the bucket behind a cloud archive and the result of its last scan.
type DiskS3 struct {
	Endpoint  string `json:"endpoint"`
	Region    string `json:"region"`
	Bucket    string `json:"bucket"`
	Prefix    string `json:"prefix"`
	AccessKey string `json:"accessKey"`
	SecretKey string `json:"-"`
	HasSecret bool   `json:"hasSecret"`

	ScanAt    *string `json:"scanAt"`
	ScanError string  `json:"scanError"`
	Objects   int     `json:"objects"`
	Size      int64   `json:"size"`
	Unmatched int     `json:"unmatched"`
}

// sameBucket reports whether a and b point to the same objects.
func sameBucket(a, b *DiskS3) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Endpoint == b.Endpoint && a.Bucket == b.Bucket && a.Prefix == b.Prefix
}

const diskSelect = `
SELECT d.id, d.label, d.kind, d.serial, d.model, d.capacity, d.place, d.notes, d.created_at,
	COUNT(a.id), COALESCE(SUM(t.size), 0), d.firmware, d.health, d.power_on_hours, d.smart_at,
	d.s3_endpoint, d.s3_region, d.s3_bucket, d.s3_prefix, d.s3_access_key, d.s3_secret_key,
	d.s3_scan_at, d.s3_error, d.s3_objects, d.s3_size,
	(SELECT COUNT(*) FROM bucket_entries e WHERE e.disk_id = d.id)
FROM disks d
LEFT JOIN archives a ON a.disk_id = d.id
LEFT JOIN torrents t ON t.hash = a.hash`

func scanDisk(row interface{ Scan(...any) error }) (Disk, error) {
	var d Disk
	var smartAt, scanAt sql.NullString
	var b DiskS3
	err := row.Scan(&d.ID, &d.Label, &d.Kind, &d.Serial, &d.Model, &d.Capacity, &d.Place, &d.Notes, &d.CreatedAt, &d.ArchiveCount, &d.ArchivedSize,
		&d.Firmware, &d.Health, &d.PowerOnHours, &smartAt,
		&b.Endpoint, &b.Region, &b.Bucket, &b.Prefix, &b.AccessKey, &b.SecretKey, &scanAt, &b.ScanError, &b.Objects, &b.Size, &b.Unmatched)
	d.SmartAt = nullStr(smartAt)
	if b.Bucket != "" {
		b.ScanAt = nullStr(scanAt)
		b.HasSecret = b.SecretKey != ""
		d.S3 = &b
	}
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

// s3Args returns the values of the s3_* configuration columns.
func s3Args(b *DiskS3) []any {
	if b == nil {
		b = &DiskS3{}
	}
	return []any{b.Endpoint, b.Region, b.Bucket, b.Prefix, b.AccessKey, b.SecretKey}
}

func (s *Store) CreateDisk(ctx context.Context, d Disk) (Disk, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO disks (label, kind, serial, model, capacity, place, notes, created_at,
			s3_endpoint, s3_region, s3_bucket, s3_prefix, s3_access_key, s3_secret_key)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		append([]any{d.Label, d.Kind, d.Serial, d.Model, d.Capacity, d.Place, d.Notes, now()}, s3Args(d.S3)...)...)
	if err != nil {
		return d, err
	}
	id, _ := res.LastInsertId()
	return s.GetDisk(ctx, id)
}

// UpdateDisk saves the disk and its bucket. Pointing the archive to other
// objects (another endpoint, bucket or prefix, or no bucket at all) forgets
// what was found in the old ones, without opening alerts: the next scan
// starts from scratch.
func (s *Store) UpdateDisk(ctx context.Context, d Disk) (Disk, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return d, err
	}
	defer tx.Rollback()
	old, err := scanDisk(tx.QueryRowContext(ctx, diskSelect+` WHERE d.id = ? GROUP BY d.id`, d.ID))
	if errors.Is(err, sql.ErrNoRows) {
		return d, ErrNotFound
	}
	if err != nil {
		return d, err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE disks SET label = ?, kind = ?, serial = ?, model = ?, capacity = ?, place = ?, notes = ?,
			s3_endpoint = ?, s3_region = ?, s3_bucket = ?, s3_prefix = ?, s3_access_key = ?, s3_secret_key = ? WHERE id = ?`,
		append(append([]any{d.Label, d.Kind, d.Serial, d.Model, d.Capacity, d.Place, d.Notes}, s3Args(d.S3)...), d.ID)...); err != nil {
		return d, err
	}
	if !sameBucket(old.S3, d.S3) {
		if err := forgetBucket(ctx, tx, d.ID); err != nil {
			return d, err
		}
	}
	if err := tx.Commit(); err != nil {
		return d, err
	}
	return s.GetDisk(ctx, d.ID)
}

func forgetBucket(ctx context.Context, tx execer, diskID int64) error {
	for _, q := range []string{
		`DELETE FROM archives WHERE disk_id = ? AND source = 's3'`,
		`DELETE FROM bucket_entries WHERE disk_id = ?`,
		`UPDATE disks SET s3_scan_at = NULL, s3_error = '', s3_objects = 0, s3_size = 0 WHERE id = ?`,
	} {
		if _, err := tx.ExecContext(ctx, q, diskID); err != nil {
			return err
		}
	}
	return nil
}

// DeleteDisk removes the disk. Moves recorded by hand stay, with their path
// only; what was found in its bucket goes with it.
func (s *Store) DeleteDisk(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM archives WHERE disk_id = ? AND source = 's3'`, id); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM disks WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
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
