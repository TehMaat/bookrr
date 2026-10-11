package qbit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDeleteTorrentLogsInAndPostsForm(t *testing.T) {
	var got struct{ hashes, deleteFiles string }
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "ok", Path: "/"})
			w.Write([]byte("Ok."))
		case "/api/v2/torrents/delete":
			if c, err := r.Cookie("SID"); err != nil || c.Value != "ok" {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			got.hashes, got.deleteFiles = r.FormValue("hashes"), r.FormValue("deleteFiles")
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	if err := New(srv.URL, "u", "p", false).DeleteTorrent(context.Background(), "abc", true); err != nil {
		t.Fatal(err)
	}
	if got.hashes != "abc" || got.deleteFiles != "true" {
		t.Fatalf("unexpected form: %+v", got)
	}
}

func TestRemoveTagsPostsHashesAndTags(t *testing.T) {
	var got struct{ hashes, tags string }
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/torrents/removeTags" || r.Method != http.MethodPost {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		got.hashes, got.tags = r.FormValue("hashes"), r.FormValue("tags")
	}))
	defer srv.Close()

	if err := New(srv.URL, "", "", false).RemoveTags(context.Background(), []string{"a", "b"}, []string{"Personal Release", "personal release"}); err != nil {
		t.Fatal(err)
	}
	if got.hashes != "a|b" || got.tags != "Personal Release,personal release" {
		t.Fatalf("unexpected form: %+v", got)
	}
}
