package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/tehmaat/bookrr/internal/config"
	"github.com/tehmaat/bookrr/internal/gaps"
	"github.com/tehmaat/bookrr/internal/store"
	"github.com/tehmaat/bookrr/internal/tmdb"
	"github.com/tehmaat/bookrr/internal/unit3d"
)

// fakeTMDB answers for the films 603 and 604 and finds tt0133093.
func fakeTMDB(t *testing.T) *tmdb.Client {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/movie/603":
			w.Write([]byte(`{"id": 603, "title": "Matrix", "original_title": "The Matrix", "release_date": "1999-03-31", "imdb_id": "tt0133093", "poster_path": "/m.jpg"}`))
		case "/movie/604":
			w.Write([]byte(`{"id": 604, "title": "Matrix Reloaded", "original_title": "The Matrix Reloaded", "release_date": "2003-05-15"}`))
		case "/find/tt0133093":
			w.Write([]byte(`{"movie_results": [{"id": 603}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	c := tmdb.New("k")
	c.BaseURL = srv.URL
	c.MinInterval = 0
	return c
}

type planList struct {
	Enabled              bool   `json:"enabled"`
	DisabledReason       string `json:"disabledReason"`
	SearchEnabled        bool   `json:"searchEnabled"`
	SearchDisabledReason string `json:"searchDisabledReason"`
	Entries              []struct {
		store.PlanEntry
		Released   bool                  `json:"released"`
		Best       *store.BestResolution `json:"best"`
		Uploads    []planUpload          `json:"uploads"`
		Others     []gaps.Ref            `json:"others"`
		TrackerURL string                `json:"trackerUrl"`
	} `json:"entries"`
}

func TestPlanDisabled(t *testing.T) {
	_, h := newServer(t)
	var out planList
	if code := call(t, h, "GET", "/api/plan", nil, &out); code != http.StatusOK || out.Enabled || out.SearchEnabled ||
		!strings.Contains(out.DisabledReason, "TMDB") || !strings.Contains(out.SearchDisabledReason, "Prowlarr") || out.Entries == nil {
		t.Fatalf("code %d, %+v", code, out)
	}
	if code := call(t, h, "POST", "/api/plan", planInput{TMDB: "603"}, nil); code != http.StatusBadRequest {
		t.Fatalf("added without TMDB: %d", code)
	}
}

func TestPlan(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	srv := New(config.Config{Unit3DURL: "https://t.example/"}, st, nil, nil, nil, nil, nil, nil, fstest.MapFS{}, "test")
	srv.SetServices(Services{TMDB: fakeTMDB(t)})
	h := srv.Handler()

	var look struct {
		Movie  store.PlanEntry `json:"movie"`
		InPlan int64           `json:"inPlan"`
	}
	if code := call(t, h, "GET", "/api/plan/lookup?q=https%3A%2F%2Fwww.themoviedb.org%2Fmovie%2F603-the-matrix", nil, &look); code != http.StatusOK ||
		look.Movie.TmdbID != 603 || look.Movie.ImdbID != 133093 || look.Movie.OriginalTitle != "The Matrix" || look.InPlan != 0 {
		t.Fatalf("lookup %d %+v", code, look)
	}
	for _, q := range []string{"https://www.themoviedb.org/tv/1399", "boh", "777"} {
		var e map[string]string
		if code := call(t, h, "POST", "/api/plan", planInput{TMDB: q}, &e); code != http.StatusBadRequest || e["error"] == "" {
			t.Errorf("%q: %d %v", q, code, e)
		}
	}
	// By IMDb id.
	var e store.PlanEntry
	if code := call(t, h, "POST", "/api/plan", planInput{TMDB: "https://www.imdb.com/title/tt0133093/", Note: "in 4K"}, &e); code != http.StatusCreated ||
		e.TmdbID != 603 || e.Title != "Matrix" || e.Note != "in 4K" || e.Status != store.PlanTodo {
		t.Fatalf("add %d %+v", code, e)
	}
	if code := call(t, h, "POST", "/api/plan", planInput{TMDB: "603"}, nil); code != http.StatusConflict {
		t.Fatalf("added twice: %d", code)
	}
	if code := call(t, h, "GET", "/api/plan/lookup?q=603", nil, &look); code != http.StatusOK || look.InPlan != e.ID {
		t.Fatalf("lookup in plan %d %+v", code, look)
	}

	// From the missing films of a saga: 603 is there already, 605 is not on TMDB.
	var bulk struct {
		Added   []store.PlanEntry `json:"added"`
		Skipped int               `json:"skipped"`
		Errors  []string          `json:"errors"`
	}
	if code := call(t, h, "POST", "/api/plan/bulk", planBulkInput{TmdbIDs: []int64{603, 604, 604, 605}, Source: "saga:collection:2344", SourceTitle: "Matrix"}, &bulk); code != http.StatusOK ||
		len(bulk.Added) != 1 || bulk.Added[0].TmdbID != 604 || bulk.Added[0].SourceTitle != "Matrix" || bulk.Skipped != 1 || len(bulk.Errors) != 1 {
		t.Fatalf("bulk %d %+v", code, bulk)
	}

	// What the tracker has of the films: the user's uploads, and the
	// torrents of others found by the check of the sagas.
	if _, err := st.SaveTrackerReleases(ctx, []store.TrackerUpload{
		{ID: 1, Name: "The.Matrix.1999.1080p.BluRay.x264-ME", Category: "Movie", TmdbID: 603, DetailsLink: "https://t.example/torrents/1"},
	}); err != nil {
		t.Fatal(err)
	}
	g := gaps.Group{Format: gaps.Format, Kind: gaps.KindSaga, Key: "collection:2344", Title: "Matrix", Units: []gaps.Unit{
		{Number: 2, TmdbID: 604, Versions: []gaps.Version{{Ref: gaps.Ref{ID: 9, Name: "The.Matrix.Reloaded.2003.2160p.REMUX-X", Quality: "2160p Remux"}}}},
	}}
	data, _ := json.Marshal(g)
	if err := st.SaveGapGroups(ctx, gaps.KindSaga, []store.GapGroup{{Key: g.Key, Title: g.Title, Data: data}}); err != nil {
		t.Fatal(err)
	}
	var list planList
	if code := call(t, h, "GET", "/api/plan", nil, &list); code != http.StatusOK || !list.Enabled || len(list.Entries) != 2 {
		t.Fatalf("list %d %+v", code, list)
	}
	for _, it := range list.Entries {
		switch it.TmdbID {
		case 603:
			if len(it.Uploads) != 1 || it.Uploads[0].Quality != unit3d.QualityOf(ups0, "", "", "").Label || len(it.Others) != 0 ||
				it.TrackerURL != "https://t.example/torrents?tmdbId=603" || !it.Released || it.Best != nil {
				t.Errorf("603 %+v", it)
			}
		case 604:
			if len(it.Uploads) != 0 || len(it.Others) != 1 || it.Others[0].ID != 9 {
				t.Errorf("604 %+v", it)
			}
		}
	}

	status, note := store.PlanDoing, "audio da sincronizzare"
	if code := call(t, h, "PATCH", "/api/plan/"+itoa(e.ID), store.PlanUpdate{Status: &status, Note: &note}, &e); code != http.StatusOK ||
		e.Status != store.PlanDoing || e.Note != note {
		t.Fatalf("patch %d %+v", code, e)
	}
	bad := "fatto"
	if code := call(t, h, "PATCH", "/api/plan/"+itoa(e.ID), store.PlanUpdate{Status: &bad}, nil); code != http.StatusBadRequest {
		t.Fatalf("bad status: %d", code)
	}
	if code := call(t, h, "POST", "/api/plan/"+itoa(e.ID)+"/search", nil, nil); code != http.StatusBadRequest {
		t.Fatalf("search without Prowlarr: %d", code)
	}
	if code := call(t, h, "DELETE", "/api/plan/"+itoa(e.ID), nil, nil); code != http.StatusNoContent {
		t.Fatalf("delete %d", code)
	}
	if code := call(t, h, "DELETE", "/api/plan/"+itoa(e.ID), nil, nil); code != http.StatusNotFound {
		t.Fatalf("delete twice %d", code)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

const ups0 = "The.Matrix.1999.1080p.BluRay.x264-ME"
