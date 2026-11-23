package arr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
)

type fake struct {
	mu     sync.Mutex
	routes map[string]any // "GET /path?query" or "GET /path" → JSON
	calls  []string
}

func (f *fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("X-Api-Key") != "KEY" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	call := r.Method + " " + r.URL.Path
	if r.URL.RawQuery != "" {
		call += "?" + r.URL.RawQuery
	}
	if r.Body != nil {
		var b any
		if json.NewDecoder(r.Body).Decode(&b) == nil {
			s, _ := json.Marshal(b)
			call += " " + string(s)
		}
	}
	f.calls = append(f.calls, call)
	for _, k := range []string{r.Method + " " + r.URL.Path + "?" + r.URL.Query().Encode(), r.Method + " " + r.URL.Path} {
		if v, ok := f.routes[k]; ok {
			json.NewEncoder(w).Encode(v)
			return
		}
	}
	if r.Method == http.MethodDelete || r.Method == http.MethodPut {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.WriteHeader(http.StatusNotFound)
}

func history(recs ...map[string]any) map[string]any {
	return map[string]any{"page": 1, "records": recs}
}

func TestRadarrFind(t *testing.T) {
	f := &fake{routes: map[string]any{
		"GET /api/v3/history?downloadId=ABC&page=1&pageSize=500": history(
			map[string]any{"eventType": "grabbed", "movieId": 7},
			map[string]any{"eventType": "downloadFolderImported", "movieId": 7, "data": map[string]string{"importedPath": "/movies/Movie (2020)/Movie.mkv"}},
		),
		"GET /api/v3/movie/7": map[string]any{"id": 7, "title": "Movie", "year": 2020, "hasFile": true, "movieFile": map[string]any{"path": "/movies/Movie (2020)/Movie.mkv"}},
		// Upgraded: Radarr has another file now.
		"GET /api/v3/history?downloadId=DEF&page=1&pageSize=500": history(
			map[string]any{"eventType": "downloadFolderImported", "movieId": 8, "data": map[string]string{"importedPath": "/movies/Other (2021)/Old.mkv"}},
		),
		"GET /api/v3/movie/8": map[string]any{"id": 8, "title": "Other", "year": 2021, "hasFile": true, "movieFile": map[string]any{"path": "/movies/Other (2021)/New.mkv", "sceneName": "Other.2021.2160p-X"}},
		// Not in the history: found by the name of the file.
		"GET /api/v3/history?downloadId=GHI&page=1&pageSize=500": history(),
		"GET /api/v3/movie": []any{
			map[string]any{"id": 9, "title": "Third", "year": 2019, "hasFile": true, "movieFile": map[string]any{"path": "/movies/Third/Third.mkv", "sceneName": "Third.2019.1080p.BluRay-GRP"}},
			map[string]any{"id": 10, "title": "Fourth", "year": 2018, "hasFile": true, "movieFile": map[string]any{"path": "/movies/Fourth/x.mkv", "originalFilePath": "Fourth.2018.720p-GRP/fourth.mkv"}},
		},
	}}
	srv := httptest.NewServer(f)
	defer srv.Close()
	l := NewRadarr(srv.URL+"/", "KEY").Lookup()
	ctx := context.Background()

	m, err := l.Find(ctx, "abc", "Movie.2020.1080p-GRP")
	if err != nil || m == nil || m.ID != 7 || !m.SameFile || !m.Deletable() || m.Via != "history" {
		t.Fatalf("abc: %+v %v", m, err)
	}
	m, err = l.Find(ctx, "def", "Other.2021.1080p-GRP")
	if err != nil || m == nil || m.ID != 8 || m.SameFile || m.Deletable() {
		t.Fatalf("def: %+v %v", m, err)
	}
	m, err = l.Find(ctx, "ghi", "Third.2019.1080p.BluRay-GRP")
	if err != nil || m == nil || m.ID != 9 || m.Via != "name" {
		t.Fatalf("ghi: %+v %v", m, err)
	}
	m, err = l.Find(ctx, "ghi", "Fourth.2018.720p-GRP")
	if err != nil || m == nil || m.ID != 10 {
		t.Fatalf("fourth: %+v %v", m, err)
	}
	m, err = l.Find(ctx, "ghi", "Nothing.2000")
	if err != nil || m != nil {
		t.Fatalf("nothing: %+v %v", m, err)
	}
	n := 0
	for _, c := range f.calls {
		if c == "GET /api/v3/movie" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("library read %d times", n)
	}

	if err := NewRadarr(srv.URL, "KEY").DeleteMovie(ctx, 7); err != nil {
		t.Fatal(err)
	}
	if last := f.calls[len(f.calls)-1]; last != "DELETE /api/v3/movie/7?addImportExclusion=false&deleteFiles=true" {
		t.Errorf("delete: %s", last)
	}
	if _, err := NewRadarr(srv.URL, "BAD").Version(ctx); err == nil || !strings.Contains(err.Error(), "BOOKRR_RADARR_API_KEY") {
		t.Errorf("bad key: %v", err)
	}
}

func TestSonarrFindAndRemove(t *testing.T) {
	imp := func(ep int, p string) map[string]any {
		return map[string]any{"eventType": "downloadFolderImported", "seriesId": 3, "episodeId": ep, "data": map[string]string{"importedPath": p}}
	}
	f := &fake{routes: map[string]any{
		"GET /api/v3/history?downloadId=AAA&page=1&pageSize=500": history(
			map[string]any{"eventType": "grabbed", "seriesId": 3, "episodeId": 31},
			map[string]any{"eventType": "grabbed", "seriesId": 3, "episodeId": 32},
			map[string]any{"eventType": "grabbed", "seriesId": 3, "episodeId": 33},
			imp(31, "/tv/Show/Season 1/e01.mkv"),
			imp(32, "/tv/Show/Season 1/e02.mkv"),
			imp(33, "/tv/Show/Season 1/e03.mkv"),
		),
		"GET /api/v3/series/3": map[string]any{"id": 3, "title": "Show", "year": 2010},
		"GET /api/v3/episode?seriesId=3": []any{
			map[string]any{"id": 31, "seasonNumber": 1, "hasFile": true, "episodeFileId": 101},
			map[string]any{"id": 32, "seasonNumber": 1, "hasFile": true, "episodeFileId": 102},
			map[string]any{"id": 33, "seasonNumber": 1, "hasFile": true, "episodeFileId": 103}, // upgraded
			map[string]any{"id": 41, "seasonNumber": 2, "hasFile": true, "episodeFileId": 201},
		},
		"GET /api/v3/episodefile?seriesId=3": []any{
			map[string]any{"id": 101, "path": "/tv/Show/Season 1/e01.mkv"},
			map[string]any{"id": 102, "path": "/tv/Show/Season 1/e02.mkv"},
			map[string]any{"id": 103, "path": "/tv/Show/Season 1/e03.PROPER.mkv"},
			map[string]any{"id": 201, "path": "/tv/Show/Season 2/e01.mkv"},
		},
		"GET /api/v3/history?downloadId=BBB&page=1&pageSize=500": history(),
	}}
	srv := httptest.NewServer(f)
	defer srv.Close()
	s := NewSonarr(srv.URL, "KEY")
	ctx := context.Background()

	e, err := s.Find(ctx, "aaa")
	if err != nil || e == nil {
		t.Fatalf("aaa: %+v %v", e, err)
	}
	want := &Episodes{SeriesID: 3, Title: "Show", Year: 2010, Seasons: []int{1}, Episodes: 3, Files: []int64{101, 102},
		Unmonitor: []int64{31, 32}, Upgraded: 1, SeriesFiles: 4}
	if !reflect.DeepEqual(e, want) {
		t.Fatalf("got %+v\nwant %+v", e, want)
	}
	if e, err := s.Find(ctx, "bbb"); err != nil || e != nil {
		t.Fatalf("bbb: %+v %v", e, err)
	}

	r := SeriesRemoval{SeriesID: 3, Files: e.Files, Unmonitor: e.Unmonitor, SeriesFiles: e.SeriesFiles}
	f.calls = nil
	if whole, err := s.Remove(ctx, r); err != nil || whole {
		t.Fatalf("remove: %v %v", whole, err)
	}
	wantCalls := []string{
		"DELETE /api/v3/episodefile/101",
		"DELETE /api/v3/episodefile/102",
		`PUT /api/v3/episode/monitor {"episodeIds":[31,32],"monitored":false}`,
	}
	if !reflect.DeepEqual(f.calls, wantCalls) {
		t.Errorf("calls %q", f.calls)
	}

	r.SeriesFiles = 2
	f.calls = nil
	if whole, err := s.Remove(ctx, r); err != nil || !whole {
		t.Fatalf("remove whole: %v %v", whole, err)
	}
	if !reflect.DeepEqual(f.calls, []string{"DELETE /api/v3/series/3?addImportListExclusion=false&deleteFiles=true"}) {
		t.Errorf("calls %q", f.calls)
	}
}
