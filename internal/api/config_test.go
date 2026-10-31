package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/tehmaat/bookrr/internal/config"
	"github.com/tehmaat/bookrr/internal/store"
)

func TestConfig(t *testing.T) {
	t.Setenv("BOOKRR_RELEASE_TAG", "Mine")
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	srv := New(config.Config{}, st, nil, nil, nil, nil, nil, nil, fstest.MapFS{}, "test")
	var applied []config.Config
	srv.Reconfigure = func(cfg config.Config) Services {
		applied = append(applied, cfg)
		return Services{Config: cfg}
	}
	h := srv.Handler()
	type option struct {
		Key    string `json:"key"`
		Value  string `json:"value"`
		Set    bool   `json:"set"`
		Locked bool   `json:"locked"`
	}
	var out struct{ Options []option }
	get := func(key string) option {
		for _, o := range out.Options {
			if o.Key == key {
				return o
			}
		}
		t.Fatalf("no %s in %+v", key, out.Options)
		return option{}
	}

	save := map[string]any{"values": map[string]any{
		"BOOKRR_UNIT3D_URL": "https://tracker.example", "BOOKRR_UNIT3D_API_KEY": "secret", "BOOKRR_RELEASE_TAG": "Other",
	}}
	if code := call(t, h, "PUT", "/api/config", save, &out); code != 200 {
		t.Fatalf("status %d", code)
	}
	if o := get("BOOKRR_UNIT3D_API_KEY"); o.Value != "" || !o.Set {
		t.Fatalf("secret sent back: %+v", o)
	}
	if o := get("BOOKRR_RELEASE_TAG"); o.Value != "Mine" || !o.Locked {
		t.Fatalf("environment: %+v", o)
	}
	if len(applied) != 1 || applied[0].Unit3DAPIKey != "secret" || applied[0].ReleaseTag != "Mine" {
		t.Fatalf("applied %+v", applied)
	}
	if srv.current().Config.Unit3DURL != "https://tracker.example" {
		t.Fatal("services not replaced")
	}

	// A secret left out is kept; "" goes back to the default.
	if code := call(t, h, "PUT", "/api/config", map[string]any{"values": map[string]any{"BOOKRR_UNIT3D_API_KEY": nil, "BOOKRR_SYNC_INTERVAL": "10m"}}, &out); code != 200 {
		t.Fatalf("status %d", code)
	}
	if applied[1].Unit3DAPIKey != "secret" || get("BOOKRR_SYNC_INTERVAL").Value != "10m" {
		t.Fatalf("applied %+v", applied[1])
	}

	// Refused: invalid, and not an option of the UI. Nothing changes.
	for _, values := range []map[string]any{{"BOOKRR_UNIT3D_API_KEY": ""}, {"BOOKRR_AUTH_PASSWORD": "x"}} {
		if code := call(t, h, "PUT", "/api/config", map[string]any{"values": values}, nil); code != 400 {
			t.Fatalf("%v: status %d", values, code)
		}
	}
	if len(applied) != 2 {
		t.Fatalf("applied after refusals: %d", len(applied))
	}
	if stored, _ := st.ConfigValues(t.Context()); stored["BOOKRR_UNIT3D_API_KEY"] != "secret" {
		t.Fatalf("stored %v", stored)
	}
}

func TestTestConfig(t *testing.T) {
	prowlarr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/system/status" || r.Header.Get("X-Api-Key") != "good" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`{"version": "1.30.2"}`))
	}))
	defer prowlarr.Close()
	st, h := newServer(t)
	if err := st.SaveConfigValues(t.Context(), map[string]string{"BOOKRR_PROWLARR_API_KEY": "good"}); err != nil {
		t.Fatal(err)
	}
	var res struct {
		OK      bool   `json:"ok"`
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	// The saved key, with the address being typed.
	body := map[string]any{"service": "prowlarr", "values": map[string]any{"BOOKRR_PROWLARR_URL": prowlarr.URL, "BOOKRR_PROWLARR_API_KEY": nil}}
	if code := call(t, h, "POST", "/api/config/test", body, &res); code != 200 || !res.OK || res.Message != "Collegato a Prowlarr 1.30.2" {
		t.Fatalf("%d %+v", code, res)
	}
	body["values"] = map[string]any{"BOOKRR_PROWLARR_URL": prowlarr.URL, "BOOKRR_PROWLARR_API_KEY": "bad"}
	if code := call(t, h, "POST", "/api/config/test", body, &res); code != 200 || res.OK || res.Error == "" {
		t.Fatalf("%d %+v", code, res)
	}
}
