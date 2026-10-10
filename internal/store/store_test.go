package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

const tag = "Personal Release"

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func mustClient(t *testing.T, s *Store, name string) Client {
	t.Helper()
	c, err := s.CreateClient(context.Background(), Client{Name: name, URL: "http://" + name, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func sync(t *testing.T, s *Store, snaps map[Client][]SnapshotTorrent) int {
	t.Helper()
	ctx := context.Background()
	var removed []Removal
	for c, ts := range snaps {
		r, err := s.ApplySnapshot(ctx, c, ts)
		if err != nil {
			t.Fatal(err)
		}
		removed = append(removed, r...)
	}
	n, err := s.FinalizeSync(ctx, removed, tag)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestSyncFlagsRemovedPersonalRelease(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	a, b := mustClient(t, s, "a"), mustClient(t, s, "b")

	mine := SnapshotTorrent{Hash: "h1", Name: "Mine", Size: 10, Tags: "Personal Release,x"}
	other := SnapshotTorrent{Hash: "h2", Name: "Other", Size: 20}
	sync(t, s, map[Client][]SnapshotTorrent{a: {mine, other}, b: {mine}})

	tr, err := s.GetTorrent(ctx, "h1")
	if err != nil {
		t.Fatal(err)
	}
	if !tr.PersonalRelease || !tr.Duplicate || tr.Status != StatusClient {
		t.Fatalf("unexpected torrent: %+v", tr)
	}

	// Removed from a only: still on b, no alert.
	if n := sync(t, s, map[Client][]SnapshotTorrent{a: {other}, b: {mine}}); n != 0 {
		t.Fatalf("expected no alert, got %d", n)
	}
	// Removed everywhere: alert. The non-personal torrent is forgotten.
	if n := sync(t, s, map[Client][]SnapshotTorrent{a: {}, b: {}}); n != 1 {
		t.Fatalf("expected 1 alert, got %d", n)
	}
	tr, _ = s.GetTorrent(ctx, "h1")
	if tr.Status != StatusMissing || tr.OpenAlert == nil {
		t.Fatalf("expected missing: %+v", tr)
	}
	if _, err := s.GetTorrent(ctx, "h2"); err != ErrNotFound {
		t.Fatalf("non-personal torrent should be forgotten, got %v", err)
	}
	// No duplicate alerts on further syncs.
	if n := sync(t, s, map[Client][]SnapshotTorrent{a: {}}); n != 0 {
		t.Fatalf("expected no new alert, got %d", n)
	}

	disk, err := s.CreateDisk(ctx, Disk{Label: "Archivio 1", Serial: "WD-123", Kind: "HDD"})
	if err != nil {
		t.Fatal(err)
	}
	mario, err := s.CreateAdopter(ctx, Adopter{Name: "mario"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ResolveAlert(ctx, tr.OpenAlert.ID, ResolveInput{Resolution: "su disco", Archive: &ArchiveInput{DiskID: &disk.ID, Path: "/rel/Mine"}, Adoption: &AdoptionInput{AdopterID: mario.ID}}); err != nil {
		t.Fatal(err)
	}
	tr, _ = s.GetTorrent(ctx, "h1")
	if tr.Status != StatusArchived || len(tr.Adoptions) != 1 || tr.Adoptions[0].AdopterName != "mario" || len(tr.Archives) != 1 || tr.Archives[0].DiskSerial != "WD-123" {
		t.Fatalf("unexpected after resolve: %+v", tr)
	}
}

func TestMoveResolvesAlert(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	a := mustClient(t, s, "a")
	sync(t, s, map[Client][]SnapshotTorrent{a: {{Hash: "h1", Name: "Mine", Tags: tag}, {Hash: "h2", Name: "Gift", Tags: tag}}})
	if n := sync(t, s, map[Client][]SnapshotTorrent{a: {}}); n != 2 {
		t.Fatalf("expected 2 alerts, got %d", n)
	}

	disk, _ := s.CreateDisk(ctx, Disk{Label: "Archivio 1", Serial: "WD-1"})
	if err := s.AddArchive(ctx, "h1", ArchiveInput{DiskID: &disk.ID, Path: "/x"}); err != nil {
		t.Fatal(err)
	}
	luigi, _ := s.CreateAdopter(ctx, Adopter{Name: "luigi"})
	if err := s.AddAdoption(ctx, "h2", AdoptionInput{AdopterID: luigi.ID}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddAdoption(ctx, "h2", AdoptionInput{AdopterID: luigi.ID}); err != ErrAlreadyAdopted {
		t.Fatalf("expected ErrAlreadyAdopted, got %v", err)
	}
	if open, _ := s.ListAlerts(ctx, true, ""); len(open) != 0 {
		t.Fatalf("moves should resolve alerts: %+v", open)
	}
	all, _ := s.ListAlerts(ctx, false, "h1")
	if len(all) != 1 || all[0].Resolution != "Spostato su Archivio 1 (SN WD-1) · /x" {
		t.Fatalf("unexpected resolution: %+v", all)
	}
	tr, _ := s.GetTorrent(ctx, "h2")
	if tr.Status != StatusAdopted {
		t.Fatalf("expected adopted: %+v", tr)
	}
	ads, _ := s.ListAdopters(ctx)
	if len(ads) != 1 || ads[0].AdoptedCount != 1 {
		t.Fatalf("unexpected adopters: %+v", ads)
	}
}

func TestRemovalAfterMoveIsNotFlagged(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	a := mustClient(t, s, "a")
	mine := SnapshotTorrent{Hash: "h1", Name: "Mine", Tags: tag}
	sync(t, s, map[Client][]SnapshotTorrent{a: {mine}})
	// Moved to a disk while still seeding, then removed from the client.
	if err := s.AddArchive(ctx, "h1", ArchiveInput{Path: "/archivio/Mine"}); err != nil {
		t.Fatal(err)
	}
	if n := sync(t, s, map[Client][]SnapshotTorrent{a: {}}); n != 0 {
		t.Fatalf("expected no new alert, got %d", n)
	}
	tr, _ := s.GetTorrent(ctx, "h1")
	if tr.Status != StatusArchived || tr.OpenAlert != nil {
		t.Fatalf("expected archived: %+v", tr)
	}
	all, _ := s.ListAlerts(ctx, false, "h1")
	if len(all) != 1 || all[0].ResolvedAt == nil || all[0].Resolution != "Già spostato su archivio · /archivio/Mine" {
		t.Fatalf("expected a resolved alert in the history: %+v", all)
	}
}

func TestMigrateAdoptedBy(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(migrations[0] + `
INSERT INTO torrents (hash, name, adopted_by, first_seen_at, updated_at) VALUES
	('h1', 'One', 'Mario', 't', 't'), ('h2', 'Two', ' mario ', 't', 't'), ('h3', 'Three', '', 't', 't');
PRAGMA user_version = 1;`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ads, _ := s.ListAdopters(ctx)
	if len(ads) != 1 || ads[0].Name != "Mario" || ads[0].AdoptedCount != 2 {
		t.Fatalf("unexpected adopters: %+v", ads)
	}
	tr, _ := s.GetTorrent(ctx, "h2")
	if tr.Status != StatusAdopted || tr.Adoptions[0].AdopterName != "Mario" {
		t.Fatalf("unexpected torrent: %+v", tr)
	}
}

func TestAlertClosedWhenTorrentReturns(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	a := mustClient(t, s, "a")
	mine := SnapshotTorrent{Hash: "h1", Name: "Mine", Tags: "personal release"}
	sync(t, s, map[Client][]SnapshotTorrent{a: {mine}})
	sync(t, s, map[Client][]SnapshotTorrent{a: {}})
	sync(t, s, map[Client][]SnapshotTorrent{a: {mine}})
	tr, _ := s.GetTorrent(ctx, "h1")
	if tr.OpenAlert != nil || tr.Status != StatusClient {
		t.Fatalf("alert should be closed: %+v", tr)
	}
}

func TestWebhookRemoval(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	a, b := mustClient(t, s, "a"), mustClient(t, s, "b")
	mine := SnapshotTorrent{Hash: "h1", Name: "Mine", Tags: "Personal Release"}
	sync(t, s, map[Client][]SnapshotTorrent{a: {mine}, b: {mine}})

	// Removed from a, still on b: no alert.
	if ok, err := s.HandleWebhookRemoval(ctx, WebhookRemoval{Hash: "h1", ClientName: "a"}, tag); err != nil || ok {
		t.Fatalf("got %v %v", ok, err)
	}
	if ok, err := s.HandleWebhookRemoval(ctx, WebhookRemoval{Hash: "h1", ClientName: "B"}, tag); err != nil || !ok {
		t.Fatalf("expected alert, got %v %v", ok, err)
	}

	// Torrent bookrr never saw, tagged in the payload.
	if ok, err := s.HandleWebhookRemoval(ctx, WebhookRemoval{Hash: "h9", Name: "New", Tags: "Personal Release"}, tag); err != nil || !ok {
		t.Fatalf("expected alert, got %v %v", ok, err)
	}
	// Untagged torrent: nothing.
	if ok, err := s.HandleWebhookRemoval(ctx, WebhookRemoval{Hash: "h8", Name: "Junk"}, tag); err != nil || ok {
		t.Fatalf("got %v %v", ok, err)
	}
	alerts, _ := s.ListAlerts(ctx, true, "")
	if len(alerts) != 2 {
		t.Fatalf("expected 2 open alerts, got %d", len(alerts))
	}
}

func TestFailedClientKeepsLocations(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	a := mustClient(t, s, "a")
	sync(t, s, map[Client][]SnapshotTorrent{a: {{Hash: "h1", Name: "Mine", Tags: "Personal Release"}}})
	// Client a fails: it isn't part of the snapshot set.
	if n := sync(t, s, map[Client][]SnapshotTorrent{}); n != 0 {
		t.Fatalf("expected no alert, got %d", n)
	}
	tr, _ := s.GetTorrent(ctx, "h1")
	if tr.Status != StatusClient {
		t.Fatalf("expected still on client: %+v", tr)
	}
}

func TestHasTag(t *testing.T) {
	if !HasTag("a, Personal Release ,b", "personal release") || HasTag("Personal Releases", tag) {
		t.Fatal("HasTag")
	}
	if NormalizeTags(" a, b ,, c") != "a,b,c" {
		t.Fatal("NormalizeTags")
	}
}

func TestRemoveDuplicateLocation(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	a, b := mustClient(t, s, "a"), mustClient(t, s, "b")

	done := SnapshotTorrent{Hash: "h1", Name: "Mine", Size: 10, Tags: tag, Progress: 1}
	partial := done
	partial.Progress = 0.5
	sync(t, s, map[Client][]SnapshotTorrent{a: {done}, b: {partial}})

	// b still has a complete copy on a: removable. a holds the only complete one.
	if err := s.CheckRemovableLocation(ctx, "h1", b.ID); err != nil {
		t.Fatalf("b should be removable: %v", err)
	}
	if err := s.CheckRemovableLocation(ctx, "h1", a.ID); err != ErrLastCopy {
		t.Fatalf("a is the only complete copy, got %v", err)
	}
	if err := s.CheckRemovableLocation(ctx, "h1", 999); err != ErrNotFound {
		t.Fatalf("unknown client, got %v", err)
	}

	if err := s.RemoveLocation(ctx, "h1", b.ID); err != nil {
		t.Fatal(err)
	}
	tr, err := s.GetTorrent(ctx, "h1")
	if err != nil {
		t.Fatal(err)
	}
	if tr.Duplicate || len(tr.Locations) != 1 || tr.Locations[0].ClientID != a.ID {
		t.Fatalf("expected only on a, got %+v", tr.Locations)
	}
	if err := s.CheckRemovableLocation(ctx, "h1", a.ID); err != ErrLastCopy {
		t.Fatalf("last copy must not be removable, got %v", err)
	}

	// The next sync confirms the removal from b without opening an alert.
	if n := sync(t, s, map[Client][]SnapshotTorrent{a: {done}, b: {}}); n != 0 {
		t.Fatalf("expected no alerts, got %d", n)
	}
}
