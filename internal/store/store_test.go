package store

import (
	"context"
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
	adopter := "mario"
	if err := s.ResolveAlert(ctx, tr.OpenAlert.ID, ResolveInput{Resolution: "su disco", Archive: &ArchiveInput{DiskID: &disk.ID, Path: "/rel/Mine"}, AdoptedBy: &adopter}); err != nil {
		t.Fatal(err)
	}
	tr, _ = s.GetTorrent(ctx, "h1")
	if tr.Status != StatusArchived || tr.AdoptedBy != "mario" || len(tr.Archives) != 1 || tr.Archives[0].DiskSerial != "WD-123" {
		t.Fatalf("unexpected after resolve: %+v", tr)
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
