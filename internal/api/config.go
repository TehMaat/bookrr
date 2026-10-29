package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/tehmaat/bookrr/internal/config"
	"github.com/tehmaat/bookrr/internal/mdblist"
	"github.com/tehmaat/bookrr/internal/prowlarr"
	"github.com/tehmaat/bookrr/internal/tmdb"
	"github.com/tehmaat/bookrr/internal/unit3d"
)

// configOption is an option of the configuration as the UI sees it.
type configOption struct {
	config.Option
	// As typed in the UI, or the environment's when Locked; never a secret.
	Value string `json:"value"`
	// It has a value, in the environment or from the UI.
	Set bool `json:"set"`
	// Set in the environment: shown, not changeable.
	Locked bool `json:"locked"`
}

func (s *Server) getConfig(w http.ResponseWriter, r *http.Request) {
	out, err := s.configOptions(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"options": out})
}

func (s *Server) configOptions(ctx context.Context) ([]configOption, error) {
	stored, err := s.store.ConfigValues(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]configOption, 0, len(config.Options))
	for _, o := range config.Options {
		v := stored[o.Key]
		locked := config.Locked(o.Key)
		if locked {
			v = os.Getenv(o.Key)
		}
		c := configOption{Option: o, Value: v, Set: v != "", Locked: locked}
		if o.Secret {
			c.Value = ""
		}
		out = append(out, c)
	}
	return out, nil
}

// configValues is what the UI sends: a value for each option changed, null
// (or nothing) to keep it, "" to go back to the default.
type configValues map[string]*string

// unknown returns the error for an option sent that the UI cannot set, "" if none.
func (in configValues) unknown() string {
	for k := range in {
		if !config.IsOption(k) {
			return fmt.Sprintf("%s non si può impostare dall'interfaccia", k)
		}
	}
	return ""
}

// merged is the stored options with the ones sent; those in the
// environment cannot be changed.
func (s *Server) merged(ctx context.Context, in configValues) (map[string]string, error) {
	stored, err := s.store.ConfigValues(ctx)
	if err != nil {
		return nil, err
	}
	for k, v := range in {
		if v != nil && !config.Locked(k) {
			stored[k] = strings.TrimSpace(*v)
		}
	}
	return stored, nil
}

// saveConfig saves the options sent and starts again, without restarting
// bookrr, the services whose options changed.
func (s *Server) saveConfig(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Values configValues `json:"values"`
	}
	if !decode(w, r, &in) {
		return
	}
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	ctx := r.Context()
	if msg := in.Values.unknown(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	values, err := s.merged(ctx, in.Values)
	if err != nil {
		s.fail(w, err)
		return
	}
	cfg, err := config.Resolve(values)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.SaveConfigValues(ctx, values); err != nil {
		s.fail(w, err)
		return
	}
	svc := s.current()
	svc.Config = cfg
	if s.Reconfigure != nil {
		svc = s.Reconfigure(cfg)
	}
	s.mu.Lock()
	s.svc = svc
	s.mu.Unlock()
	s.getConfig(w, r)
}

// testConfig tries the connection to a service with the options sent (the
// saved ones for those not sent): {"service": "unit3d", "values": {…}}.
func (s *Server) testConfig(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Service string       `json:"service"`
		Values  configValues `json:"values"`
	}
	if !decode(w, r, &in) {
		return
	}
	if msg := in.Values.unknown(); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	values, err := s.merged(r.Context(), in.Values)
	if err != nil {
		s.fail(w, err)
		return
	}
	get := func(key string) string {
		if config.Locked(key) {
			return os.Getenv(key)
		}
		return values[key]
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	var msg string
	switch in.Service {
	case "unit3d":
		url, key := strings.TrimRight(get("BOOKRR_UNIT3D_URL"), "/"), get("BOOKRR_UNIT3D_API_KEY")
		if url == "" || key == "" {
			err = errors.New("servono indirizzo e API key")
			break
		}
		var name string
		if name, err = unit3d.New(url, key).Username(ctx); err == nil {
			msg = "Collegato come " + name
		}
	case "tmdb":
		if key := get("BOOKRR_TMDB_API_KEY"); key == "" {
			err = errors.New("serve la chiave")
		} else if err = tmdb.New(key).Check(ctx); err == nil {
			msg = "Chiave TMDB valida"
		}
	case "mdblist":
		if key := get("BOOKRR_MDBLIST_API_KEY"); key == "" {
			err = errors.New("serve la chiave")
		} else {
			var name string
			if name, err = mdblist.New(key).User(ctx); err == nil {
				msg = strings.TrimSpace("Chiave MDBList valida " + name)
			}
		}
	case "prowlarr":
		url, key := strings.TrimRight(get("BOOKRR_PROWLARR_URL"), "/"), get("BOOKRR_PROWLARR_API_KEY")
		if url == "" || key == "" {
			err = errors.New("servono indirizzo e API key")
			break
		}
		var v string
		if v, err = prowlarr.New(url, key).Version(ctx); err == nil {
			msg = strings.TrimSpace("Collegato a Prowlarr " + v)
		}
	default:
		writeError(w, http.StatusBadRequest, "servizio sconosciuto")
		return
	}
	if err != nil {
		writeJSON(w, 200, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "message": msg})
}
