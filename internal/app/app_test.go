package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/tehmaat/bookrr/internal/config"
	"github.com/tehmaat/bookrr/internal/store"
)

func TestApply(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "bookrr.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	tracker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) }))
	defer tracker.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := New(ctx, st)

	cfg := config.Config{ReleaseTag: "PR", SyncInterval: time.Hour, S3ScanInterval: time.Hour, Unit3DReleasesInterval: time.Hour}
	a := r.Apply(cfg)
	if a.Syncer == nil || a.Buckets == nil || a.History != nil || a.Missing != nil || a.Best != nil {
		t.Fatalf("first %+v", a)
	}

	// Only the reading of the clients changes.
	cfg.SyncInterval = 2 * time.Hour
	b := r.Apply(cfg)
	if b.Syncer == a.Syncer || b.Buckets != a.Buckets {
		t.Fatal("sync interval")
	}

	// A tracker, TMDB and Prowlarr: every part that reads the tracker starts.
	cfg.Unit3DURL, cfg.Unit3DAPIKey, cfg.TMDBAPIKey = tracker.URL, "k", "t"
	cfg.ProwlarrURL, cfg.ProwlarrAPIKey, cfg.ProwlarrRecheck = "http://prowlarr", "p", 24*time.Hour
	c := r.Apply(cfg)
	if c.Syncer != b.Syncer || c.Hashes == nil || c.History == nil || c.Missing == nil || c.Missing.MDBList != nil || c.Best == nil {
		t.Fatalf("tracker %+v", c)
	}

	// MDBList: only the check of the missing ones starts again.
	cfg.MDBListAPIKey = "m"
	d := r.Apply(cfg)
	if d.History != c.History || d.Best != c.Best || d.Missing == c.Missing || d.Missing.MDBList == nil {
		t.Fatalf("mdblist %+v", d)
	}

	// No tracker: nothing reads it.
	cfg.Unit3DURL, cfg.Unit3DAPIKey = "", ""
	e := r.Apply(cfg)
	if e.Hashes != nil || e.History != nil || e.Missing != nil || e.Best != nil || e.Syncer != d.Syncer {
		t.Fatalf("no tracker %+v", e)
	}
	if len(r.stops) != 2 {
		t.Fatalf("running %v", r.stops)
	}
}
