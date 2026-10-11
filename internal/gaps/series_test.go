package gaps

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tehmaat/bookrr/internal/store"
	"github.com/tehmaat/bookrr/internal/tmdb"
	"github.com/tehmaat/bookrr/internal/unit3d"
)

func torrentJSON(id int64, name, category, typ, res, created, mediaInfo string) map[string]any {
	a := map[string]any{"name": name, "category": category, "type": typ, "resolution": res, "size": 1000,
		"created_at": created, "details_link": "https://t.example/torrents/" + jsonID(id)}
	if mediaInfo != "" {
		a["media_info"] = mediaInfo
	}
	return map[string]any{"id": id, "attributes": a}
}

func jsonID(id int64) string { b, _ := json.Marshal(id); return string(b) }

// fakes serves a TMDB and a UNIT3D tracker and records the tracker queries.
type fakes struct {
	tmdb, tracker *httptest.Server
	mu            sync.Mutex
	queries       []string
}

func newFakes(t *testing.T) *fakes {
	f := &fakes{}
	shows := map[string]string{
		"/3/tv/100": `{"id":100,"name":"Serie Test","first_air_date":"2019-03-01","seasons":[
			{"season_number":0,"name":"Speciali","air_date":"2019-01-01","episode_count":3},
			{"season_number":1,"name":"Stagione 1","air_date":"2019-03-01","episode_count":10},
			{"season_number":2,"name":"Stagione 2","air_date":"2020-03-01","episode_count":10},
			{"season_number":3,"name":"Stagione 3","air_date":"2021-03-01","episode_count":10},
			{"season_number":4,"name":"Stagione 4","air_date":"2022-03-01","episode_count":10},
			{"season_number":5,"name":"","air_date":"2023-03-01","episode_count":8},
			{"season_number":6,"name":"Stagione 6","air_date":"2099-03-01","episode_count":8},
			{"season_number":7,"name":"Stagione 7","air_date":null,"episode_count":0}]}`,
		"/3/tv/200": `{"id":200,"name":"Altra Serie","first_air_date":"2019-05-01","seasons":[
			{"season_number":1,"name":"Stagione 1","air_date":"2019-05-01","episode_count":6},
			{"season_number":2,"name":"Stagione 2","air_date":"2021-05-01","episode_count":6}]}`,
		"/3/find/555": `{"movie_results":[],"tv_results":[{"id":200}]}`,
		"/3/find/999": `{"movie_results":[],"tv_results":[]}`,
	}
	f.tmdb = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("api_key") != "TMDBKEY" {
			http.Error(w, "no key", http.StatusUnauthorized)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/3/find/") && r.URL.Query().Get("external_source") != "tvdb_id" {
			t.Errorf("find without external_source: %s", r.URL)
		}
		body, ok := shows[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(body))
	}))
	t.Cleanup(f.tmdb.Close)

	mediaInfo := "General\nFormat : Matroska\n\nAudio #1\nLanguage : Italian\n\nAudio #2\nLanguage : English\n\nText\nLanguage : French\n"
	pages := map[string][]map[string]any{
		"100/1": {
			torrentJSON(1, "Serie.Test.S01.2160p.WEB-DL.DDP5.1.ITA.ENG.DV.HDR.H.265-ME", "TV", "WEB-DL", "2160p", "2023-01-01T10:00:00Z", ""),
			torrentJSON(10, "Serie Test S02 2160p WEB-DL DDP5.1 ITA ENG DV HDR10 H.265-OTHER", "TV", "WEB-DL", "2160p", "2023-02-01T10:00:00Z", ""),
			torrentJSON(11, "Serie.Test.S03.2160p.WEB-DL.DDP5.1.ITA.DV.HDR.H.265-OTHER", "TV", "WEB-DL", "2160p", "2023-02-01T10:00:00Z", ""),
			torrentJSON(12, "Serie Test S03 2160p WEB-DL HDR ITA ENG", "TV", "WEB-DL", "2160p", "2023-02-01T10:00:00Z", ""),
			// Single episodes never count, not even for season 5.
			torrentJSON(13, "Serie.Test.S05E01.2160p.WEB-DL.ITA.ENG.DV.HDR-OTHER", "TV", "WEB-DL", "2160p", "2023-04-01T10:00:00Z", ""),
		},
		"100/2": {
			torrentJSON(14, "Serie Test S02-S03 1080p BluRay Remux ITA ENG-X", "TV", "Remux", "1080p", "2023-02-01T10:00:00Z", ""),
			torrentJSON(15, "Serie.Test.S04.1080p.BluRay.REMUX.AVC-X", "TV", "Remux", "1080p", "2023-02-01T10:00:00Z", mediaInfo),
			// A movie with the same TMDB id: another category.
			torrentJSON(16, "Serie.Test.S05.2160p.WEB-DL.DDP5.1.ITA.ENG.DV.HDR-ODD", "Movie", "WEB-DL", "2160p", "2023-04-01T10:00:00Z", ""),
			torrentJSON(2, "Serie Test S01 1080p BluRay REMUX AVC DTS-HD MA ITA ENG-ME", "TV", "Remux", "1080p", "2023-01-01T10:00:00Z", ""),
		},
		"200/1": {
			torrentJSON(5, "Altra.Serie.S02.1080p.WEB-DL.ITA-ME", "TV", "WEB-DL", "1080p", "2022-01-01T10:00:00Z", ""),
			// Uploaded before season 2 aired: it holds season 1 only.
			torrentJSON(20, "Altra Serie Complete 1080p WEB-DL ITA", "TV", "WEB-DL", "1080p", "2020-06-01T10:00:00Z", ""),
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
		res := map[string]any{"data": pages[q.Get("tmdbId")+"/"+page]}
		if q.Get("tmdbId") == "100" && page == "1" {
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

func newChecker(t *testing.T, f *fakes) (*Checker, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	tr := unit3d.New(f.tracker.URL, "KEY")
	tr.MinInterval = 0
	tm := tmdb.New("TMDBKEY")
	tm.BaseURL = f.tmdb.URL + "/3"
	tm.MinInterval = 0
	c := New(st, tr, tm, f.tracker.URL, 4*60)
	c.now = func() time.Time { return time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC) }
	return c, st
}

func TestCheckSeries(t *testing.T) {
	f := newFakes(t)
	c, st := newChecker(t, f)
	ctx := context.Background()
	ups := []store.TrackerUpload{
		{ID: 1, Name: "Serie.Test.S01.2160p.WEB-DL.DDP5.1.ITA.ENG.DV.HDR.H.265-ME", Category: "TV", Type: "WEB-DL", Resolution: "2160p", TmdbID: 100},
		{ID: 2, Name: "Serie Test S01 1080p BluRay REMUX AVC DTS-HD MA ITA ENG-ME", Category: "TV", Type: "Remux", Resolution: "1080p", TmdbID: 100},
		// My episodes are ignored: no quality of their own.
		{ID: 3, Name: "Serie.Test.S03E01.720p.WEB-DL.ITA-ME", Category: "TV", Type: "WEB-DL", Resolution: "720p", TmdbID: 100},
		{ID: 4, Name: "Film.2019.1080p.WEB-DL.ITA-ME", Category: "Movie", Type: "WEB-DL", Resolution: "1080p", TmdbID: 300},
		// No TMDB id: found from the TVDB id.
		{ID: 5, Name: "Altra.Serie.S02.1080p.WEB-DL.ITA-ME", Category: "TV", Type: "WEB-DL", Resolution: "1080p", TvdbID: 555},
		// Nowhere on TMDB: it cannot be checked.
		{ID: 6, Name: "Ignota.S01.720p.HDTV.ITA-ME", Category: "TV", Type: "HDTV", Resolution: "720p", TvdbID: 999},
	}
	if _, err := st.SaveTrackerReleases(ctx, ups); err != nil {
		t.Fatal(err)
	}
	c.Check(ctx)
	if s := c.State(); s.LastError != "" || s.Running || s.Done != 2 || s.Total != 2 {
		t.Fatalf("state %+v", s)
	}
	stored, err := st.GapGroups(ctx, KindSeries)
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
		if sg.Missing != g.Missing || sg.Title != g.Title {
			t.Errorf("stored row %+v vs %+v", sg, g)
		}
	}
	if len(groups) != 3 {
		t.Fatalf("%d groups: %v", len(groups), groups)
	}
	if at, _ := st.GapsCheckedAt(ctx, KindSeries); at == nil {
		t.Error("no check date")
	}

	g := groups["tmdb:100"]
	if g.Title != "Serie Test" || g.Year != "2019" || g.Status != StatusOK {
		t.Fatalf("%+v", g)
	}
	// Specials, seasons not out yet and without a date are left out.
	var units []int
	for _, u := range g.Units {
		units = append(units, u.Number)
	}
	if !reflect.DeepEqual(units, []int{1, 2, 3, 4, 5}) || g.Units[4].Name != "Stagione 5" {
		t.Fatalf("units %+v", g.Units)
	}
	type cell struct {
		state string
		ids   []int64
		quals []string
	}
	want := map[string][]cell{
		"2160p WEB-DL DV HDR ITA ENG": {
			{StateMine, []int64{1}, nil},
			{StateAvailable, []int64{10}, nil},
			{StateOtherQuality, []int64{11, 12, 14}, []string{"1080p Remux ITA ENG", "2160p WEB-DL DV HDR ITA", "2160p WEB-DL HDR ITA ENG"}},
			{StateOtherQuality, []int64{15}, []string{"1080p Remux ITA ENG"}},
			{StateAbsent, nil, nil},
		},
		"1080p Remux ITA ENG": {
			{StateMine, []int64{2}, nil},
			{StateAvailable, []int64{14}, nil},
			{StateAvailable, []int64{14}, nil},
			{StateAvailable, []int64{15}, nil}, // languages from the media info
			{StateAbsent, nil, nil},
		},
	}
	if len(g.Rows) != 2 || g.Rows[0].Quality.Label != "2160p WEB-DL DV HDR ITA ENG" {
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
				if r.URL == "" {
					t.Errorf("no link for %d", r.ID)
				}
			}
			var quals []string
			if len(c.Qualities) > 0 {
				quals = c.Qualities
			}
			if c.Unit != i+1 || c.State != cells[i].state || !reflect.DeepEqual(ids, cells[i].ids) || !reflect.DeepEqual(quals, cells[i].quals) {
				t.Errorf("%s S%d: %+v, want %+v", row.Quality.Label, c.Unit, c, cells[i])
			}
		}
		if row.Missing != 4 {
			t.Errorf("%s: %d missing", row.Quality.Label, row.Missing)
		}
	}
	if g.Missing != 8 || g.Available != 4 || g.OtherQuality != 2 || g.Absent != 2 {
		t.Errorf("totals %d %d %d %d", g.Missing, g.Available, g.OtherQuality, g.Absent)
	}
	if len(g.Mine) != 2 || g.SearchURL != f.tracker.URL+"/torrents?tmdbId=100" {
		t.Errorf("mine %+v, search %q", g.Mine, g.SearchURL)
	}

	g = groups["tmdb:200"]
	if g.Title != "Altra Serie" || g.TvdbID != 555 || len(g.Rows) != 1 {
		t.Fatalf("%+v", g)
	}
	if cs := g.Rows[0].Cells; cs[0].State != StateAvailable || cs[0].Torrents[0].ID != 20 || cs[1].State != StateMine {
		t.Errorf("complete pack: %+v", cs)
	}

	g = groups["tvdb:999"]
	if g.Status != StatusUnverifiable || g.Title != "Ignota" || len(g.Mine) != 1 || g.Mine[0].ID != 6 || g.Note == "" {
		t.Errorf("unverifiable %+v", g)
	}

	// One search per series, with every page, against the shared cache.
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.queries) != 3 {
		t.Fatalf("queries %v", f.queries)
	}
	for _, q := range f.queries {
		if !strings.Contains(q, "bookrr_user=me") || !strings.Contains(q, "perPage=100") || !strings.Contains(q, "tmdbId=") {
			t.Errorf("query %q", q)
		}
	}
}

func TestCheckFailureKeepsResults(t *testing.T) {
	f := newFakes(t)
	c, st := newChecker(t, f)
	ctx := context.Background()
	if _, err := st.SaveTrackerReleases(ctx, []store.TrackerUpload{
		{ID: 1, Name: "Serie.Test.S01.2160p.WEB-DL.ITA.ENG-ME", Category: "TV", Type: "WEB-DL", Resolution: "2160p", TmdbID: 100},
	}); err != nil {
		t.Fatal(err)
	}
	c.Check(ctx)
	if s := c.State(); s.LastError != "" {
		t.Fatal(s.LastError)
	}
	// A wrong key: the round fails, the last results stay.
	c.tmdb = tmdb.New("WRONG")
	c.tmdb.BaseURL = f.tmdb.URL + "/3"
	c.Check(ctx)
	if s := c.State(); !strings.Contains(s.LastError, "BOOKRR_TMDB_API_KEY") {
		t.Errorf("error %q", s.LastError)
	}
	if gs, _ := st.GapGroups(ctx, KindSeries); len(gs) != 1 {
		t.Errorf("results %+v", gs)
	}
}

func TestNextRun(t *testing.T) {
	c := &Checker{at: 4*60 + 30}
	loc := time.FixedZone("CET", 3600)
	for now, want := range map[time.Time]time.Time{
		time.Date(2026, 10, 11, 1, 0, 0, 0, loc):  time.Date(2026, 10, 11, 4, 30, 0, 0, loc),
		time.Date(2026, 10, 11, 4, 30, 0, 0, loc): time.Date(2026, 10, 12, 4, 30, 0, 0, loc),
		time.Date(2026, 10, 11, 23, 0, 0, 0, loc): time.Date(2026, 10, 12, 4, 30, 0, 0, loc),
	} {
		if got := c.nextRun(now); !got.Equal(want) {
			t.Errorf("%s: %s, want %s", now, got, want)
		}
	}
}
