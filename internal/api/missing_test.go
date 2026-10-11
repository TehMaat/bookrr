package api

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/tehmaat/bookrr/internal/config"
	"github.com/tehmaat/bookrr/internal/store"
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
		if code := call(t, h, "POST", "/api/missing/check", nil, nil); code != http.StatusBadRequest {
			t.Errorf("check while disabled: %d", code)
		}
		st.Close()
	}
}
