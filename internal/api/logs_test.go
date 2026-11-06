package api

import (
	"bytes"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/tehmaat/bookrr/internal/config"
	"github.com/tehmaat/bookrr/internal/logs"
	"github.com/tehmaat/bookrr/internal/store"
)

func TestLogs(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	w, err := logs.New(filepath.Join(t.TempDir(), "logs"))
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	log := slog.New(logs.Handler(w, &bytes.Buffer{}))
	log.Info("sync completato", "ok", 1)
	log.Warn("client non raggiungibile", "client", "nas")

	srv := New(config.Config{}, st, nil, nil, nil, nil, nil, nil, fstest.MapFS{}, "test")
	// Without log files.
	h := srv.Handler()
	var files struct {
		Enabled bool        `json:"enabled"`
		Files   []logs.File `json:"files"`
		Today   string      `json:"today"`
	}
	if code := call(t, h, "GET", "/api/logs/files", nil, &files); code != 200 || files.Enabled {
		t.Fatalf("disabled: %d %+v", code, files)
	}
	if code := call(t, h, "GET", "/api/logs", nil, nil); code != 404 {
		t.Fatalf("disabled read: %d", code)
	}

	srv.Logs = w
	h = srv.Handler()
	if code := call(t, h, "GET", "/api/logs/files", nil, &files); code != 200 || !files.Enabled || len(files.Files) != 1 || files.Files[0].Day != files.Today {
		t.Fatalf("files: %d %+v", code, files)
	}
	var res logs.Result
	if code := call(t, h, "GET", "/api/logs?level=warn", nil, &res); code != 200 || len(res.Entries) != 1 || res.Entries[0].Attrs["client"] != "nas" {
		t.Fatalf("read: %d %+v", code, res)
	}
	log.Error("nuovo")
	var next logs.Result
	if code := call(t, h, "GET", "/api/logs?day="+files.Today+"&after="+strconv.FormatInt(res.Next, 10), nil, &next); code != 200 || len(next.Entries) != 1 || next.Entries[0].Msg != "nuovo" {
		t.Fatalf("after: %d %+v", code, next)
	}
	for _, q := range []string{"?day=ieri", "?level=loud", "?after=-1"} {
		if code := call(t, h, "GET", "/api/logs"+q, nil, nil); code != 400 {
			t.Errorf("%s: %d", q, code)
		}
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/logs/files/"+files.Files[0].Name, nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"msg":"nuovo"`) || !strings.Contains(rec.Header().Get("Content-Disposition"), files.Files[0].Name) {
		t.Fatalf("download: %d %s %v", rec.Code, rec.Body.String(), rec.Header())
	}
	for _, p := range []string{"/api/logs/files/..%2Ftest.db", "/api/logs/files/bookrr-2020-01-01.log", "/api/logs/files/test.db"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", p, nil))
		if rec.Code != 404 {
			t.Errorf("%s: %d", p, rec.Code)
		}
	}
}
