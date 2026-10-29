package api

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/tehmaat/bookrr/internal/config"
	"github.com/tehmaat/bookrr/internal/store"
)

func TestAcceptsGzip(t *testing.T) {
	for header, want := range map[string]bool{
		"":                        false,
		"gzip":                    true,
		"gzip, deflate, br, zstd": true,
		"br;q=1.0, GZIP;q=0.5":    true,
		"gzip;q=0":                false,
		"gzip; q=0.000":           false,
		"deflate":                 false,
	} {
		if got := acceptsGzip(header); got != want {
			t.Errorf("acceptsGzip(%q) = %v, want %v", header, got, want)
		}
	}
}

// The API and the scripts of the web UI are compressed for the browsers
// that ask for it; tiny files, byte ranges and other clients get them as
// they are.
func TestCompressedResponses(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	script := strings.Repeat("console.log('bookrr');\n", 200)
	web := fstest.MapFS{
		"index.html":       {Data: []byte("<!doctype html><title>bookrr</title>")},
		"assets/app-1.js":  {Data: []byte(script)},
		"assets/logo.webp": {Data: []byte(strings.Repeat("x", 4096))},
	}
	h := New(config.Config{}, st, nil, nil, nil, nil, nil, web, "test").Handler()

	get := func(path string, header map[string]string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", path, nil)
		for k, v := range header {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	gunzip := func(rec *httptest.ResponseRecorder) string {
		t.Helper()
		if rec.Header().Get("Content-Encoding") != "gzip" || rec.Header().Get("Content-Length") != "" {
			t.Fatalf("not compressed: %v", rec.Header())
		}
		r, err := gzip.NewReader(rec.Body)
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(r)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	accept := map[string]string{"Accept-Encoding": "gzip, deflate"}

	rec := get("/api/torrents", accept)
	if body := gunzip(rec); rec.Code != 200 || strings.TrimSpace(body) != "[]" || rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("api: %d %q %v", rec.Code, body, rec.Header())
	}
	rec = get("/assets/app-1.js", accept)
	if body := gunzip(rec); body != script || rec.Body.Len() > len(script)/4 {
		t.Fatalf("script: %d bytes compressed", rec.Body.Len())
	}
	if rec.Header().Get("Vary") != "Accept-Encoding" || !strings.Contains(rec.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("headers: %v", rec.Header())
	}

	for name, rec := range map[string]*httptest.ResponseRecorder{
		"no gzip":  get("/assets/app-1.js", nil),
		"range":    get("/assets/app-1.js", map[string]string{"Accept-Encoding": "gzip", "Range": "bytes=0-9"}),
		"tiny":     get("/", accept),
		"not text": get("/assets/logo.webp", accept),
	} {
		if rec.Header().Get("Content-Encoding") != "" {
			t.Errorf("%s: compressed", name)
		}
		if name == "range" {
			if rec.Code != http.StatusPartialContent || rec.Body.String() != script[:10] {
				t.Errorf("range: %d %q", rec.Code, rec.Body.String())
			}
		} else if rec.Code != 200 {
			t.Errorf("%s: status %d", name, rec.Code)
		}
	}
}
