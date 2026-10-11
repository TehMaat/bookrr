package api

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/tehmaat/bookrr/internal/store"
)

type notOnTracker struct {
	Hash    string `json:"hash"`
	Name    string `json:"name"`
	Tracker string `json:"tracker"`
}

// listReleases returns the user's uploads on the UNIT3D tracker with the
// status of the linked torrents, and the personal releases of bookrr that
// are not among them (once the uploads have been read at least once).
func (s *Server) listReleases(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	out := map[string]any{"enabled": s.history != nil, "state": nil, "syncedAt": nil, "notOnTracker": []notOnTracker{}}
	if s.history != nil {
		out["state"] = s.history.State()
	}
	releases, err := s.store.ListTrackerReleases(ctx)
	if err != nil {
		s.fail(w, err)
		return
	}
	out["releases"] = releases
	synced, err := s.store.TrackerReleasesSyncedAt(ctx)
	if err != nil {
		s.fail(w, err)
		return
	}
	out["syncedAt"] = synced
	if synced != nil && s.history != nil {
		missing, err := s.store.PersonalReleasesNotOnTracker(ctx)
		if err != nil {
			s.fail(w, err)
			return
		}
		list := []notOnTracker{}
		for _, m := range missing {
			if sameSite(m.Tracker, s.cfg.Unit3DURL) {
				list = append(list, notOnTracker{Hash: m.Hash, Name: m.Name, Tracker: m.Tracker})
			}
		}
		out["notOnTracker"] = list
	}
	writeJSON(w, 200, out)
}

// sameSite reports whether a torrent's announce URL may belong to the
// tracker: same domain (announce hosts are often a subdomain of the site).
// A torrent with no known tracker (added by hand) may be from anywhere.
func sameSite(announce, site string) bool {
	if strings.TrimSpace(announce) == "" {
		return true
	}
	a, err1 := url.Parse(announce)
	b, err2 := url.Parse(site)
	if err1 != nil || err2 != nil || a.Hostname() == "" {
		return true
	}
	return baseDomain(a.Hostname()) == baseDomain(b.Hostname())
}

func baseDomain(host string) string {
	parts := strings.Split(strings.ToLower(strings.TrimSuffix(host, ".")), ".")
	if len(parts) > 2 {
		parts = parts[len(parts)-2:]
	}
	return strings.Join(parts, ".")
}

// syncReleases starts reading the uploads now, in the background.
func (s *Server) syncReleases(w http.ResponseWriter, r *http.Request) {
	if s.history == nil {
		writeError(w, http.StatusBadRequest, "nessun tracker UNIT3D configurato (BOOKRR_UNIT3D_URL)")
		return
	}
	s.history.Wake()
	writeJSON(w, http.StatusAccepted, s.history.State())
}

// addReleases creates in bookrr the torrents of uploads it does not know:
// {"ids": [12, 34]} or {"all": true} for every one whose hash is known.
func (s *Server) addReleases(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []int64 `json:"ids"`
		All bool    `json:"all"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !in.All && len(in.IDs) == 0 {
		writeError(w, http.StatusBadRequest, "nessuna release indicata")
		return
	}
	if in.All {
		in.IDs = nil
	}
	added, err := s.store.AddTrackerReleases(r.Context(), in.IDs)
	if errors.Is(err, store.ErrNoReleaseHash) {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	slog.Info("release del tracker aggiunte a bookrr", "count", added)
	writeJSON(w, http.StatusCreated, map[string]int{"added": added})
}
