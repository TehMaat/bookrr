package store

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// A database created before the release history keeps its data and gains
// the table of the tracker's uploads.
func TestReleasesMigration(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	const before = 8 // migrations up to the S3 buckets
	for i := 0; i < before; i++ {
		if _, err := db.ExecContext(ctx, migrations[i]); err != nil {
			t.Fatalf("migration %d: %v", i+1, err)
		}
	}
	if _, err := db.ExecContext(ctx, `PRAGMA user_version = 8;
INSERT INTO torrents (hash, name, personal_release, manual, first_seen_at, updated_at) VALUES ('h1', 'Vecchio', 1, 1, 'x', 'x')`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var version int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil || version != len(migrations) {
		t.Fatalf("version %d %v", version, err)
	}
	if tr, err := s.GetTorrent(ctx, "h1"); err != nil || tr.Name != "Vecchio" {
		t.Fatalf("data lost: %+v %v", tr, err)
	}
	if rs, err := s.ListTrackerReleases(ctx); err != nil || len(rs) != 0 {
		t.Fatalf("releases: %+v %v", rs, err)
	}
	if at, err := s.TrackerReleasesSyncedAt(ctx); err != nil || at != nil {
		t.Fatalf("never synced: %v %v", at, err)
	}
	// Before the first sync every personal release is "not on the tracker";
	// the API only shows them once the uploads have been read.
	if m, err := s.PersonalReleasesNotOnTracker(ctx); err != nil || len(m) != 1 {
		t.Fatalf("not on tracker: %+v %v", m, err)
	}
}

// The .torrent files stored in the rows of the uploads move to their own
// table, and the space they took is given back to the disk.
func TestReleaseFilesMigration(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	// The database as it was before the migration that moved the files out.
	before := 0
	for !strings.Contains(migrations[before], "CREATE TABLE tracker_release_files") {
		before++
	}
	for i := 0; i < before; i++ {
		if _, err := db.ExecContext(ctx, migrations[i]); err != nil {
			t.Fatalf("migration %d: %v", i+1, err)
		}
	}
	file := bytes.Repeat([]byte("d8:announce"), 50_000)
	if _, err := db.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", before)); err != nil {
		t.Fatal(err)
	}
	for id := 1; id <= 20; id++ {
		var data any
		if id != 2 {
			data = file
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO tracker_releases (id, name, hash, torrent_file, seen_at, updated_at) VALUES (?, ?, ?, ?, 'x', 'x')`,
			id, fmt.Sprint("Release ", id), fmt.Sprintf("h%d", id), data); err != nil {
			t.Fatal(err)
		}
	}
	var pagesBefore int64
	if err := db.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pagesBefore); err != nil {
		t.Fatal(err)
	}
	db.Close()

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var pages, free int64
	s.db.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pages)
	s.db.QueryRowContext(ctx, "PRAGMA freelist_count").Scan(&free)
	if free != 0 || pages > pagesBefore*11/10 { // without VACUUM the file would double
		t.Fatalf("not compacted: %d pages (%d before), %d free", pages, pagesBefore, free)
	}
	for id, want := range map[int64]bool{1: true, 2: false, 3: true} {
		if r, err := s.GetTrackerRelease(ctx, id); err != nil || r.HasTorrentFile != want {
			t.Fatalf("release %d: %+v %v", id, r, err)
		}
	}
	// The file still reaches the torrent created from the upload.
	if n, err := s.AddTrackerReleases(ctx, []int64{1}); err != nil || n != 1 {
		t.Fatalf("added %d %v", n, err)
	}
	if got, err := s.TorrentFile(ctx, "h1"); err != nil || !bytes.Equal(got, file) {
		t.Fatalf("file of h1: %d bytes, %v", len(got), err)
	}
	var cols int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('tracker_releases') WHERE name = 'torrent_file'`).Scan(&cols); err != nil || cols != 0 {
		t.Fatalf("torrent_file column still there: %d %v", cols, err)
	}
}

// The status of a release is the status of the torrent with its hash.
func TestTrackerReleaseStatus(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	adopter, err := s.CreateAdopter(ctx, Adopter{Name: "Mario"})
	if err != nil {
		t.Fatal(err)
	}
	add := func(hash string, in ManualInput) {
		t.Helper()
		in.Hash, in.Name = hash, hash
		if _, err := s.CreateManualTorrent(ctx, in); err != nil {
			t.Fatal(err)
		}
	}
	add("adopted", ManualInput{Adoption: &AdoptionInput{AdopterID: adopter.ID}})
	add("nowhere", ManualInput{})
	ups := []TrackerUpload{{ID: 1, Name: "A"}, {ID: 2, Name: "B", Size: 77}, {ID: 3, Name: "C"}, {ID: 4, Name: "D"}}
	if n, err := s.SaveTrackerReleases(ctx, ups); err != nil || n != 4 {
		t.Fatalf("saved %d %v", n, err)
	}
	for id, h := range map[int64]string{1: "adopted", 2: "nowhere", 3: "new"} {
		if err := s.SetTrackerReleaseHash(ctx, id, h, []byte("d4:infodee")); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SetTrackerReleaseHashError(ctx, 4, "HTTP 404"); err != nil {
		t.Fatal(err)
	}
	if ids, _ := s.TrackerReleasesWithoutHash(ctx); len(ids) != 1 || ids[0] != 4 {
		t.Fatalf("without hash: %v", ids)
	}
	want := map[int64]string{1: StatusAdopted, 2: StatusUnknown, 3: ReleaseUntracked, 4: ReleaseUntracked}
	rs, err := s.ListTrackerReleases(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rs {
		if r.Status != want[r.ID] {
			t.Errorf("release %d: status %s, want %s", r.ID, r.Status, want[r.ID])
		}
	}
	if tr, _ := s.GetTorrent(ctx, "nowhere"); tr.Size != 77 {
		t.Fatalf("a torrent added without size gets the tracker's: %+v", tr)
	}
	if r, _ := s.GetTrackerRelease(ctx, 4); r.HashError != "HTTP 404" || r.HasTorrentFile {
		t.Fatalf("release without hash: %+v", r)
	}
	// "Aggiungi tutte" skips what has no hash or is already known.
	if n, err := s.AddTrackerReleases(ctx, nil); err != nil || n != 1 {
		t.Fatalf("added %d %v", n, err)
	}
	if _, err := s.AddTrackerReleases(ctx, []int64{4}); err != ErrNoReleaseHash {
		t.Fatalf("expected ErrNoReleaseHash, got %v", err)
	}
	if _, err := s.AddTrackerReleases(ctx, []int64{99}); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if r, _ := s.GetTrackerRelease(ctx, 3); r.Status != StatusUnknown || r.TorrentHash != "new" {
		t.Fatalf("added release: %+v", r)
	}
}
