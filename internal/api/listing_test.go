package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tehmaat/bookrr/internal/config"
	"github.com/tehmaat/bookrr/internal/store"
	"github.com/tehmaat/bookrr/internal/syncer"
)

func TestDiskListing(t *testing.T) {
	ctx := context.Background()
	var mu sync.Mutex
	var deletes []string
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/torrents/delete" {
			r.ParseForm()
			mu.Lock()
			deletes = append(deletes, r.Form.Get("hashes")+" "+r.Form.Get("deleteFiles"))
			mu.Unlock()
		}
		fmt.Fprint(w, "Ok.")
	}))
	defer qb.Close()
	radarr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v3/history" && r.URL.Query().Get("downloadId") == strings.ToUpper(hashA):
			fmt.Fprint(w, `{"records":[{"eventType":"downloadFolderImported","movieId":5,"data":{"importedPath":"/movies/A/a.mkv"}}]}`)
		case r.URL.Path == "/api/v3/history":
			fmt.Fprint(w, `{"records":[]}`)
		case r.URL.Path == "/api/v3/movie/5" && r.Method == http.MethodGet:
			fmt.Fprint(w, `{"id":5,"title":"Film A","year":2020,"hasFile":true,"movieFile":{"path":"/movies/A/a.mkv"}}`)
		case r.URL.Path == "/api/v3/movie" && r.Method == http.MethodGet:
			fmt.Fprint(w, `[]`)
		case r.URL.Path == "/api/v3/movie/5" && r.Method == http.MethodDelete:
			mu.Lock()
			deletes = append(deletes, "radarr 5 "+r.URL.RawQuery)
			mu.Unlock()
		default:
			t.Errorf("radarr: unexpected %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer radarr.Close()

	st, err := store.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	srv := New(config.Config{RadarrURL: radarr.URL, RadarrAPIKey: "K"}, st, syncer.New(st, "Personal Release", time.Minute),
		nil, nil, nil, nil, nil, nil, "test")
	h := srv.Handler()

	c, err := st.CreateClient(ctx, store.Client{Name: "seedbox", URL: qb.URL, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.ApplySnapshot(ctx, c, []store.SnapshotTorrent{
		{Hash: hashA, Name: "Film.A.2020.1080p-GRP", Size: 1000, SavePath: "/data", Progress: 1},
		{Hash: hashB, Name: "Film.B.2021.1080p-GRP", Size: 2000, SavePath: "/data", Progress: 1},
		{Hash: hashC, Name: "Film.C.2022.1080p-GRP.mkv", Size: 3000, SavePath: "/data", Progress: 1},
	}); err != nil {
		t.Fatal(err)
	}
	var disk struct {
		ID int64 `json:"id"`
	}
	call(t, h, "POST", "/api/disks", map[string]any{"label": "Archive 04", "kind": "HDD"}, &disk)

	text := "1000 Films/Film.A.2020.1080p-GRP/a.mkv\n1500 Films/Film.B.2021.1080p-GRP/b.mkv\n3000 Films/Film.C.2022.1080p-GRP.mkv\n10 notes.txt\n"
	var p listingPreview
	if code := call(t, h, "POST", fmt.Sprintf("/api/disks/%d/listing", disk.ID), map[string]any{"listing": text, "prefix": "/Archive/"}, &p); code != 200 {
		t.Fatalf("preview: %d %+v", code, p)
	}
	checks := map[string]string{}
	for _, it := range p.Items {
		checks[it.Name] = it.Check + " " + it.Path
	}
	want := map[string]string{
		"Film.A.2020.1080p-GRP":     "complete Archive/Films/Film.A.2020.1080p-GRP",
		"Film.B.2021.1080p-GRP":     "incomplete Archive/Films/Film.B.2021.1080p-GRP",
		"Film.C.2022.1080p-GRP.mkv": "complete Archive/Films/Film.C.2022.1080p-GRP.mkv",
	}
	if fmt.Sprint(checks) != fmt.Sprint(want) || len(p.Unmatched) != 1 || !p.Radarr || p.Sonarr {
		t.Fatalf("preview %+v", p)
	}
	if p.Items[0].Radarr == nil || p.Items[0].Radarr.ID != 5 || !p.Items[0].Radarr.SameFile {
		t.Fatalf("radarr %+v", p.Items[0].Radarr)
	}

	apply := map[string]any{"listing": text, "prefix": "Archive", "items": []map[string]any{
		{"hash": hashA, "clients": true, "radarr": true},
		{"hash": hashB, "clients": true},
		{"hash": hashC, "clients": true, "radarr": true},
	}}
	var bad map[string]string
	if code := call(t, h, "POST", fmt.Sprintf("/api/disks/%d/listing/apply", disk.ID), apply, &bad); code != http.StatusBadRequest {
		t.Fatalf("without the choice on the files: %d %v", code, bad)
	}
	apply["deleteFiles"] = false
	var res struct {
		Results []listingResult `json:"results"`
	}
	if code := call(t, h, "POST", fmt.Sprintf("/api/disks/%d/listing/apply", disk.ID), apply, &res); code != 200 {
		t.Fatalf("apply: %d", code)
	}
	out, _ := json.Marshal(res)
	if fmt.Sprint(deletes) != fmt.Sprintf("[%s false radarr 5 addImportExclusion=false&deleteFiles=true %s false]", hashA, hashC) {
		t.Fatalf("deletes %v\n%s", deletes, out)
	}
	steps := func(i int) string {
		var s []string
		for _, st := range res.Results[i].Steps {
			s = append(s, fmt.Sprintf("%s:%t", st.What, st.OK))
		}
		return strings.Join(s, " ")
	}
	if steps(0) != "Archive:true seedbox:true Radarr:true" || steps(1) != "Disk:false" || steps(2) != "Archive:true seedbox:true" {
		t.Fatalf("steps %s", out)
	}

	a, _ := st.GetTorrent(ctx, hashA)
	if len(a.Locations) != 0 || len(a.Archives) != 1 || a.Archives[0].Path != "Archive/Films/Film.A.2020.1080p-GRP" {
		t.Fatalf("A after: %+v", a)
	}
	b, _ := st.GetTorrent(ctx, hashB)
	if len(b.Locations) != 1 || len(b.Archives) != 0 {
		t.Fatalf("B must be untouched: %+v", b)
	}

	// Again: already archived, nothing left on the clients, so no choice on
	// the files is needed.
	call(t, h, "POST", fmt.Sprintf("/api/disks/%d/listing", disk.ID), map[string]any{"listing": text}, &p)
	if !p.Items[0].Archived || len(p.Items[0].Locations) != 0 {
		t.Fatalf("second preview %+v", p.Items[0])
	}
	again := map[string]any{"listing": text, "prefix": "Archive", "items": []map[string]any{{"hash": hashA, "clients": true}}}
	if code := call(t, h, "POST", fmt.Sprintf("/api/disks/%d/listing/apply", disk.ID), again, &res); code != 200 || steps(0) != "Archive:true" {
		t.Fatalf("again: %d %+v", code, res)
	}
}

const (
	hashA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	hashB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	hashC = "cccccccccccccccccccccccccccccccccccccccc"
)
