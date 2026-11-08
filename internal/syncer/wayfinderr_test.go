package syncer

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"github.com/tehmaat/bookrr/internal/store"
)

// fakeWayfinderr keeps the source .torrent files sent to it, by hash (the
// test sends the hash as the file name, a real Wayfinderr reads it from the file).
type fakeWayfinderr struct {
	mu    sync.Mutex
	files map[string][]byte
	query map[string]string
	down  bool
}

func (f *fakeWayfinderr) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("X-Api-Key") != "key" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if f.down {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/source-torrents":
		data, _ := io.ReadAll(r.Body)
		hash := r.URL.Query().Get("name")
		f.files[hash[:len(hash)-len(".torrent")]] = data
		f.query = map[string]string{}
		for k := range r.URL.Query() {
			f.query[k] = r.URL.Query().Get(k)
		}
		w.WriteHeader(http.StatusCreated)
	case r.Method == http.MethodGet:
		if _, ok := f.files[filepath.Base(r.URL.Path)]; ok {
			w.Write([]byte(`{}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestWayfinderrHandover(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	fake := &fakeWayfinderr{files: map[string][]byte{}}
	srv := httptest.NewServer(fake)
	defer srv.Close()

	plain, err := st.CreateClient(ctx, store.Client{Name: "plain", URL: "http://plain", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	ripper, err := st.CreateClient(ctx, store.Client{Name: "ripper", URL: "http://ripper", Enabled: true,
		WayfinderrURL: srv.URL, WayfinderrAPIKey: "key"})
	if err != nil {
		t.Fatal(err)
	}
	e, err := st.AddPlanEntry(ctx, store.PlanEntry{TmdbID: 603, Title: "Matrix"})
	if err != nil {
		t.Fatal(err)
	}
	// The hash is also the torrent name, so that the fake finds it.
	if _, err := st.SetPlanTorrent(ctx, e.ID, 0, store.PlanTorrentInput{Hash: "aaa", Name: "aaa", Label: "video", Data: []byte("v")}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SetPlanTorrent(ctx, e.ID, 0, store.PlanTorrentInput{Hash: "bbb", Name: "bbb", Data: []byte("a")}); err != nil {
		t.Fatal(err)
	}
	run := func(wantSent, wantDropped int) {
		t.Helper()
		sent, dropped, err := Wayfinderr(ctx, st)
		if err != nil || sent != wantSent || dropped != wantDropped {
			t.Fatalf("sent %d dropped %d (want %d %d): %v", sent, dropped, wantSent, wantDropped, err)
		}
	}
	snapshot := func(c store.Client, ts ...store.SnapshotTorrent) {
		t.Helper()
		if _, err := st.ApplySnapshot(ctx, c, ts); err != nil {
			t.Fatal(err)
		}
	}

	// Complete on a client without a Wayfinderr, half done on the other: nothing to send.
	snapshot(plain, store.SnapshotTorrent{Hash: "aaa", Progress: 1})
	snapshot(ripper, store.SnapshotTorrent{Hash: "aaa", Progress: 0.5}, store.SnapshotTorrent{Hash: "bbb", Progress: 0.2})
	run(0, 0)

	// Complete on the client with a Wayfinderr: sent, with its film and label.
	snapshot(ripper, store.SnapshotTorrent{Hash: "aaa", Progress: 1}, store.SnapshotTorrent{Hash: "bbb", Progress: 0.2})
	run(1, 0)
	if string(fake.files["aaa"]) != "v" || fake.query["tmdbId"] != "603" || fake.query["label"] != "video" || fake.query["origin"] != "bookrr" {
		t.Fatalf("sent %q with %v", fake.files["aaa"], fake.query)
	}
	run(0, 0) // once only

	// Wayfinderr down: the error is kept and the next sync tries again.
	fake.down = true
	snapshot(ripper, store.SnapshotTorrent{Hash: "aaa", Progress: 1}, store.SnapshotTorrent{Hash: "bbb", Progress: 1})
	run(0, 0)
	ts, _ := st.PlanTorrentsOf(ctx, e.ID)
	if ts[1].WayfinderrAt != nil || ts[1].WayfinderrError == "" || ts[0].WayfinderrClientName != "ripper" {
		t.Fatalf("after a failure: %+v", ts)
	}
	fake.down = false
	run(1, 0)

	// Done by hand: the files stay.
	done := store.PlanDone
	if _, err := st.UpdatePlanEntry(ctx, e.ID, store.PlanUpdate{Status: &done}); err != nil {
		t.Fatal(err)
	}
	run(0, 0)

	// Released (found on the tracker), but Wayfinderr lost one: only the other goes.
	delete(fake.files, "bbb")
	reopen := store.PlanDownloaded
	if _, err := st.UpdatePlanEntry(ctx, e.ID, store.PlanUpdate{Status: &reopen}); err != nil {
		t.Fatal(err)
	}
	if err := st.CompletePlanEntry(ctx, e.ID, 42); err != nil {
		t.Fatal(err)
	}
	run(0, 1)
	if _, err := st.PlanTorrentFile(ctx, e.ID, 1); !errors.Is(err, store.ErrPlanTorrentDropped) {
		t.Fatalf("dropped file: %v", err)
	}
	if data, err := st.PlanTorrentFile(ctx, e.ID, 2); err != nil || string(data) != "a" {
		t.Fatalf("kept file: %q %v", data, err)
	}
	ts, _ = st.PlanTorrentsOf(ctx, e.ID)
	if !ts[0].Dropped || ts[1].Dropped || ts[0].Name != "aaa" || ts[0].Label != "video" {
		t.Fatalf("after the drop: %+v", ts)
	}
	fake.files["bbb"] = []byte("a")
	run(0, 1)

	// The same file loaded again comes back.
	if _, err := st.SetPlanTorrent(ctx, e.ID, 1, store.PlanTorrentInput{Hash: "aaa", Name: "aaa", Data: []byte("v")}); err != nil {
		t.Fatal(err)
	}
	if data, err := st.PlanTorrentFile(ctx, e.ID, 1); err != nil || string(data) != "v" {
		t.Fatalf("loaded again: %q %v", data, err)
	}
}

func TestFileName(t *testing.T) {
	if got := FileName("a/b\\c", "h"); got != "a_b_c.torrent" {
		t.Fatal(got)
	}
	if got := FileName("", "h"); got != "h.torrent" {
		t.Fatal(got)
	}
}
