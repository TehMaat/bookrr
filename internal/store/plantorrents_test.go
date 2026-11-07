package store

import (
	"context"
	"errors"
	"testing"
)

func TestPlanTorrents(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	e, err := s.AddPlanEntry(ctx, PlanEntry{TmdbID: 603, Title: "Matrix"})
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.CreateClient(ctx, Client{Name: "seedbox", URL: "http://x", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	status := func() PlanEntry {
		t.Helper()
		got, err := s.PlanEntry(ctx, e.ID)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	sync := func(ts ...SnapshotTorrent) {
		t.Helper()
		if _, err := s.ApplySnapshot(ctx, c, ts); err != nil {
			t.Fatal(err)
		}
		if _, err := s.PromoteDownloadedPlan(ctx); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := s.SetPlanTorrent(ctx, 999, 0, PlanTorrentInput{Hash: "a", Data: []byte("x")}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown film: %v", err)
	}
	video, err := s.SetPlanTorrent(ctx, e.ID, 0, PlanTorrentInput{Hash: "aaa", Name: "Video", Size: 10, Label: " video ", Data: []byte("v")})
	if err != nil || video.Position != 1 || video.Label != "video" || video.SentAt != nil || len(video.Downloads) != 0 {
		t.Fatalf("first: %+v %v", video, err)
	}
	if _, err := s.SetPlanTorrent(ctx, e.ID, 0, PlanTorrentInput{Hash: "aaa", Data: []byte("v")}); !errors.Is(err, ErrPlanTorrentDup) {
		t.Fatalf("same torrent twice: %v", err)
	}
	// Complete on the client, but a single torrent of two is not the film.
	sync(SnapshotTorrent{Hash: "aaa", Name: "Video", Progress: 1, State: "uploading"})
	if got := status(); got.Status != PlanDownloaded || got.DownloadedAt == nil {
		t.Fatalf("one torrent complete: %+v", got)
	}

	// A second torrent for a merge puts it back in progress.
	audio, err := s.SetPlanTorrent(ctx, e.ID, 0, PlanTorrentInput{Hash: "bbb", Name: "Audio", Size: 5, Data: []byte("a")})
	if err != nil || audio.Position != 2 {
		t.Fatalf("second: %+v %v", audio, err)
	}
	if got := status(); got.Status != PlanDoing || got.DownloadedAt != nil {
		t.Fatalf("after the second torrent: %+v", got)
	}
	if _, err := s.SetPlanTorrent(ctx, e.ID, 0, PlanTorrentInput{Hash: "ccc", Data: []byte("c")}); !errors.Is(err, ErrPlanTorrentsFull) {
		t.Fatalf("third: %v", err)
	}
	if err := s.MarkPlanTorrentSent(ctx, e.ID, 2, c.ID); err != nil {
		t.Fatal(err)
	}
	sync(SnapshotTorrent{Hash: "aaa", Progress: 1}, SnapshotTorrent{Hash: "bbb", Progress: 0.5, State: "downloading"})
	ts, err := s.PlanTorrentsOf(ctx, e.ID)
	if err != nil || len(ts) != 2 || len(ts[1].Downloads) != 1 || ts[1].Downloads[0].Progress != 0.5 ||
		ts[1].Downloads[0].ClientName != "seedbox" || ts[1].SentClientName != "seedbox" || ts[1].Gone || ts[1].Complete() || !ts[0].Complete() {
		t.Fatalf("torrents %+v %v", ts, err)
	}
	if got := status(); got.Status != PlanDoing {
		t.Fatalf("half downloaded: %+v", got)
	}
	sync(SnapshotTorrent{Hash: "aaa", Progress: 1}, SnapshotTorrent{Hash: "bbb", Progress: 1})
	if got := status(); got.Status != PlanDownloaded {
		t.Fatalf("both complete: %+v", got)
	}

	// Put back in progress by hand: not made downloaded again by itself...
	doing := PlanDoing
	if _, err := s.UpdatePlanEntry(ctx, e.ID, PlanUpdate{Status: &doing}); err != nil {
		t.Fatal(err)
	}
	sync(SnapshotTorrent{Hash: "aaa", Progress: 1}, SnapshotTorrent{Hash: "bbb", Progress: 1})
	if got := status(); got.Status != PlanDoing {
		t.Fatalf("promoted again: %+v", got)
	}
	// ...until sent again.
	if err := s.MarkPlanTorrentSent(ctx, e.ID, 1, c.ID); err != nil {
		t.Fatal(err)
	}
	sync(SnapshotTorrent{Hash: "aaa", Progress: 1}, SnapshotTorrent{Hash: "bbb", Progress: 1})
	if got := status(); got.Status != PlanDownloaded {
		t.Fatalf("after sending again: %+v", got)
	}

	// Done on the tracker also when downloaded.
	if err := s.CompletePlanEntry(ctx, e.ID, 7); err != nil {
		t.Fatal(err)
	}
	if got := status(); got.Status != PlanDone || got.ReleaseID != 7 {
		t.Fatalf("completed: %+v", got)
	}

	// Removed from the client after it was sent (and the client read
	// since): gone.
	if _, err := s.db.ExecContext(ctx, `UPDATE plan_torrents SET sent_at = '2000-01-01T00:00:00Z'`); err != nil {
		t.Fatal(err)
	}
	sync()
	if ts, _ := s.PlanTorrentsOf(ctx, e.ID); !ts[1].Gone || !ts[0].Gone || len(ts[1].Downloads) != 0 {
		t.Fatalf("gone: %+v", ts)
	}

	// Replace, label, file, remove.
	if got, err := s.SetPlanTorrent(ctx, e.ID, 2, PlanTorrentInput{Hash: "ddd", Name: "Audio 2", Data: []byte("d")}); err != nil ||
		got.Hash != "ddd" || got.SentAt != nil {
		t.Fatalf("replaced: %+v %v", got, err)
	}
	if got, err := s.SetPlanTorrentLabel(ctx, e.ID, 2, "audio ITA"); err != nil || got.Label != "audio ITA" {
		t.Fatalf("label: %+v %v", got, err)
	}
	if data, err := s.PlanTorrentFile(ctx, e.ID, 2); err != nil || string(data) != "d" {
		t.Fatalf("file %q %v", data, err)
	}
	if err := s.DeletePlanTorrent(ctx, e.ID, 1); err != nil {
		t.Fatal(err)
	}
	if err := s.DeletePlanTorrent(ctx, e.ID, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted twice: %v", err)
	}
	// The free position is taken again.
	if got, err := s.SetPlanTorrent(ctx, e.ID, 0, PlanTorrentInput{Hash: "eee", Data: []byte("e")}); err != nil || got.Position != 1 {
		t.Fatalf("again: %+v %v", got, err)
	}
	// Removing the default client of the plan makes it ask again.
	if _, err := s.SaveSettings(ctx, Settings{PlanClientID: c.ID}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteClient(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	if st, err := s.GetSettings(ctx); err != nil || st.PlanClientID != 0 {
		t.Fatalf("default client after removal: %+v %v", st, err)
	}
	// Removing the film removes its torrents.
	if err := s.DeletePlanEntry(ctx, e.ID); err != nil {
		t.Fatal(err)
	}
	if all, err := s.PlanTorrents(ctx); err != nil || len(all) != 0 {
		t.Fatalf("left %+v %v", all, err)
	}
}
