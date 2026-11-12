package wayfinderr

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func server(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return New(srv.URL+"/", " key ")
}

func TestPing(t *testing.T) {
	ctx := context.Background()
	status := `{"configured":true,"username":"admin"}`
	sources := http.StatusOK
	c := server(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "key" {
			t.Errorf("key %q", r.Header.Get("X-Api-Key"))
		}
		switch r.URL.Path {
		case "/api/auth/status":
			w.Write([]byte(status))
		case "/api/source-torrents":
			w.WriteHeader(sources)
			w.Write([]byte(`[]`))
		}
	})
	if user, err := c.Ping(ctx); err != nil || user != "admin" {
		t.Fatalf("ok: %q %v", user, err)
	}
	status = `{"configured":true,"username":null}`
	if _, err := c.Ping(ctx); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("wrong key: %v", err)
	}
	status = `{"configured":false,"username":null}`
	if _, err := c.Ping(ctx); err == nil || !strings.Contains(err.Error(), "no account") {
		t.Fatalf("no account: %v", err)
	}
	status = `<html>`
	if _, err := c.Ping(ctx); err == nil || !strings.Contains(err.Error(), "does not answer like Wayfinderr") {
		t.Fatalf("another app: %v", err)
	}
	status, sources = `{"configured":true,"username":"admin"}`, http.StatusNotFound
	if _, err := c.Ping(ctx); err == nil || !strings.Contains(err.Error(), "too old") {
		t.Fatalf("old Wayfinderr: %v", err)
	}
}

func TestSourceTorrents(t *testing.T) {
	ctx := context.Background()
	c := server(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost:
			q := r.URL.Query()
			if r.Header.Get("Content-Type") != "application/x-bittorrent" || q.Get("origin") != "bookrr" ||
				q.Get("name") != "Matrix.torrent" || q.Get("tmdbId") != "603" || q.Get("label") != "" {
				t.Errorf("post %v %v", r.Header, q)
			}
			if q.Get("name") == "bad.torrent" {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"error":"Not a .torrent file"}`))
				return
			}
			w.WriteHeader(http.StatusCreated)
		case r.URL.Path == "/api/source-torrents/abc":
			w.Write([]byte(`{}`))
		case r.URL.Path == "/api/source-torrents/abc/file":
			w.Write([]byte("d8:announce"))
		case r.URL.Path == "/api/source-torrents/err":
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error":"Failed"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	if err := c.AddSourceTorrent(ctx, SourceTorrent{FileName: "Matrix.torrent", Data: []byte("x"), TmdbID: 603}); err != nil {
		t.Fatal(err)
	}
	if has, err := c.HasSourceTorrent(ctx, "ABC"); !has || err != nil {
		t.Fatalf("has: %v %v", has, err)
	}
	if has, err := c.HasSourceTorrent(ctx, "def"); has || err != nil {
		t.Fatalf("has not: %v %v", has, err)
	}
	if _, err := c.HasSourceTorrent(ctx, "err"); err == nil || err.Error() != "Wayfinderr: Failed (HTTP 500)" {
		t.Fatalf("error: %v", err)
	}
	if data, err := c.SourceTorrentFile(ctx, "abc"); err != nil || string(data) != "d8:announce" {
		t.Fatalf("file: %q %v", data, err)
	}
	if _, err := c.SourceTorrentFile(ctx, "def"); err == nil {
		t.Fatal("missing file")
	}
}

func TestMergePlans(t *testing.T) {
	ctx := context.Background()
	var posted map[string]any
	planned := false
	c := server(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/merge/plans":
			if r.Header.Get("Content-Type") != "application/json" {
				t.Errorf("content type %q", r.Header.Get("Content-Type"))
			}
			json.NewDecoder(r.Body).Decode(&posted)
			if planned {
				w.Write([]byte(`{"id":"p1","status":"WAITING","pairId":null,"message":null}`))
				return
			}
			planned = true
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id":"p1","status":"WAITING","pairId":null,"message":null}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/merge/plans/p1":
			w.Write([]byte(`{"id":"p1","status":"PAIRED","pairId":"pair9","message":null}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/merge/plans/p1/pair":
			w.Write([]byte(`{"id":"p1","status":"ERROR","pairId":null,"message":"file gone"}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/api/merge/plans/p1":
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/api/merge/plans/busy":
			w.WriteHeader(http.StatusConflict)
			w.Write([]byte(`{"error":"Merging is off"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":"Planned pair not found"}`))
		}
	})
	plan := MergePlan{TmdbID: 603, Files: [2]MergeSide{{InfoHash: "aaa", Name: "Matrix.UHD", Label: "video"}, {InfoHash: "bbb"}}}
	p, created, err := c.PlanMerge(ctx, plan)
	if err != nil || !created || p.ID != "p1" || p.Status != MergeWaiting {
		t.Fatalf("planned: %+v %v %v", p, created, err)
	}
	files, _ := posted["files"].([]any)
	first, _ := files[0].(map[string]any)
	second, _ := files[1].(map[string]any)
	if posted["origin"] != "bookrr" || posted["tmdbId"] != float64(603) || len(files) != 2 ||
		first["infoHash"] != "aaa" || first["name"] != "Matrix.UHD" || first["label"] != "video" ||
		second["infoHash"] != "bbb" || second["name"] != nil || second["label"] != nil {
		t.Fatalf("posted %v", posted)
	}
	if _, created, err := c.PlanMerge(ctx, plan); err != nil || created {
		t.Fatalf("planned again: %v %v", created, err)
	}
	if p, err := c.MergePlanOf(ctx, "p1"); err != nil || p.Status != MergePaired || *p.PairID != "pair9" {
		t.Fatalf("read: %+v %v", p, err)
	}
	if _, err := c.MergePlanOf(ctx, "gone"); !errors.Is(err, ErrPlanNotFound) {
		t.Fatalf("gone: %v", err)
	}
	if _, err := c.MergePlanOf(ctx, "busy"); err == nil || err.Error() != "Wayfinderr: Merging is off (HTTP 409)" {
		t.Fatalf("refused: %v", err)
	}
	if p, err := c.PairMerge(ctx, "p1"); err != nil || p.Status != MergeError || *p.Message != "file gone" {
		t.Fatalf("pair again: %+v %v", p, err)
	}
	if err := c.DeleteMergePlan(ctx, "p1"); err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteMergePlan(ctx, "gone"); err != nil {
		t.Fatalf("delete gone: %v", err)
	}
	if err := c.DeleteMergePlan(ctx, "busy"); err == nil {
		t.Fatal("delete refused")
	}

	// A Wayfinderr before 0.29.0.
	old := server(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) })
	if _, _, err := old.PlanMerge(ctx, plan); !errors.Is(err, ErrMergeUnsupported) {
		t.Fatalf("old: %v", err)
	}
}

func TestSameWayfinderr(t *testing.T) {
	if !SameWayfinderr("http://WF:3000/", " http://wf:3000") || SameWayfinderr("", "") || SameWayfinderr("http://a", "http://b") {
		t.Fatal("SameWayfinderr")
	}
}
