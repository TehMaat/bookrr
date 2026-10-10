package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// ArchiveSourceS3 marks an archive found in the bucket of its disk.
const ArchiveSourceS3 = "s3"

// BucketDisks returns the archives backed by an S3 bucket.
func (s *Store) BucketDisks(ctx context.Context) ([]Disk, error) {
	ds, err := s.ListDisks(ctx)
	if err != nil {
		return nil, err
	}
	out := []Disk{}
	for _, d := range ds {
		if d.S3 != nil {
			out = append(out, d)
		}
	}
	return out, nil
}

// TorrentNames maps every torrent name, trimmed and lowercased, to the
// hashes of the torrents with that name.
func (s *Store) TorrentNames(ctx context.Context) (map[string][]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT hash, name FROM torrents`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var h, n string
		if err := rows.Scan(&h, &n); err != nil {
			return nil, err
		}
		k := strings.ToLower(strings.TrimSpace(n))
		out[k] = append(out[k], h)
	}
	return out, rows.Err()
}

// BucketEntry is a file or folder of a bucket.
type BucketEntry struct {
	Path       string  `json:"path"` // full key, folders end with "/"
	Name       string  `json:"name"`
	Size       int64   `json:"size"`
	Files      int     `json:"files"`
	ModifiedAt *string `json:"modifiedAt"`
}

// BucketMatch is a torrent found in a bucket.
type BucketMatch struct {
	Hash string
	Path string // shown as the path of the archive
}

// BucketScan is what a listing of the bucket found.
type BucketScan struct {
	Matches   []BucketMatch
	Unmatched []BucketEntry
	Objects   int
	Size      int64
}

// BucketResult counts the changes made by a scan.
type BucketResult struct {
	Added   int // torrents newly found in the bucket
	Removed int // torrents no longer there
	Alerts  int // new "where did it go?" alerts
}

func (s *Store) SetBucketError(ctx context.Context, diskID int64, scanErr error) error {
	_, err := s.db.ExecContext(ctx, `UPDATE disks SET s3_error = ? WHERE id = ?`, scanErr.Error(), diskID)
	return err
}

// ErrBucketChanged means the archive was pointed elsewhere during the scan.
var ErrBucketChanged = errors.New("il bucket dell'archivio è cambiato durante la lettura")

// ApplyBucketScan keeps the archives found in a bucket in sync with what it
// holds now. A torrent that appears there is recorded as moved to the disk,
// which answers its open alert; one that is no longer there loses that
// archive, and a personal release left with nowhere to be gets an alert.
// Torrents already recorded by hand on the same disk are left alone.
func (s *Store) ApplyBucketScan(ctx context.Context, disk Disk, scan BucketScan, releaseTag string) (BucketResult, error) {
	var res BucketResult
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()

	cur, err := scanDisk(tx.QueryRowContext(ctx, diskSelect+` WHERE d.id = ? GROUP BY d.id`, disk.ID))
	if errors.Is(err, sql.ErrNoRows) {
		return res, ErrNotFound
	}
	if err != nil {
		return res, err
	}
	if cur.S3 == nil || !sameBucket(cur.S3, disk.S3) {
		return res, ErrBucketChanged
	}

	type archive struct {
		id     int64
		path   string
		source string
	}
	auto, manual := map[string]archive{}, map[string]bool{}
	// Two torrents merged into one (see ReplaceHash) can leave the same
	// torrent found twice: those extra archives are dropped.
	var extra []int64
	rows, err := tx.QueryContext(ctx, `SELECT id, hash, path, source FROM archives WHERE disk_id = ? ORDER BY id`, disk.ID)
	if err != nil {
		return res, err
	}
	for rows.Next() {
		var a archive
		var h string
		if err := rows.Scan(&a.id, &h, &a.path, &a.source); err != nil {
			rows.Close()
			return res, err
		}
		switch {
		case a.source != ArchiveSourceS3:
			manual[h] = true
		case auto[h].id != 0:
			extra = append(extra, a.id)
		default:
			auto[h] = a
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return res, err
	}
	for _, id := range extra {
		if _, err := tx.ExecContext(ctx, `DELETE FROM archives WHERE id = ?`, id); err != nil {
			return res, err
		}
	}

	found := map[string]bool{}
	for _, m := range scan.Matches {
		if found[m.Hash] {
			continue
		}
		found[m.Hash] = true
		if manual[m.Hash] {
			continue
		}
		if a, ok := auto[m.Hash]; ok {
			if a.path != m.Path {
				if _, err := tx.ExecContext(ctx, `UPDATE archives SET path = ? WHERE id = ?`, m.Path, a.id); err != nil {
					return res, err
				}
			}
			continue
		}
		in := ArchiveInput{DiskID: &disk.ID, Path: m.Path}
		if _, err := tx.ExecContext(ctx, `INSERT INTO archives (hash, disk_id, path, notes, created_at, source) VALUES (?, ?, ?, '', ?, ?)`,
			m.Hash, disk.ID, m.Path, now(), ArchiveSourceS3); err != nil {
			return res, err
		}
		desc, err := describeArchive(ctx, tx, in)
		if err != nil {
			return res, err
		}
		if err := resolveOpenAlerts(ctx, tx, m.Hash, desc); err != nil {
			return res, err
		}
		res.Added++
	}

	for h, a := range auto {
		if found[h] && !manual[h] {
			continue
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM archives WHERE id = ?`, a.id); err != nil {
			return res, err
		}
		if found[h] {
			continue // still in the bucket, now recorded by hand
		}
		res.Removed++
		created, err := flagRemoval(ctx, tx, Removal{Hash: h, ClientName: disk.Label}, releaseTag, "s3")
		if err != nil {
			return res, err
		}
		if created {
			res.Alerts++
		}
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM bucket_entries WHERE disk_id = ?`, disk.ID); err != nil {
		return res, err
	}
	ins, err := tx.PrepareContext(ctx, `INSERT OR IGNORE INTO bucket_entries (disk_id, path, name, size, files, modified_at) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return res, err
	}
	defer ins.Close()
	for _, e := range scan.Unmatched {
		if _, err := ins.ExecContext(ctx, disk.ID, e.Path, e.Name, e.Size, e.Files, e.ModifiedAt); err != nil {
			return res, err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE disks SET s3_scan_at = ?, s3_error = '', s3_objects = ?, s3_size = ? WHERE id = ?`,
		now(), scan.Objects, scan.Size, disk.ID); err != nil {
		return res, err
	}
	return res, tx.Commit()
}

// BucketEntries returns what the last scan found in the disk's bucket that
// matches no torrent.
func (s *Store) BucketEntries(ctx context.Context, diskID int64) ([]BucketEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT path, name, size, files, modified_at FROM bucket_entries WHERE disk_id = ? ORDER BY name COLLATE NOCASE`, diskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []BucketEntry{}
	for rows.Next() {
		var e BucketEntry
		var mod sql.NullString
		if err := rows.Scan(&e.Path, &e.Name, &e.Size, &e.Files, &mod); err != nil {
			return nil, err
		}
		e.ModifiedAt = nullStr(mod)
		out = append(out, e)
	}
	return out, rows.Err()
}
