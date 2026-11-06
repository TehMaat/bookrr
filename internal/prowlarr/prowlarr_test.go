package prowlarr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tehmaat/bookrr/internal/store"
)

func TestSearchMovie(t *testing.T) {
	var got *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		w.Write([]byte(`[
			{"title": "The.Matrix.1999.2160p.UHD.BluRay.x265-GRP", "indexer": "A", "infoUrl": "https://a/1", "size": 123, "seeders": 4, "imdbId": 133093, "tmdbId": 603},
			{"title": "The.Matrix.1999.1080p.WEB-DL", "indexer": "B", "guid": "https://b/2", "seeders": null, "imdbId": "tt0133093"}
		]`))
	}))
	defer srv.Close()
	c := New(srv.URL+"/", "key")
	c.MinInterval = 0
	rs, err := c.SearchMovie(context.Background(), 133093, 603)
	if err != nil {
		t.Fatal(err)
	}
	if got.URL.Path != "/api/v1/search" || got.Header.Get("X-Api-Key") != "key" {
		t.Fatalf("request %s %v", got.URL, got.Header)
	}
	q := got.URL.Query()
	if q.Get("query") != "{ImdbId:tt0133093}{TmdbId:603}" || q.Get("type") != "movie" || q.Get("categories") != "2000" {
		t.Fatalf("query %v", q)
	}
	want := []Release{
		{Title: "The.Matrix.1999.2160p.UHD.BluRay.x265-GRP", Indexer: "A", InfoURL: "https://a/1", Size: 123, Seeders: 4, ImdbID: 133093, TmdbID: 603},
		{Title: "The.Matrix.1999.1080p.WEB-DL", Indexer: "B", InfoURL: "https://b/2", ImdbID: 133093},
	}
	if len(rs) != 2 || rs[0] != want[0] || rs[1] != want[1] {
		t.Fatalf("got %+v", rs)
	}
}

func TestSearchMovieRejectedKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) }))
	defer srv.Close()
	c := New(srv.URL, "bad")
	c.MinInterval = 0
	if _, err := c.SearchMovie(context.Background(), 1, 0); err == nil || !strings.Contains(err.Error(), "BOOKRR_PROWLARR_API_KEY") {
		t.Fatalf("err %v", err)
	}
}

func TestBest(t *testing.T) {
	f := films([]store.TrackerUpload{
		{Name: "The.Matrix.1999.1080p.BluRay.REMUX-ME", Category: "Movie", Resolution: "1080p", TmdbID: 603, ImdbID: 133093},
	})[0]
	b := f.best([]Release{
		// Another film, by id.
		{Title: "Other.Film.2001.2160p.WEB-DL", TmdbID: 999},
		// No ids: another film by title, and the same title another year.
		{Title: "Matrix.Reloaded.2003.2160p.WEB-DL"},
		{Title: "The.Matrix.2021.2160p.WEB-DL"},
		// An upscale is not a 2160p.
		{Title: "The.Matrix.1999.2160p.AI.Upscale", TmdbID: 603},
		{Title: "The.Matrix.1999.720p.HDTV", TmdbID: 603},
		{Title: "The.Matrix.1999.2160p.WEB-DL", Indexer: "A", Seeders: 3, TmdbID: 603},
		// No ids, same title and year: the most seeded 2160p of the best type.
		{Title: "The Matrix 1999 2160p UHD BluRay x265", Indexer: "B", InfoURL: "https://b", Seeders: 9},
		{Title: "The.Matrix.1999.2160p.BluRay.x265-LOW", Indexer: "C", Seeders: 2},
	})
	if b.Resolution != "2160p" || b.Type != "Encode" || b.Indexer != "B" || b.URL != "https://b" || b.Results != 4 ||
		b.Film != "tmdb:603" || b.Ranking != Ranking {
		t.Fatalf("got %+v", b)
	}
	// The type comes before the resolution: a 1080p Remux beats a 2160p Encode.
	b = f.best([]Release{
		{Title: "The.Matrix.1999.2160p.WEB-DL", Seeders: 50, TmdbID: 603},
		{Title: "The.Matrix.1999.1080p.BluRay.REMUX", Indexer: "R", Seeders: 1, TmdbID: 603},
		{Title: "The.Matrix.1999.720p.BluRay.REMUX", Seeders: 9, TmdbID: 603},
	})
	if b.Resolution != "1080p" || b.Type != "Remux" || b.Indexer != "R" {
		t.Fatalf("remux: %+v", b)
	}
	if b := f.best(nil); b.Resolution != "" || b.Results != 0 {
		t.Fatalf("nothing found: %+v", b)
	}
}

func TestGrade(t *testing.T) {
	for _, c := range []struct {
		a, b grade
		want bool
	}{
		// The issue: a 1080p Remux is better than a 2160p WEB-DL.
		{grade{"Remux", "1080p"}, grade{"WEB-DL", "2160p"}, true},
		{grade{"WEB-DL", "2160p"}, grade{"Remux", "1080p"}, false},
		{grade{"WEB-DL", "2160p"}, grade{"WEB-DL", "1080p"}, true},
		{grade{"Encode", "1080p"}, grade{"WEB-DL", "2160p"}, true},
		{grade{"WEB-DL", "1080p"}, grade{"", "2160p"}, true},
		// A Full Disc counts as a Remux.
		{grade{"Remux", "2160p"}, grade{"Full Disc", "1080p"}, true},
		{grade{"Remux", "1080p"}, grade{"Full Disc", "1080p"}, false},
		{grade{"Full Disc", "1080p"}, grade{"Remux", "1080p"}, false},
	} {
		if got := c.a.better(c.b); got != c.want {
			t.Errorf("%v better than %v: %v", c.a, c.b, got)
		}
	}
	if !(grade{"Full Disc", "2160p"}).top() || !(grade{"Remux", "2160p"}).top() || (grade{"WEB-DL", "2160p"}).top() {
		t.Error("top")
	}
}

func TestCheckAndAnnotate(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "bookrr.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.SaveTrackerReleases(ctx, []store.TrackerUpload{
		{ID: 1, Name: "The.Matrix.1999.1080p.BluRay.REMUX-ME", Category: "Movie", Resolution: "1080p", TmdbID: 603},
		{ID: 2, Name: "Dune.2021.2160p.BluRay.REMUX-ME", Category: "Movie", Type: "Remux", Resolution: "2160p", TmdbID: 438631},
		{ID: 3, Name: "Heat.1995.1080p.BluRay-ME", Category: "Movie", Resolution: "1080p", ImdbID: 113277},
		{ID: 4, Name: "Show.S01.1080p.WEB-DL-ME", Category: "TV", Resolution: "1080p", TmdbID: 603},
		{ID: 5, Name: "Old.Film.2000.720p-ME", Category: "Movie", Resolution: "720p"},
		// A 2160p WEB-DL is searched: there may be a Remux. The one found
		// in 2160p WEB-DL is no better than the user's 1080p Remux.
		{ID: 6, Name: "The.Wolverine.2013.1080p.BluRay.REMUX-ME", Category: "Movie", Type: "Remux", Resolution: "1080p", TmdbID: 76170},
		{ID: 7, Name: "Tenet.2020.2160p.WEB-DL-ME", Category: "Movie", Type: "WEB-DL", Resolution: "2160p", TmdbID: 577922},
	}); err != nil {
		t.Fatal(err)
	}
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("query")
		queries = append(queries, q)
		var out []map[string]any
		switch q {
		case "{TmdbId:603}":
			out = append(out, map[string]any{"title": "The.Matrix.1999.2160p.WEB-DL", "indexer": "A", "tmdbId": 603})
		case "{TmdbId:76170}":
			out = append(out, map[string]any{"title": "The.Wolverine.2013.Theatrical.Cut.2160p.MA.WEB-DL.DTS-HD.MA.7.1.H.265-FLUX", "tmdbId": 76170})
		case "{TmdbId:577922}":
			out = append(out, map[string]any{"title": "Tenet.2020.2160p.BluRay.REMUX", "indexer": "T", "tmdbId": 577922})
		}
		json.NewEncoder(w).Encode(out)
	}))
	defer srv.Close()
	c := New(srv.URL, "k")
	c.MinInterval = 0
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	f := NewFinder(st, c, 7*24*time.Hour)
	f.now = func() time.Time { return now }
	f.Check(ctx)
	// Not the 2160p Remux, not the series, not the film with no ids.
	if strings.Join(queries, " ") != "{TmdbId:603} {ImdbId:tt0113277} {TmdbId:76170} {TmdbId:577922}" {
		t.Fatalf("queries %v", queries)
	}
	if s := f.State(); s.LastError != "" || s.Done != 4 || !strings.HasPrefix(s.LastResult, "4 film cercati, 1 ") {
		t.Fatalf("state %+v", s)
	}
	// Searched again only once recheck has passed.
	now = now.Add(6 * 24 * time.Hour)
	f.Check(ctx)
	if len(queries) != 4 {
		t.Fatalf("searched again too soon: %v", queries)
	}
	now = now.Add(24 * time.Hour)
	f.Check(ctx)
	if len(queries) != 8 {
		t.Fatalf("not searched again: %v", queries)
	}

	rs, err := st.ListTrackerReleases(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found, err := st.BestResolutions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	Annotate(rs, found)
	byID := map[int64]*store.BestResolution{}
	for _, r := range rs {
		byID[r.ID] = r.Best
	}
	// The tracker does not give its type: read from the name, a Remux, better
	// than the 2160p WEB-DL found.
	if b := byID[1]; b == nil || b.Resolution != "2160p" || b.Type != "WEB-DL" || b.Mine != "1080p" || b.MineType != "Remux" || b.Better {
		t.Fatalf("matrix %+v", b)
	}
	if b := byID[2]; b == nil || b.CheckedAt != "" || b.Mine != "2160p" || b.MineType != "Remux" || b.Better {
		t.Fatalf("dune %+v", b)
	}
	if b := byID[6]; b == nil || b.Resolution != "2160p" || b.Type != "WEB-DL" || b.Mine != "1080p" || b.Better {
		t.Fatalf("wolverine %+v", b)
	}
	if b := byID[7]; b == nil || b.Type != "Remux" || b.MineType != "WEB-DL" || !b.Better || b.Indexer != "T" {
		t.Fatalf("tenet %+v", b)
	}
	if b := byID[3]; b == nil || b.Resolution != "" || b.CheckedAt == "" || b.Better {
		t.Fatalf("heat %+v", b)
	}
	if byID[4] != nil || byID[5] != nil {
		t.Fatalf("series or film without ids: %+v %+v", byID[4], byID[5])
	}
}

// A film searched with an older ranking is searched again, even before recheck.
func TestCheckOlderRanking(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "bookrr.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.SaveTrackerReleases(ctx, []store.TrackerUpload{
		{ID: 1, Name: "The.Matrix.1999.1080p.WEB-DL-ME", Category: "Movie", Resolution: "1080p", TmdbID: 603},
		{ID: 2, Name: "Heat.1995.1080p.WEB-DL-ME", Category: "Movie", Resolution: "1080p", TmdbID: 949},
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	recent := now.Add(-time.Hour).Format(time.RFC3339)
	for _, b := range []store.BestResolution{
		{Film: "tmdb:603", Resolution: "2160p", Name: "The.Matrix.1999.2160p.WEB-DL", CheckedAt: recent},
		{Film: "tmdb:949", CheckedAt: recent, Ranking: Ranking},
	} {
		if err := st.SaveBestResolution(ctx, b); err != nil {
			t.Fatal(err)
		}
	}
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.Query().Get("query"))
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	c := New(srv.URL, "k")
	c.MinInterval = 0
	f := NewFinder(st, c, 7*24*time.Hour)
	f.now = func() time.Time { return now }
	f.Check(ctx)
	if strings.Join(queries, " ") != "{TmdbId:603}" {
		t.Fatalf("queries %v", queries)
	}
}
