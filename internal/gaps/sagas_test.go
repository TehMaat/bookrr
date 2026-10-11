package gaps

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/tehmaat/bookrr/internal/store"
)

func movieJSON(id int64, name, category, typ, res string, tmdbID int64) map[string]any {
	t := torrentJSON(id, name, category, typ, res, "2024-01-01T10:00:00Z", "")
	t["attributes"].(map[string]any)["tmdb_id"] = tmdbID
	return t
}

// newSagaFakes serves three sagas: 500 found by the search by saga (two
// pages), 600 that the search by saga does not find (one search per
// film) and 700 where the tracker ignores the filter.
func newSagaFakes(t *testing.T) *fakes {
	f := &fakes{}
	tm := map[string]string{
		"/3/movie/1001":     `{"id":1001,"title":"Film Uno","release_date":"2001-12-19","belongs_to_collection":{"id":500,"name":"Saga Test"}}`,
		"/3/movie/1002":     `{"id":1002,"title":"Film Due","release_date":"2002-12-18","belongs_to_collection":{"id":500,"name":"Saga Test"}}`,
		"/3/movie/2000":     `{"id":2000,"title":"Film Solo","release_date":"2015-01-01","belongs_to_collection":null}`,
		"/3/movie/3001":     `{"id":3001,"title":"Altro Film","release_date":"2010-05-01","belongs_to_collection":{"id":600,"name":"Altra Saga"}}`,
		"/3/movie/4001":     `{"id":4001,"title":"Terzo Film","release_date":"2015-05-01","belongs_to_collection":{"id":700,"name":"Terza Saga"}}`,
		"/3/find/tt0000777": `{"movie_results":[{"id":1002}],"tv_results":[]}`,
		"/3/collection/500": `{"id":500,"name":"Saga Test - La collezione","parts":[
			{"id":1003,"title":"Film Tre","release_date":"2003-12-17"},
			{"id":1001,"title":"Film Uno","release_date":"2001-12-19"},
			{"id":1004,"title":"Film Quattro","release_date":"2099-01-01"},
			{"id":1005,"title":"Film Cinque","release_date":""},
			{"id":1002,"title":"Film Due","release_date":"2002-12-18"}]}`,
		"/3/collection/600": `{"id":600,"name":"Altra Saga","parts":[
			{"id":3002,"title":"Altro Film 2","release_date":"2012-05-01"},
			{"id":3001,"title":"Altro Film","release_date":"2010-05-01"}]}`,
		"/3/collection/700": `{"id":700,"name":"Terza Saga","parts":[{"id":4001,"title":"Terzo Film","release_date":"2015-05-01"}]}`,
	}
	f.tmdb = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("api_key") != "TMDBKEY" {
			http.Error(w, "no key", http.StatusUnauthorized)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/3/find/") && r.URL.Query().Get("external_source") != "imdb_id" {
			t.Errorf("find without external_source: %s", r.URL)
		}
		body, ok := tm[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(body))
	}))
	t.Cleanup(f.tmdb.Close)

	pages := map[string][]map[string]any{
		"collectionId=500/1": {
			movieJSON(1, "Film Uno 2001 2160p UHD BluRay REMUX DV HDR ITA ENG-ME", "Movie", "Remux", "2160p", 1001),
			movieJSON(10, "Film.Uno.2001.1080p.WEB-DL.ITA.ENG-X", "Movie", "WEB-DL", "1080p", 1001),
			movieJSON(11, "Film Due 2002 2160p BluRay REMUX DV HDR ITA-Y", "Movie", "Remux", "2160p", 1002),
		},
		"collectionId=500/2": {
			movieJSON(2, "Film.Due.2002.1080p.WEB-DL.ITA.ENG-ME", "Movie", "WEB-DL", "1080p", 1002),
			movieJSON(12, "Film.Tre.2003.2160p.BluRay.REMUX.DV.HDR.ITA.ENG-Z", "Movie", "Remux", "2160p", 1003),
			// Not out yet for TMDB: left out.
			movieJSON(13, "Film.Quattro.2099.2160p.BluRay.REMUX.DV.HDR.ITA.ENG-Z", "Movie", "Remux", "2160p", 1004),
		},
		"tmdbId=3001/1": {
			movieJSON(4, "Altro.Film.2010.1080p.BluRay.x264.ITA-ME", "Movie", "Encode", "1080p", 3001),
		},
		"tmdbId=3002/1": {
			// A series with the same TMDB id: not a film.
			movieJSON(30, "Altro.Show.S01.1080p.WEB-DL.ITA-X", "TV", "WEB-DL", "1080p", 3002),
		},
		"collectionId=700/1": {
			movieJSON(40, "Tutt.Altro.2020.1080p.WEB-DL.ITA-X", "Movie", "WEB-DL", "1080p", 9999),
		},
		"tmdbId=4001/1": {
			movieJSON(5, "Terzo.Film.2015.720p.WEB-DL.ITA-ME", "Movie", "WEB-DL", "720p", 4001),
		},
	}
	f.tracker = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer KEY" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/api/user" {
			w.Write([]byte(`{"username":"me"}`))
			return
		}
		q := r.URL.Query()
		f.mu.Lock()
		f.queries = append(f.queries, q.Encode())
		f.mu.Unlock()
		page := q.Get("page")
		if page == "" {
			page = "1"
		}
		key := "collectionId=" + q.Get("collectionId")
		if q.Get("tmdbId") != "" {
			key = "tmdbId=" + q.Get("tmdbId")
		}
		res := map[string]any{"data": pages[key+"/"+page]}
		if (key == "collectionId=500" || key == "collectionId=700") && page == "1" {
			res["links"] = map[string]any{"next": "https://elsewhere.example/api/torrents/filter?page=2"}
		}
		if res["data"] == nil {
			res["data"] = []any{}
		}
		json.NewEncoder(w).Encode(res)
	}))
	t.Cleanup(f.tracker.Close)
	return f
}

func TestCheckSagas(t *testing.T) {
	f := newSagaFakes(t)
	c, st := newChecker(t, f)
	ctx := context.Background()
	ups := []store.TrackerUpload{
		{ID: 1, Name: "Film Uno 2001 2160p UHD BluRay REMUX DV HDR ITA ENG-ME", Category: "Movie", Type: "Remux", Resolution: "2160p", TmdbID: 1001},
		// No TMDB id: found from the IMDb id.
		{ID: 2, Name: "Film.Due.2002.1080p.WEB-DL.ITA.ENG-ME", Category: "Movie", Type: "WEB-DL", Resolution: "1080p", ImdbID: 777},
		// A film of no saga: not shown, not searched on the tracker.
		{ID: 3, Name: "Film.Solo.2015.1080p.WEB-DL.ITA-ME", Category: "Movie", Type: "WEB-DL", Resolution: "1080p", TmdbID: 2000},
		{ID: 4, Name: "Altro.Film.2010.1080p.BluRay.x264.ITA-ME", Category: "Movie", Type: "Encode", Resolution: "1080p", TmdbID: 3001},
		{ID: 5, Name: "Terzo.Film.2015.720p.WEB-DL.ITA-ME", Category: "Movie", Type: "WEB-DL", Resolution: "720p", TmdbID: 4001},
		// A series with the TMDB id of a film of a saga: not a film.
		{ID: 6, Name: "Uno.Show.S01.1080p.WEB-DL.ITA-ME", Category: "TV", Type: "WEB-DL", Resolution: "1080p", TmdbID: 1001},
		// No id at all: it cannot be checked.
		{ID: 7, Name: "Film.Ignoto.1999.1080p.WEB-DL.ITA-ME", Category: "Movie", Type: "WEB-DL", Resolution: "1080p"},
	}
	if _, err := st.SaveTrackerReleases(ctx, ups); err != nil {
		t.Fatal(err)
	}
	if _, err := c.checkSagas(ctx); err != nil {
		t.Fatal(err)
	}
	stored, err := st.GapGroups(ctx, KindSaga)
	if err != nil {
		t.Fatal(err)
	}
	groups := map[string]Group{}
	for _, sg := range stored {
		var g Group
		if err := json.Unmarshal(sg.Data, &g); err != nil {
			t.Fatal(err)
		}
		groups[g.Key] = g
	}
	var keys []string
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if !reflect.DeepEqual(keys, []string{"collection:500", "collection:600", "collection:700", "title:film ignoto"}) {
		t.Fatalf("groups %v", keys)
	}
	if at, _ := st.GapsCheckedAt(ctx, KindSaga); at == nil {
		t.Error("no check date")
	}

	g := groups["collection:500"]
	if g.Kind != KindSaga || g.Title != "Saga Test - La collezione" || g.Year != "2001" || g.Status != StatusOK || g.TmdbID != 500 ||
		g.SearchURL != f.tracker.URL+"/torrents?collectionId=500" {
		t.Fatalf("%+v", g)
	}
	// In order of release, without the films not out yet.
	var films []int64
	for i, u := range g.Units {
		films = append(films, u.TmdbID)
		if u.Number != i+1 {
			t.Errorf("unit %+v", u)
		}
	}
	if !reflect.DeepEqual(films, []int64{1001, 1002, 1003}) || g.Units[0].Name != "Film Uno" || g.Units[0].AirDate != "2001-12-19" {
		t.Fatalf("units %+v", g.Units)
	}
	type cell struct {
		state string
		ids   []int64
		quals []string
	}
	want := map[string][]cell{
		"2160p Remux DV HDR ITA ENG": {
			{StateMine, []int64{1}, nil},
			// Only in other qualities: without English, in 1080p.
			{StateOtherQuality, []int64{11, 2}, []string{"1080p WEB-DL ITA ENG", "2160p Remux DV HDR ITA"}},
			{StateAvailable, []int64{12}, nil},
		},
		"1080p WEB-DL ITA ENG": {
			{StateAvailable, []int64{10}, nil},
			{StateMine, []int64{2}, nil},
			{StateOtherQuality, []int64{12}, []string{"2160p Remux DV HDR ITA ENG"}},
		},
	}
	if len(g.Rows) != 2 || g.Rows[0].Quality.Label != "2160p Remux DV HDR ITA ENG" {
		t.Fatalf("rows %+v", g.Rows)
	}
	for _, row := range g.Rows {
		cells := want[row.Quality.Label]
		if len(row.Cells) != len(cells) {
			t.Fatalf("%s: %+v", row.Quality.Label, row.Cells)
		}
		for i, c := range row.Cells {
			var ids []int64
			for _, r := range c.Torrents {
				ids = append(ids, r.ID)
			}
			var quals []string
			if len(c.Qualities) > 0 {
				quals = c.Qualities
			}
			if c.Unit != i+1 || c.State != cells[i].state || !reflect.DeepEqual(ids, cells[i].ids) || !reflect.DeepEqual(quals, cells[i].quals) {
				t.Errorf("%s film %d: %+v, want %+v", row.Quality.Label, c.Unit, c, cells[i])
			}
		}
	}
	if g.Missing != 4 || g.Available != 2 || g.OtherQuality != 2 || g.Absent != 0 || len(g.Mine) != 2 {
		t.Errorf("totals %d %d %d %d, mine %+v", g.Missing, g.Available, g.OtherQuality, g.Absent, g.Mine)
	}

	// Not found by saga: searched film by film; the series is left out.
	g = groups["collection:600"]
	if len(g.Units) != 2 || len(g.Rows) != 1 || g.Rows[0].Quality.Label != "1080p Encode ITA" {
		t.Fatalf("%+v", g)
	}
	if cs := g.Rows[0].Cells; cs[0].State != StateMine || cs[1].State != StateAbsent || g.Missing != 1 || g.Absent != 1 {
		t.Errorf("cells %+v", cs)
	}

	// The tracker ignores the filter: searched film by film.
	g = groups["collection:700"]
	if len(g.Rows) != 1 || g.Rows[0].Cells[0].State != StateMine || g.Missing != 0 {
		t.Errorf("%+v", g)
	}

	g = groups["title:film ignoto"]
	if g.Status != StatusUnverifiable || g.Title != "Film Ignoto" || len(g.Mine) != 1 || g.Mine[0].ID != 7 || g.Note == "" {
		t.Errorf("unverifiable %+v", g)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	var asked []string
	for _, q := range f.queries {
		if !strings.Contains(q, "bookrr_user=me") || !strings.Contains(q, "perPage=100") {
			t.Errorf("query %q", q)
		}
		for _, p := range []string{"collectionId", "tmdbId"} {
			if i := strings.Index(q, p+"="); i >= 0 {
				v := q[i:]
				if j := strings.Index(v, "&"); j >= 0 {
					v = v[:j]
				}
				if strings.Contains(q, "page=2") {
					v += "/2"
				}
				asked = append(asked, v)
			}
		}
	}
	wantAsked := []string{"collectionId=500", "collectionId=500/2", "collectionId=600", "tmdbId=3001", "tmdbId=3002", "collectionId=700", "tmdbId=4001"}
	if !reflect.DeepEqual(asked, wantAsked) {
		t.Errorf("queries %v, want %v", asked, wantAsked)
	}
}

func TestIsMovie(t *testing.T) {
	for _, tc := range []struct {
		u    store.TrackerUpload
		want bool
	}{
		{store.TrackerUpload{Name: "Film.2019.1080p.WEB-DL.ITA", Category: "Movie"}, true},
		{store.TrackerUpload{Name: "Film 2019 2160p UHD Blu-ray Complete", Category: "Film"}, true},
		{store.TrackerUpload{Name: "Serie.S01.1080p.WEB-DL.ITA", Category: "TV"}, false},
		{store.TrackerUpload{Name: "Film.2019.1080p.WEB-DL.ITA", Category: "Serie TV"}, false},
		{store.TrackerUpload{Name: "Film.2019.1080p.WEB-DL.ITA"}, true},
		{store.TrackerUpload{Name: "Serie.S01E02.1080p.WEB-DL.ITA"}, false},
		{store.TrackerUpload{Name: "Film.2019.1080p.WEB-DL.ITA", TvdbID: 5}, false},
	} {
		if got := isMovie(tc.u); got != tc.want {
			t.Errorf("%+v: %v", tc.u, got)
		}
	}
}
