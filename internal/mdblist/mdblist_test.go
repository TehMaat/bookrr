package mdblist

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestParseURL(t *testing.T) {
	for in, want := range map[string]string{
		"https://mdblist.com/lists/garycrawfordgc/marvel-cinematic-universe":      "garycrawfordgc/marvel-cinematic-universe",
		"https://www.mdblist.com/lists/garycrawfordgc/marvel-cinematic-universe/": "garycrawfordgc/marvel-cinematic-universe",
		"https://mdblist.com/lists/user/pixar/json":                               "user/pixar",
		"https://mdblist.com/lists/user/pixar?sort=rank":                          "user/pixar",
		" user/disney-classics ":                                                  "user/disney-classics",
		"https://mdblist.com/lists/user/external/12345":                           "user/external/12345",
		"mdblist.com/lists/user/pixar":                                            "",
		"https://example.com/lists/user/pixar":                                    "",
		"https://mdblist.com/lists/user":                                          "",
		"https://mdblist.com/lists/official/movies/top":                           "",
		"https://mdblist.com/lists/user/pixar/extra":                              "",
		"../..":          "",
		"user/../../etc": "",
	} {
		got, err := ParseURL(in)
		if want == "" {
			if err == nil {
				t.Errorf("%q: %q, want an error", in, got)
			}
			continue
		}
		if err != nil || got != want {
			t.Errorf("%q: %q %v, want %q", in, got, err, want)
		}
	}
}

func newClient(t *testing.T, h http.HandlerFunc) *Client {
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := New("KEY")
	c.BaseURL = srv.URL
	c.MinInterval = 0
	return c
}

func TestItems(t *testing.T) {
	var asked []string
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("apikey") != "KEY" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		asked = append(asked, r.URL.Path+"?offset="+r.URL.Query().Get("offset"))
		switch r.URL.Path + "/" + r.URL.Query().Get("offset") {
		case "/lists/user/mcu/items/movie/0":
			w.Header().Set("X-Has-More", "true")
			w.Write([]byte(`{"movies":[
				{"id":1726,"rank":1,"title":"Iron Man","imdb_id":"tt0371746","mediatype":"movie","release_year":2008},
				{"id":10138,"rank":3,"title":"Iron Man 2","imdb_id":"tt1228705","mediatype":"movie","release_year":2010}],"shows":[]}`))
		case "/lists/user/mcu/items/movie/2":
			w.Write([]byte(`{"movies":[
				{"id":1724,"rank":2,"title":"L'incredibile Hulk","imdb_id":"tt0800080","mediatype":"movie"},
				{"id":null,"rank":4,"title":"Senza TMDB","imdb_id":"tt0000001","mediatype":"movie"}],"shows":[]}`))
		case "/external/lists/77/items//0":
			w.Write([]byte(`[{"id":10,"rank":0,"title":"A","mediatype":"movie"},{"id":20,"rank":1,"title":"Serie","mediatype":"show"}]`))
		default:
			http.NotFound(w, r)
		}
	})
	ctx := context.Background()
	items, err := c.Items(ctx, "user/mcu")
	if err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for _, it := range items {
		ids = append(ids, it.TmdbID)
	}
	// In the order of the list, across the pages.
	if !reflect.DeepEqual(ids, []int64{1726, 1724, 10138, 0}) || items[3].Imdb() != 1 || items[0].Title != "Iron Man" {
		t.Errorf("items %+v", items)
	}
	if !reflect.DeepEqual(asked, []string{"/lists/user/mcu/items/movie?offset=0", "/lists/user/mcu/items/movie?offset=2"}) {
		t.Errorf("asked %v", asked)
	}

	// Lists mirrored from elsewhere: the films only.
	items, err = c.Items(ctx, "someone/external/77")
	if err != nil || len(items) != 1 || items[0].TmdbID != 10 {
		t.Errorf("external: %+v %v", items, err)
	}

	if _, err := c.Items(ctx, "user/gone"); !errors.Is(err, ErrNotFound) {
		t.Errorf("gone: %v", err)
	}
}

func TestList(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/lists/user/mcu":
			w.Write([]byte(`[{"id":5,"name":"Marvel Cinematic Universe","slug":"mcu","items":34,"user_name":"user"}]`))
		case "/lists/user/empty":
			w.Write([]byte(`[]`))
		case "/lists/user/error":
			w.Write([]byte(`{"response":false,"error":"List not found"}`))
		case "/lists/user/limit":
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error":"API Limit Reached!"}`))
		case "/lists/user/key":
			w.WriteHeader(http.StatusForbidden)
		}
	})
	ctx := context.Background()
	l, err := c.List(ctx, "user/mcu")
	if err != nil || l.Name != "Marvel Cinematic Universe" || l.Items != 34 {
		t.Errorf("%+v %v", l, err)
	}
	for _, p := range []string{"user/empty", "user/error"} {
		if _, err := c.List(ctx, p); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s: %v", p, err)
		}
	}
	if _, err := c.List(ctx, "user/limit"); err == nil || !strings.Contains(err.Error(), "limite") {
		t.Errorf("limit: %v", err)
	}
	if _, err := c.List(ctx, "user/key"); err == nil || !strings.Contains(err.Error(), "BOOKRR_MDBLIST_API_KEY") {
		t.Errorf("key: %v", err)
	}
}
