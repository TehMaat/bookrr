package wayfinderr

import (
	"context"
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
