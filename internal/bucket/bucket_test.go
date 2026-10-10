package bucket

import (
	"fmt"
	"testing"
	"time"

	"github.com/tehmaat/bookrr/internal/s3"
)

func TestMatch(t *testing.T) {
	day := time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)
	objs := []s3.Object{
		{Key: "torrent/"}, // folder marker of the prefix itself
		{Key: "torrent/Film/Nome.Release.2160p/a.mkv", Size: 10, LastModified: day},
		{Key: "torrent/Film/Nome.Release.2160p/Sample/s.mkv", Size: 1},
		{Key: "torrent/Film/Sconosciuto/x.mkv", Size: 5},
		{Key: "torrent/Film/Sconosciuto/y.mkv", Size: 5, LastModified: day},
		{Key: "torrent/Singolo.File.mkv", Size: 7},
		{Key: "torrent/Backup/vecchio.zip", Size: 3},
		{Key: "torrent/Vuota/"},
	}
	names := map[string][]string{
		"nome.release.2160p": {"h1"},
		"singolo.file.mkv":   {"h2", "manual-x"},
		"sample":             {"never"}, // inside a torrent: the shallowest match wins
	}
	scan := Match("archivio", "torrent/", objs, names)

	if scan.Objects != 8 || scan.Size != 31 {
		t.Fatalf("objects %d size %d", scan.Objects, scan.Size)
	}
	got := fmt.Sprint(scan.Matches)
	want := "[{h1 s3://archivio/torrent/Film/Nome.Release.2160p/} {h2 s3://archivio/torrent/Singolo.File.mkv} {manual-x s3://archivio/torrent/Singolo.File.mkv}]"
	if got != want {
		t.Fatalf("matches\n got %s\nwant %s", got, want)
	}
	if len(scan.Unmatched) != 3 {
		t.Fatalf("unmatched %+v", scan.Unmatched)
	}
	b, s, v := scan.Unmatched[0], scan.Unmatched[1], scan.Unmatched[2]
	if b.Path != "torrent/Backup/" || b.Name != "Backup" || b.Files != 1 || b.Size != 3 {
		t.Errorf("backup %+v", b)
	}
	// Film holds a torrent, so its other folders are listed one by one.
	if s.Path != "torrent/Film/Sconosciuto/" || s.Files != 2 || s.Size != 10 || s.ModifiedAt == nil || *s.ModifiedAt != "2024-03-01T00:00:00Z" {
		t.Errorf("sconosciuto %+v", s)
	}
	if v.Path != "torrent/Vuota/" || v.Files != 0 {
		t.Errorf("vuota %+v", v)
	}
}

func TestNormalizePrefix(t *testing.T) {
	for in, want := range map[string]string{"": "", "/": "", " /torrent ": "torrent/", "a/b/": "a/b/"} {
		if got := NormalizePrefix(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}
