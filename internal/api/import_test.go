package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/tehmaat/bookrr/internal/config"
	"github.com/tehmaat/bookrr/internal/store"
)

func newServer(t *testing.T) (*store.Store, http.Handler) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st, New(config.Config{}, st, nil, nil, fstest.MapFS{}, "test").Handler()
}

func call(t *testing.T, h http.Handler, method, path string, body any, out any) int {
	t.Helper()
	b, _ := json.Marshal(body)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, bytes.NewReader(b)))
	if out != nil {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			t.Fatalf("%s %s: %v (%s)", method, path, err, rec.Body.String())
		}
	}
	return rec.Code
}

type importResult struct {
	Rows []struct {
		Hash  string `json:"hash"`
		Error string `json:"error"`
	} `json:"rows"`
	Imported int `json:"imported"`
}

// The same input must produce the same stored torrent whether it comes from
// the "Aggiungi" form or from the bulk import.
func TestImportStoresSameDataAsForm(t *testing.T) {
	ctx := context.Background()
	st, h := newServer(t)
	disk, err := st.CreateDisk(ctx, store.Disk{Label: "Archivio 1", Kind: "HDD", Serial: "SN42"})
	if err != nil {
		t.Fatal(err)
	}
	adopter, err := st.CreateAdopter(ctx, store.Adopter{Name: "Mario"})
	if err != nil {
		t.Fatal(err)
	}
	item := func(hash string) map[string]any {
		return map[string]any{
			"hash": " " + hash + " ", "name": " La.Mia.Release ", "size": 1234,
			"tags": []string{"Personal Release", " x ", ""}, "personalRelease": true, "notes": "nota",
			"archive":  map[string]any{"diskId": disk.ID, "path": "/Release/2024", "notes": "scatola 3"},
			"adoption": map[string]any{"adopterId": adopter.ID, "notes": ""},
		}
	}
	formHash := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	importHash := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	if code := call(t, h, "POST", "/api/torrents", item(formHash), nil); code != http.StatusCreated {
		t.Fatalf("form: %d", code)
	}
	var res importResult
	if code := call(t, h, "POST", "/api/torrents/import", map[string]any{"items": []any{item(importHash)}}, &res); code != http.StatusCreated {
		t.Fatalf("import: %d %+v", code, res)
	}
	if res.Imported != 1 {
		t.Fatalf("imported %d", res.Imported)
	}

	a, err := st.GetTorrent(ctx, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatal(err)
	}
	b, err := st.GetTorrent(ctx, importHash)
	if err != nil {
		t.Fatal(err)
	}
	if a.Status != store.StatusArchived || len(a.Archives) != 1 || len(a.Adoptions) != 1 {
		t.Fatalf("unexpected form torrent: %+v", a)
	}
	if got, want := comparable(b), comparable(a); got != want {
		t.Fatalf("import differs from form:\n form   %s\n import %s", want, got)
	}
}

// comparable drops what legitimately differs between two creations: the
// hash, row ids and timestamps.
func comparable(t store.Torrent) string {
	t.Hash, t.FirstSeenAt, t.UpdatedAt = "", "", ""
	for i := range t.Archives {
		t.Archives[i].ID, t.Archives[i].CreatedAt = 0, ""
	}
	for i := range t.Adoptions {
		t.Adoptions[i].ID, t.Adoptions[i].CreatedAt = 0, ""
	}
	b, _ := json.Marshal(t)
	return string(b)
}

func TestImportIsAllOrNothing(t *testing.T) {
	ctx := context.Background()
	st, h := newServer(t)
	disk, err := st.CreateDisk(ctx, store.Disk{Label: "D", Kind: "HDD"})
	if err != nil {
		t.Fatal(err)
	}
	existing := "cccccccccccccccccccccccccccccccccccccccc"
	if _, err := st.CreateManualTorrent(ctx, store.ManualInput{TorrentInput: store.TorrentInput{Hash: existing, Name: "Già qui"}}); err != nil {
		t.Fatal(err)
	}
	ok := "dddddddddddddddddddddddddddddddddddddddd"
	arch := map[string]any{"diskId": disk.ID}
	items := []any{
		map[string]any{"name": "Buono", "hash": ok, "archive": arch},
		map[string]any{"name": "Ripetuto", "hash": ok, "archive": arch},
		map[string]any{"name": "Esistente", "hash": existing, "archive": arch},
		map[string]any{"name": "Senza archivio"},
		map[string]any{"name": "Hash rotto", "hash": "xyz", "archive": arch},
		map[string]any{"name": "Disco fantasma", "archive": map[string]any{"diskId": 999}},
		map[string]any{"name": "", "archive": arch},
	}
	var res importResult
	if code := call(t, h, "POST", "/api/torrents/import", map[string]any{"items": items}, &res); code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", code)
	}
	for i, r := range res.Rows {
		if (i == 0) != (r.Error == "") {
			t.Errorf("row %d: unexpected error %q", i, r.Error)
		}
	}
	if found, _ := st.TorrentExists(ctx, ok); found {
		t.Fatal("nothing must be written when a row fails")
	}

	// Dry run of the valid row: checked but not written.
	if code := call(t, h, "POST", "/api/torrents/import", map[string]any{"items": items[:1], "dryRun": true}, &res); code != 200 {
		t.Fatalf("dry run: %d", code)
	}
	if found, _ := st.TorrentExists(ctx, ok); found {
		t.Fatal("dry run must not write")
	}
}
