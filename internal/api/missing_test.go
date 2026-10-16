package api

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/tehmaat/bookrr/internal/config"
	"github.com/tehmaat/bookrr/internal/gaps"
	"github.com/tehmaat/bookrr/internal/store"
	"github.com/tehmaat/bookrr/internal/unit3d"
)

func TestMissingSeriesDisabled(t *testing.T) {
	for _, tc := range []struct {
		cfg  config.Config
		want string
	}{
		{config.Config{}, "BOOKRR_UNIT3D_URL"},
		{config.Config{TMDBAPIKey: "k"}, "BOOKRR_UNIT3D_URL"},
		{config.Config{Unit3DURL: "https://t.example", Unit3DAPIKey: "k", MissingCheckAt: 4 * 60}, "BOOKRR_TMDB_API_KEY"},
	} {
		st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
		if err != nil {
			t.Fatal(err)
		}
		h := New(tc.cfg, st, nil, nil, nil, nil, nil, fstest.MapFS{}, "test").Handler()
		var out struct {
			Enabled        bool    `json:"enabled"`
			DisabledReason string  `json:"disabledReason"`
			CheckedAt      *string `json:"checkedAt"`
			CheckAt        string  `json:"checkAt"`
			Series         []any   `json:"series"`
		}
		if code := call(t, h, "GET", "/api/missing/series", nil, &out); code != http.StatusOK || out.Enabled || out.Series == nil ||
			out.CheckedAt != nil || !strings.Contains(out.DisabledReason, tc.want) {
			t.Errorf("%+v: code %d, %+v", tc.cfg, code, out)
		}
		if tc.cfg.MissingCheckAt != 0 && out.CheckAt != "04:00" {
			t.Errorf("check at %q", out.CheckAt)
		}
		var sagas struct {
			Enabled bool  `json:"enabled"`
			Sagas   []any `json:"sagas"`
		}
		if code := call(t, h, "GET", "/api/missing/sagas", nil, &sagas); code != http.StatusOK || sagas.Enabled || sagas.Sagas == nil {
			t.Errorf("%+v: sagas code %d, %+v", tc.cfg, code, sagas)
		}
		if code := call(t, h, "POST", "/api/missing/check", nil, nil); code != http.StatusBadRequest {
			t.Errorf("check while disabled: %d", code)
		}
		st.Close()
	}
}

// The minimum quality and the user's choices are kept and applied to the
// stored results when read.
func TestMissingMarks(t *testing.T) {
	ctx := context.Background()
	st, h := newServer(t)
	q := unit3d.QualityOf("Saga.2001.1080p.BluRay.REMUX.ITA.ENG", "", "", "")
	enc := unit3d.QualityOf("Saga.2.2003.1080p.BluRay.x264.ITA", "Encode", "", "")
	g := gaps.Group{Format: gaps.Format, Kind: gaps.KindSaga, Key: "collection:1", Title: "Saga", Status: gaps.StatusOK,
		Units: []gaps.Unit{
			{Number: 1, Name: "Uno", TmdbID: 11, Versions: []gaps.Version{{Ref: gaps.Ref{ID: 1, Mine: true}, Q: q}}},
			{Number: 2, Name: "Due", TmdbID: 12, Versions: []gaps.Version{{Ref: gaps.Ref{ID: 2}, Q: enc}}},
		},
		Rows: []gaps.Row{{Quality: q, Missing: 1, OtherQuality: 1, Cells: []gaps.Cell{
			{Unit: 1, State: gaps.StateMine}, {Unit: 2, State: gaps.StateOtherQuality, Torrents: []gaps.Ref{{ID: 2}}},
		}}}}
	data, _ := json.Marshal(g)
	if err := st.SaveGapGroups(ctx, gaps.KindSaga, []store.GapGroup{{Key: g.Key, Title: g.Title, Data: data}}); err != nil {
		t.Fatal(err)
	}
	type result struct {
		Prefs store.GapPrefs `json:"prefs"`
		Sagas []gaps.Group   `json:"sagas"`
	}
	get := func() gaps.Group {
		t.Helper()
		var out result
		if code := call(t, h, "GET", "/api/missing/sagas", nil, &out); code != http.StatusOK || len(out.Sagas) != 1 {
			t.Fatalf("get: %d %+v", code, out)
		}
		return out.Sagas[0]
	}
	if got := get(); got.PartTotals.Missing != 1 || got.PartTotals.Available != 1 || got.Missing != 1 {
		t.Fatalf("no minimum: %+v, %d", got.PartTotals, got.Missing)
	}

	if code := call(t, h, "PUT", "/api/missing/sagas/prefs", map[string]any{"type": "Bluray"}, nil); code != http.StatusBadRequest {
		t.Errorf("bad type: %d", code)
	}
	if code := call(t, h, "PUT", "/api/missing/films/prefs", map[string]any{}, nil); code != http.StatusNotFound {
		t.Errorf("bad kind: %d", code)
	}
	var prefs store.GapPrefs
	if code := call(t, h, "PUT", "/api/missing/sagas/prefs", map[string]any{"type": "remux", "lang": "ita"}, &prefs); code != http.StatusOK ||
		prefs.Type != "Remux" || prefs.Lang != "ITA" {
		t.Fatalf("prefs: %d %+v", code, prefs)
	}
	if got := get(); got.PartTotals.Below != 1 || got.Parts[1].State != gaps.PartBelow {
		t.Fatalf("remux at least: %+v", got.Parts)
	}

	accept := map[string]any{"group": g.Key, "action": "accept", "unit": 12, "torrent": 2}
	if code := call(t, h, "POST", "/api/missing/sagas/marks", accept, nil); code != http.StatusOK {
		t.Fatalf("accept: %d", code)
	}
	if got := get(); got.PartTotals.Missing != 0 || got.Parts[1].Mark != gaps.MarkAccepted || got.Missing != 0 {
		t.Fatalf("accepted: %+v, %d", got.Parts, got.Missing)
	}
	// Ignoring the part replaces the acceptance.
	if code := call(t, h, "POST", "/api/missing/sagas/marks", map[string]any{"group": g.Key, "action": "ignore", "unit": 12}, nil); code != http.StatusOK {
		t.Fatalf("ignore: %d", code)
	}
	if marks, _ := st.GapMarks(ctx, gaps.KindSaga); len(marks) != 1 || marks[0].Action != "ignore" {
		t.Fatalf("marks: %+v", marks)
	}

	for _, m := range []map[string]any{
		{"group": g.Key, "action": "hide"},
		{"group": g.Key, "action": "complete"},
		{"group": g.Key, "action": "threshold", "threshold": map[string]string{"type": "Encode"}},
	} {
		if code := call(t, h, "POST", "/api/missing/sagas/marks", m, nil); code != http.StatusOK {
			t.Fatalf("%v: %d", m, code)
		}
	}
	if got := get(); !got.Hidden || !got.Completed || !got.CustomThreshold || got.Threshold.Type != "Encode" {
		t.Fatalf("hidden, complete, own minimum: %+v", got)
	}
	if code := call(t, h, "DELETE", "/api/missing/sagas/marks", map[string]any{"group": g.Key, "action": "hide"}, nil); code != http.StatusNoContent {
		t.Fatalf("unhide: %d", code)
	}
	if got := get(); got.Hidden {
		t.Fatal("still hidden")
	}

	for _, bad := range []map[string]any{
		{"group": g.Key, "action": "accept", "unit": 12},
		{"group": g.Key, "action": "nope"},
		{"action": "hide"},
		{"group": g.Key, "action": "threshold"},
	} {
		if code := call(t, h, "POST", "/api/missing/sagas/marks", bad, nil); code != http.StatusBadRequest {
			t.Errorf("%v: %d", bad, code)
		}
	}
	if code := call(t, h, "POST", "/api/missing/sagas/marks", map[string]any{"group": "collection:9", "action": "hide"}, nil); code != http.StatusNotFound {
		t.Errorf("unknown group: %d", code)
	}
}
