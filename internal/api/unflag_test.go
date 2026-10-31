package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/tehmaat/bookrr/internal/config"
	"github.com/tehmaat/bookrr/internal/store"
	"github.com/tehmaat/bookrr/internal/syncer"
)

// Unflagging removes the release tag on the clients and clears the flag; a
// client that fails keeps both on the torrents it has.
func TestUnflagReleases(t *testing.T) {
	ctx := context.Background()
	const tag = "Personal Release"
	var gotHashes, gotTags string
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/torrents/removeTags" {
			gotHashes, gotTags = r.FormValue("hashes"), r.FormValue("tags")
		}
	}))
	defer ok.Close()
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer broken.Close()

	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	a, _ := st.CreateClient(ctx, store.Client{Name: "seedbox", URL: ok.URL, Enabled: true})
	b, _ := st.CreateClient(ctx, store.Client{Name: "rotto", URL: broken.URL, Enabled: true})
	if _, err := st.ApplySnapshot(ctx, a, []store.SnapshotTorrent{
		{Hash: "h1", Name: "Uno", Tags: "4K,personal release"},
		{Hash: "h2", Name: "Due", Tags: "Personal Release"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ApplySnapshot(ctx, b, []store.SnapshotTorrent{{Hash: "h2", Name: "Due", Tags: "Personal Release"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.FinalizeSync(ctx, nil, tag); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateManualTorrent(ctx, store.ManualInput{TorrentInput: store.TorrentInput{
		Hash: "h3", Name: "Tre", Tags: []string{"Personal Release"}, PersonalRelease: true,
	}}); err != nil {
		t.Fatal(err)
	}

	h := New(config.Config{ReleaseTag: tag}, st, syncer.New(st, tag, time.Minute), nil, nil, nil, nil, nil, fstest.MapFS{}, "test").Handler()
	var out struct {
		Unflagged int      `json:"unflagged"`
		Errors    []string `json:"errors"`
	}
	if code := call(t, h, "POST", "/api/releases/unflag", map[string]any{"hashes": []string{"h1", "h2", "h3"}}, &out); code != 200 {
		t.Fatalf("unflag: %d %+v", code, out)
	}
	if out.Unflagged != 2 || len(out.Errors) != 1 {
		t.Fatalf("result %+v", out)
	}
	if gotHashes != "h1|h2" || gotTags != "personal release,Personal Release" {
		t.Fatalf("removeTags got hashes %q tags %q", gotHashes, gotTags)
	}

	for hash, want := range map[string]bool{"h1": false, "h2": true, "h3": false} {
		tr, err := st.GetTorrent(ctx, hash)
		if err != nil {
			t.Fatal(err)
		}
		if tr.PersonalRelease != want {
			t.Errorf("%s: personal %v, want %v", hash, tr.PersonalRelease, want)
		}
	}
	if tr, _ := st.GetTorrent(ctx, "h1"); len(tr.Tags) != 1 || tr.Tags[0] != "4K" || tr.Locations[0].Tags != "4K" {
		t.Errorf("h1 tags %v, location %+v", tr.Tags, tr.Locations)
	}
	if tr, _ := st.GetTorrent(ctx, "h3"); len(tr.Tags) != 0 {
		t.Errorf("h3 tags %v", tr.Tags)
	}

	// The next sync must not set the flag again on h1.
	if _, err := st.ApplySnapshot(ctx, a, []store.SnapshotTorrent{{Hash: "h1", Name: "Uno", Tags: "4K"}, {Hash: "h2", Name: "Due"}}); err != nil {
		t.Fatal(err)
	}
	st.FinalizeSync(ctx, nil, tag)
	if tr, _ := st.GetTorrent(ctx, "h1"); tr.PersonalRelease {
		t.Error("h1 flagged again")
	}

	if code := call(t, h, "POST", "/api/releases/unflag", map[string]any{"all": true}, nil); code != http.StatusBadRequest {
		t.Fatalf("all without tracker: %d", code)
	}
}
