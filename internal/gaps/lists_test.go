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

	"github.com/tehmaat/bookrr/internal/mdblist"
	"github.com/tehmaat/bookrr/internal/store"
)

// newListFakes serves four lists on MDBList: "mcu" (an item with only the
// IMDb id, one not out yet, one not out yet but uploaded by the user, one
// twice), "pixar" (a film of the MCU too), "empty" (none of the user's
// films) and "gone" (deleted).
func newListFakes(t *testing.T) (*fakes, *httptest.Server) {
	f := &fakes{}
	tm := map[string]string{
		"/3/movie/1726":     `{"id":1726,"title":"Iron Man","release_date":"2008-04-30"}`,
		"/3/movie/1724":     `{"id":1724,"title":"L'incredibile Hulk","release_date":"2008-06-12"}`,
		"/3/movie/10138":    `{"id":10138,"title":"Iron Man 2","release_date":"2010-04-28"}`,
		"/3/movie/9999":     `{"id":9999,"title":"Futuro","release_date":"2099-01-01"}`,
		"/3/movie/8888":     `{"id":8888,"title":"Futuro Mio","release_date":"2099-05-01"}`,
		"/3/movie/7777":     `{"id":7777,"title":"Toy Story","release_date":"1995-11-22"}`,
		"/3/find/tt0000555": `{"movie_results":[{"id":1724}],"tv_results":[]}`,
	}
	f.tmdb = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := tm[r.URL.Path]
		if !ok || r.URL.Query().Get("api_key") != "TMDBKEY" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(body))
	}))
	t.Cleanup(f.tmdb.Close)

	lists := map[string]string{
		"/lists/me/mcu/items/movie": `{"movies":[
			{"id":10138,"rank":3,"title":"Iron Man 2","mediatype":"movie"},
			{"id":1726,"rank":1,"title":"Iron Man","mediatype":"movie"},
			{"id":null,"rank":2,"title":"The Incredible Hulk","imdb_id":"tt0000555","mediatype":"movie"},
			{"id":9999,"rank":4,"title":"Future","mediatype":"movie"},
			{"id":8888,"rank":5,"title":"My Future","mediatype":"movie"},
			{"id":1726,"rank":6,"title":"Iron Man","mediatype":"movie"}],"shows":[]}`,
		"/lists/me/pixar/items/movie": `{"movies":[{"id":10138,"rank":1,"title":"Iron Man 2","mediatype":"movie"}],"shows":[]}`,
		"/lists/me/empty/items/movie": `{"movies":[{"id":7777,"rank":1,"title":"Toy Story","mediatype":"movie"}],"shows":[]}`,
	}
	md := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := lists[r.URL.Path]
		if !ok || r.URL.Query().Get("apikey") != "MDBKEY" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(body))
	}))
	t.Cleanup(md.Close)

	pages := map[string][]map[string]any{
		"1726": {
			movieJSON(1, "Iron.Man.2008.1080p.WEB-DL.ITA-ME", "Movie", "WEB-DL", "1080p", 1726),
			movieJSON(20, "Iron Man 2008 2160p UHD BluRay REMUX DV HDR ITA-X", "Movie", "Remux", "2160p", 1726),
		},
		"1724": {
			movieJSON(21, "L.Incredibile.Hulk.2008.1080p.WEB-DL.ITA-Y", "Movie", "WEB-DL", "1080p", 1724),
			// A series with the same TMDB id: not a film.
			movieJSON(22, "Hulk.Show.S01.1080p.WEB-DL.ITA-Y", "TV", "WEB-DL", "1080p", 1724),
		},
		"7777": {
			movieJSON(23, "Toy.Story.1995.1080p.BluRay.x264.ITA-Z", "Movie", "Encode", "1080p", 7777),
		},
	}
	f.tracker = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/user" {
			w.Write([]byte(`{"username":"me"}`))
			return
		}
		q := r.URL.Query()
		f.mu.Lock()
		f.queries = append(f.queries, "tmdbId="+q.Get("tmdbId"))
		f.mu.Unlock()
		data := pages[q.Get("tmdbId")]
		if data == nil {
			data = []map[string]any{}
		}
		json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	t.Cleanup(f.tracker.Close)
	return f, md
}

func TestCheckLists(t *testing.T) {
	f, md := newListFakes(t)
	c, st := newChecker(t, f)
	ctx := context.Background()

	// Without an MDBList key the lists are not checked.
	if res, err := c.checkLists(ctx); res != "" || err != nil {
		t.Fatalf("without key: %q %v", res, err)
	}
	if at, _ := st.GapsCheckedAt(ctx, KindList); at != nil {
		t.Fatal("checked without key")
	}

	c.MDBList = mdblist.New("MDBKEY")
	c.MDBList.BaseURL = md.URL
	c.MDBList.MinInterval = 0
	for _, l := range []store.GapList{
		{Source: "mdblist", Path: "me/mcu", Title: "Marvel Cinematic Universe"},
		{Source: "mdblist", Path: "me/pixar", Title: "Pixar"},
		{Source: "mdblist", Path: "me/empty"},
		{Source: "mdblist", Path: "me/gone", Title: "Sparita"},
	} {
		if _, err := st.AddGapList(ctx, l); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.AddGapList(ctx, store.GapList{Source: "mdblist", Path: "me/mcu"}); err == nil {
		t.Error("same list added twice")
	}
	ups := []store.TrackerUpload{
		{ID: 1, Name: "Iron.Man.2008.1080p.WEB-DL.ITA-ME", Category: "Movie", Type: "WEB-DL", Resolution: "1080p", TmdbID: 1726},
		{ID: 2, Name: "Futuro Mio 2099 2160p UHD BluRay REMUX DV HDR ITA-ME", Category: "Movie", Type: "Remux", Resolution: "2160p", TmdbID: 8888},
	}
	if _, err := st.SaveTrackerReleases(ctx, ups); err != nil {
		t.Fatal(err)
	}
	res, err := c.checkLists(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res != "3 collezioni, 6 film mancanti, 1 lista non trovata" {
		t.Errorf("result %q", res)
	}
	stored, err := st.GapGroups(ctx, KindList)
	if err != nil {
		t.Fatal(err)
	}
	groups := map[string]Group{}
	var keys []string
	for _, sg := range stored {
		var g Group
		if err := json.Unmarshal(sg.Data, &g); err != nil {
			t.Fatal(err)
		}
		groups[g.Key] = g
		keys = append(keys, g.Key)
	}
	sort.Strings(keys)
	if !reflect.DeepEqual(keys, []string{"mdblist:me/empty", "mdblist:me/gone", "mdblist:me/mcu", "mdblist:me/pixar"}) {
		t.Fatalf("groups %v", keys)
	}

	g := groups["mdblist:me/mcu"]
	if g.Kind != KindList || g.Title != "Marvel Cinematic Universe" || g.Year != "2008" || g.Status != StatusOK ||
		g.SourceURL != "https://mdblist.com/lists/me/mcu" || g.SearchURL != "" || len(g.Mine) != 2 {
		t.Fatalf("%+v", g)
	}
	// In the order of the list, once each, without the film not out yet
	// unless it is the user's.
	var films []int64
	for i, u := range g.Units {
		films = append(films, u.TmdbID)
		if u.Number != i+1 {
			t.Errorf("unit %+v", u)
		}
	}
	if !reflect.DeepEqual(films, []int64{1726, 1724, 10138, 8888}) || g.Units[1].Name != "L'incredibile Hulk" {
		t.Fatalf("units %+v", g.Units)
	}
	if len(g.Rows) != 2 || g.Rows[0].Quality.Label != "2160p Remux DV HDR ITA" || g.Rows[1].Quality.Label != "1080p WEB-DL ITA" {
		t.Fatalf("rows %+v", g.Rows)
	}
	var states []string
	for _, c := range g.Rows[1].Cells {
		states = append(states, c.State)
	}
	if want := []string{StateMine, StateAvailable, StateAbsent, StateOtherQuality}; !reflect.DeepEqual(states, want) {
		t.Errorf("1080p cells %v, want %v", states, want)
	}

	// None of the user's films: no rows, but the films are there.
	g = groups["mdblist:me/empty"]
	if g.Title != "me/empty" || g.Status != StatusOK || len(g.Rows) != 0 || len(g.Units) != 1 || g.Year != "1995" {
		t.Fatalf("empty %+v", g)
	}
	Apply(&g, store.GapPrefs{}, nil, nil)
	if len(g.Parts) != 1 || g.Parts[0].State != StateAvailable || g.PartTotals.Missing != 1 {
		t.Errorf("empty parts %+v %+v", g.Parts, g.PartTotals)
	}

	g = groups["mdblist:me/gone"]
	if g.Status != StatusUnverifiable || g.Note == "" || g.Title != "Sparita" {
		t.Errorf("gone %+v", g)
	}

	// A film in two lists is searched once.
	f.mu.Lock()
	n := strings.Count(strings.Join(f.queries, " "), "tmdbId=10138")
	f.mu.Unlock()
	if n != 1 {
		t.Errorf("Iron Man 2 searched %d times: %v", n, f.queries)
	}

	// Removing a list takes its results and choices away.
	if err := st.SetGapMark(ctx, store.GapMark{Kind: KindList, Group: "mdblist:me/pixar", Action: ActionHide}); err != nil {
		t.Fatal(err)
	}
	all, _ := st.GapLists(ctx)
	for _, l := range all {
		if l.Path == "me/pixar" {
			if _, err := st.DeleteGapList(ctx, l.ID, KindList); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := st.GapGroup(ctx, KindList, "mdblist:me/pixar"); err != store.ErrNotFound {
		t.Errorf("group still there: %v", err)
	}
	if marks, _ := st.GapMarks(ctx, KindList); len(marks) != 0 {
		t.Errorf("marks %+v", marks)
	}
}

// A check of one kind leaves the others' results and outcome alone.
func TestCheckKinds(t *testing.T) {
	f, md := newListFakes(t)
	c, st := newChecker(t, f)
	ctx := context.Background()
	c.MDBList = mdblist.New("MDBKEY")
	c.MDBList.BaseURL = md.URL
	c.MDBList.MinInterval = 0
	c.results[KindSeries] = "2 serie, 1 stagioni mancanti"
	c.check(ctx, map[string]bool{KindList: true})
	if at, _ := st.GapsCheckedAt(ctx, KindSeries); at != nil {
		t.Error("series checked")
	}
	if at, _ := st.GapsCheckedAt(ctx, KindList); at == nil {
		t.Error("lists not checked")
	}
	s := c.State()
	if s.Running || s.LastResult != "2 serie, 1 stagioni mancanti · 0 collezioni, 0 film mancanti" || s.LastError != "" {
		t.Errorf("state %+v", s)
	}
}
