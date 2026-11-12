package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/tehmaat/bookrr/internal/config"
	"github.com/tehmaat/bookrr/internal/store"
	"github.com/tehmaat/bookrr/internal/syncer"
)

func TestPlanMerge(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	sy := syncer.New(st, "mine", time.Hour)
	h := New(config.Config{}, st, sy, nil, nil, nil, nil, nil, fstest.MapFS{}, "test").Handler()

	// A Wayfinderr that plans every pair as a new one, and forgets them.
	var mu sync.Mutex
	var planned []map[string]any
	removed := 0
	wf := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/merge/plans":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			planned = append(planned, body)
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id":"p` + itoa(int64(len(planned))) + `","status":"WAITING","pairId":null,"message":null}`))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/merge/plans/"):
			w.Write([]byte(`{"id":"` + strings.TrimPrefix(r.URL.Path, "/api/merge/plans/") + `","status":"WAITING"}`))
		case r.Method == http.MethodDelete:
			removed++
			w.WriteHeader(http.StatusNoContent)
		default:
			// Source .torrent files are taken and forgotten.
			w.WriteHeader(http.StatusCreated)
		}
	}))
	defer wf.Close()
	q := &fakeQbit{}
	qs := q.serve(t)
	c, err := st.CreateClient(ctx, store.Client{Name: "ripper", URL: qs.URL, Enabled: true, WayfinderrURL: wf.URL, WayfinderrAPIKey: "key"})
	if err != nil {
		t.Fatal(err)
	}
	e, err := st.AddPlanEntry(ctx, store.PlanEntry{TmdbID: 603, Title: "Matrix"})
	if err != nil {
		t.Fatal(err)
	}
	base := "/api/plan/" + itoa(e.ID)
	video, videoHash := torrentOf("Matrix.Video", 1000)
	audio, _ := torrentOf("Matrix.Audio", 200)
	upload(t, h, "POST", base+"/torrents", video, "video", nil)

	var errOut map[string]string
	if code := call(t, h, "POST", base+"/merge-plan", nil, &errOut); code != http.StatusBadRequest ||
		!strings.Contains(errOut["error"], "two .torrent") {
		t.Fatalf("one torrent: %d %v", code, errOut)
	}
	upload(t, h, "POST", base+"/torrents", audio, "audio ITA", nil)

	// Sending the two plans their pair at once.
	var sent struct {
		Results []planSendResult `json:"results"`
		Merge   *store.PlanMerge `json:"merge"`
	}
	if code := call(t, h, "POST", base+"/torrents/send", planSendInput{ClientID: c.ID}, &sent); code != http.StatusOK ||
		sent.Merge == nil || sent.Merge.RemoteID != "p1" || sent.Merge.Status != "WAITING" || sent.Merge.ClientName != "ripper" {
		t.Fatalf("send: %d %+v", code, sent.Merge)
	}
	mu.Lock()
	files, _ := planned[0]["files"].([]any)
	first, _ := files[0].(map[string]any)
	if planned[0]["tmdbId"] != float64(603) || first["infoHash"] != videoHash || first["name"] != "Matrix.Video" || first["label"] != "video" {
		t.Fatalf("planned: %v", planned[0])
	}
	mu.Unlock()

	var list struct {
		Entries []struct {
			Merge *store.PlanMerge `json:"merge"`
		} `json:"entries"`
	}
	if code := call(t, h, "GET", "/api/plan", nil, &list); code != http.StatusOK || list.Entries[0].Merge == nil ||
		list.Entries[0].Merge.RemoteID != "p1" {
		t.Fatalf("list: %d %+v", code, list)
	}

	var m store.PlanMerge
	if code := call(t, h, "POST", base+"/merge-plan/pair", nil, &errOut); code != http.StatusBadRequest {
		t.Fatalf("pair again, not in error: %d %v", code, errOut)
	}
	if code := call(t, h, "DELETE", base+"/merge-plan", nil, &m); code != http.StatusOK || m.Status != store.MergeRemoved || removed != 1 {
		t.Fatalf("remove: %d %+v %d", code, m, removed)
	}
	if code := call(t, h, "POST", base+"/merge-plan", map[string]int64{"clientId": c.ID}, &m); code != http.StatusOK ||
		m.RemoteID != "p2" || m.Status != "WAITING" {
		t.Fatalf("by hand: %d %+v", code, m)
	}
	if code := call(t, h, "POST", "/api/plan/999/merge-plan", nil, nil); code != http.StatusNotFound {
		t.Fatalf("unknown film: %d", code)
	}

	// The film removed: its plan goes from Wayfinderr.
	if code := call(t, h, "DELETE", base, nil, nil); code != http.StatusNoContent || removed != 2 {
		t.Fatalf("film removed: %d %d", code, removed)
	}
}
