package store

import (
	"context"
	"testing"
)

// The archived copy of a personal release that bookrr already knows from
// a client: once its hash is found, the two become one torrent.
func TestReplaceHashMergesIntoKnownTorrent(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	a := mustClient(t, s, "a")
	real := "1111111111111111111111111111111111111111"
	sync(t, s, map[Client][]SnapshotTorrent{a: {{Hash: real, Name: "Mine", Size: 10, Tags: tag}}})
	sync(t, s, map[Client][]SnapshotTorrent{a: {}})
	if tr, _ := s.GetTorrent(ctx, real); tr.OpenAlert == nil {
		t.Fatal("expected an open alert")
	}

	adopter, err := s.CreateAdopter(ctx, Adopter{Name: "Mario"})
	if err != nil {
		t.Fatal(err)
	}
	placeholder := ManualHashPrefix + "abc"
	_, err = s.CreateManualTorrent(ctx, ManualInput{
		TorrentInput: TorrentInput{Hash: placeholder, Name: "Mine", Notes: "scatola 3"},
		Archive:      &ArchiveInput{Path: "/archivio"},
		Adoption:     &AdoptionInput{AdopterID: adopter.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetHashLookupError(ctx, placeholder, "non trovato", true); err != nil {
		t.Fatal(err)
	}

	merged, err := s.ReplaceHash(ctx, placeholder, real)
	if err != nil || !merged {
		t.Fatalf("merged=%v err=%v", merged, err)
	}
	tr, err := s.GetTorrent(ctx, real)
	if err != nil {
		t.Fatal(err)
	}
	if len(tr.Archives) != 1 || len(tr.Adoptions) != 1 || tr.Notes != "scatola 3" || tr.Size != 10 || tr.OpenAlert != nil || tr.HashLookupError != "" {
		t.Fatalf("unexpected merged torrent: %+v", tr)
	}
	if found, _ := s.TorrentExists(ctx, placeholder); found {
		t.Fatal("the placeholder must be gone")
	}
	if _, err := s.ReplaceHash(ctx, placeholder, real); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
