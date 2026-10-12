package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/tehmaat/bookrr/internal/gaps"
)

// missingDisabled says why the check of the missing seasons and films is off, "" if on.
func (s *Server) missingDisabled() string {
	switch {
	case s.missing != nil:
		return ""
	case s.cfg.Unit3DURL == "" && s.cfg.TMDBAPIKey == "":
		return "Servono un tracker UNIT3D (BOOKRR_UNIT3D_URL e BOOKRR_UNIT3D_API_KEY) e una chiave TMDB (BOOKRR_TMDB_API_KEY)."
	case s.cfg.Unit3DURL == "":
		return "Serve un tracker UNIT3D: imposta BOOKRR_UNIT3D_URL e BOOKRR_UNIT3D_API_KEY."
	default:
		return "Serve una chiave TMDB: imposta BOOKRR_TMDB_API_KEY."
	}
}

// missingSeries returns, for every series the user uploaded at least a
// season pack of, the grid of its seasons in each of their qualities, as
// stored by the last check.
func (s *Server) missingSeries(w http.ResponseWriter, r *http.Request) {
	s.missingGroups(w, r, gaps.KindSeries, "series")
}

// missingSagas returns, for every saga the user uploaded at least a film
// of, the grid of its films in each of their qualities, as stored by the
// last check.
func (s *Server) missingSagas(w http.ResponseWriter, r *http.Request) {
	s.missingGroups(w, r, gaps.KindSaga, "sagas")
}

// missingGroups answers with the stored results of a kind of check, as
// field, with the state of the checker.
func (s *Server) missingGroups(w http.ResponseWriter, r *http.Request, kind, field string) {
	ctx := r.Context()
	groups, err := s.store.GapGroups(ctx, kind)
	if err != nil {
		s.fail(w, err)
		return
	}
	checked, err := s.store.GapsCheckedAt(ctx, kind)
	if err != nil {
		s.fail(w, err)
		return
	}
	list := make([]json.RawMessage, len(groups))
	for i, g := range groups {
		list[i] = g.Data
	}
	out := map[string]any{"enabled": s.missing != nil, "disabledReason": s.missingDisabled(), "state": nil,
		"checkedAt": checked, "checkAt": minutesClock(s.cfg.MissingCheckAt), field: list}
	if s.missing != nil {
		out["state"] = s.missing.State()
	}
	writeJSON(w, http.StatusOK, out)
}

// checkMissing starts a check now, in the background ("Controlla ora").
func (s *Server) checkMissing(w http.ResponseWriter, r *http.Request) {
	if s.missing == nil {
		writeError(w, http.StatusBadRequest, s.missingDisabled())
		return
	}
	s.missing.Wake()
	writeJSON(w, http.StatusAccepted, s.missing.State())
}

// minutesClock renders 240 as "04:00".
func minutesClock(m int) string { return fmt.Sprintf("%02d:%02d", m/60, m%60) }
