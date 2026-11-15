package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/tehmaat/bookrr/internal/config"
	"github.com/tehmaat/bookrr/internal/prowlarr"
	"github.com/tehmaat/bookrr/internal/store"
)

func TestPlanDiscs(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	e, err := st.AddPlanEntry(ctx, store.PlanEntry{TmdbID: 603, ImdbID: 133093, Title: "Matrix", OriginalTitle: "The Matrix", ReleaseDate: "1999-03-31"})
	if err != nil {
		t.Fatal(err)
	}
	disc, hash := torrentOf("The.Matrix.1999.COMPLETE.UHD.BLURAY-GRP", 60<<20)
	var searches []string
	var downloads []*http.Request
	prow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/search":
			searches = append(searches, r.URL.Query().Get("limit"))
			json.NewEncoder(w).Encode([]map[string]any{
				// Prowlarr writes the host it is known by: bookrr asks its own.
				{"title": "The.Matrix.1999.COMPLETE.UHD.BLURAY-GRP", "indexer": "A", "guid": "a-1", "infoUrl": "https://a/1", "seeders": 3,
					"size": 60 << 30, "tmdbId": 603, "protocol": "torrent", "downloadUrl": "http://prowlarr.lan:9696/1/download?apikey=k&link=x"},
				{"title": "The.Matrix.1999.1080p.BluRay.AVC.DTS-HD.MA.5.1-FGT", "indexer": "B", "guid": "b-1", "seeders": 9, "size": 30 << 30,
					"categories": []map[string]any{{"id": 2050}}, "tmdbId": 603, "protocol": "torrent"},
				// Not discs, or another film, or an NZB.
				{"title": "The.Matrix.1999.2160p.BluRay.REMUX-X", "indexer": "A", "tmdbId": 603, "protocol": "torrent", "downloadUrl": "http://p/2"},
				{"title": "Other.2001.COMPLETE.BLURAY", "indexer": "A", "tmdbId": 999, "protocol": "torrent", "downloadUrl": "http://p/3"},
				{"title": "The.Matrix.1999.BD50", "indexer": "N", "tmdbId": 603, "protocol": "usenet", "downloadUrl": "http://p/4"},
			})
		case "/1/download":
			downloads = append(downloads, r)
			w.Write(disc)
		default:
			http.NotFound(w, r)
		}
	}))
	defer prow.Close()
	c := prowlarr.New(prow.URL, "k")
	c.MinInterval = 0
	srv := New(config.Config{}, st, nil, nil, nil, nil, nil, nil, fstest.MapFS{}, "test")
	srv.SetServices(Services{Best: prowlarr.NewFinder(st, c, 7*24*time.Hour)})
	h := srv.Handler()
	path := "/api/plan/" + itoa(e.ID) + "/discs"

	var got prowlarr.DiscSearch
	if code := call(t, h, "GET", path, nil, &got); code != http.StatusOK || got.CheckedAt != "" || got.Discs == nil || got.Searching {
		t.Fatalf("before %d %+v", code, got)
	}
	if code := call(t, h, "POST", path, nil, &got); code != http.StatusAccepted || !got.Searching {
		t.Fatalf("search %d %+v", code, got)
	}
	for deadline := time.Now().Add(5 * time.Second); got.Searching && time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		call(t, h, "GET", path, nil, &got)
	}
	if got.Searching || got.Error != "" || got.CheckedAt == "" || len(got.Discs) != 2 || strings.Join(searches, ",") != "500" {
		t.Fatalf("found %+v (searches %v)", got, searches)
	}
	uhd, bd := got.Discs[0], got.Discs[1]
	if uhd.Quality != "2160p Full Disc" || uhd.Indexer != "A" || uhd.URL != "https://a/1" || !uhd.Loadable ||
		bd.Resolution != "1080p" || bd.Loadable {
		t.Fatalf("discs %+v", got.Discs)
	}
	// The key of Prowlarr never reaches the user.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	if strings.Contains(rec.Body.String(), "apikey") || strings.Contains(rec.Body.String(), "download") {
		t.Fatalf("download address sent: %s", rec.Body.String())
	}

	var errOut map[string]string
	if code := call(t, h, "POST", path+"/"+bd.ID+"/load", nil, &errOut); code != http.StatusBadRequest || !strings.Contains(errOut["error"], "magnet") {
		t.Fatalf("magnet %d %v", code, errOut)
	}
	if code := call(t, h, "POST", path+"/nope/load", nil, &errOut); code != http.StatusNotFound {
		t.Fatalf("unknown disc %d %v", code, errOut)
	}
	var pt store.PlanTorrent
	if code := call(t, h, "POST", path+"/"+uhd.ID+"/load", nil, &pt); code != http.StatusCreated || pt.Hash != hash || pt.Position != 1 ||
		pt.Name != "The.Matrix.1999.COMPLETE.UHD.BLURAY-GRP" {
		t.Fatalf("load %d %+v", code, pt)
	}
	if len(downloads) != 1 || downloads[0].Header.Get("X-Api-Key") != "k" || downloads[0].URL.Query().Get("link") != "x" {
		t.Fatalf("downloads %+v", downloads)
	}
	// Loaded once only.
	if code := call(t, h, "POST", path+"/"+uhd.ID+"/load", nil, &errOut); code != http.StatusConflict {
		t.Fatalf("loaded twice %d %v", code, errOut)
	}
}
