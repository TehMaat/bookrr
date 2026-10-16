package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/tehmaat/bookrr/internal/gaps"
	"github.com/tehmaat/bookrr/internal/store"
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
// field, with the user's options and choices applied, and the state of
// the checker.
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
	prefs, err := s.store.GapPrefs(ctx, kind)
	if err != nil {
		s.fail(w, err)
		return
	}
	marks, err := s.store.GapMarks(ctx, kind)
	if err != nil {
		s.fail(w, err)
		return
	}
	byGroup := map[string][]store.GapMark{}
	for _, m := range marks {
		byGroup[m.Group] = append(byGroup[m.Group], m)
	}
	list := make([]gaps.Group, 0, len(groups))
	for _, row := range groups {
		var g gaps.Group
		if err := json.Unmarshal(row.Data, &g); err != nil {
			s.fail(w, fmt.Errorf("risultato di %q: %w", row.Key, err))
			return
		}
		gaps.Apply(&g, prefs, byGroup[g.Key])
		list = append(list, g)
	}
	out := map[string]any{"enabled": s.missing != nil, "disabledReason": s.missingDisabled(), "state": nil,
		"checkedAt": checked, "checkAt": minutesClock(s.cfg.MissingCheckAt), field: list, "prefs": prefs,
		"types": gaps.TypeLadder, "resolutions": gaps.Resolutions}
	if s.missing != nil {
		out["state"] = s.missing.State()
	}
	writeJSON(w, http.StatusOK, out)
}

// missingKind is the kind of check of the path ("series", "sagas").
func missingKind(w http.ResponseWriter, r *http.Request) (string, bool) {
	switch r.PathValue("kind") {
	case "series":
		return gaps.KindSeries, true
	case "sagas":
		return gaps.KindSaga, true
	}
	writeError(w, http.StatusNotFound, "tipo di controllo sconosciuto: "+r.PathValue("kind"))
	return "", false
}

// saveMissingPrefs stores the minimum quality of the view by part and
// whether what others uploaded counts as present.
func (s *Server) saveMissingPrefs(w http.ResponseWriter, r *http.Request) {
	kind, ok := missingKind(w, r)
	if !ok {
		return
	}
	var in store.GapPrefs
	if !decode(w, r, &in) {
		return
	}
	t, err := gaps.ValidThreshold(in.GapThreshold)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	in.GapThreshold = t
	if err := s.store.SaveGapPrefs(r.Context(), kind, in); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, in)
}

// markIn is a choice of the user on a result (see store.GapMark).
type markIn struct {
	Group     string              `json:"group"`
	Action    string              `json:"action"`
	Quality   string              `json:"quality"`
	Unit      int64               `json:"unit"`
	Torrent   int64               `json:"torrent"`
	Threshold *store.GapThreshold `json:"threshold"`
}

// readMark reads and checks a choice; what an action does not use is cleared.
func readMark(w http.ResponseWriter, r *http.Request) (store.GapMark, *markIn, bool) {
	kind, ok := missingKind(w, r)
	if !ok {
		return store.GapMark{}, nil, false
	}
	var in markIn
	if !decode(w, r, &in) {
		return store.GapMark{}, nil, false
	}
	m := store.GapMark{Kind: kind, Group: in.Group, Action: in.Action, Quality: in.Quality, Unit: in.Unit, Torrent: in.Torrent}
	bad := ""
	switch in.Action {
	case gaps.ActionHide:
		m.Unit, m.Torrent = 0, 0
	case gaps.ActionComplete, gaps.ActionThreshold:
		m.Quality, m.Unit, m.Torrent = "", 0, 0
	case gaps.ActionAccept:
		if m.Unit == 0 || m.Torrent == 0 {
			bad = "indica la parte e il torrent da accettare"
		}
	case gaps.ActionIgnore:
		m.Torrent = 0
		if m.Unit == 0 {
			bad = "indica la parte da ignorare"
		}
	default:
		bad = "azione sconosciuta: " + in.Action
	}
	if in.Group == "" {
		bad = "nessuna serie o saga indicata"
	}
	if bad != "" {
		writeError(w, http.StatusBadRequest, bad)
		return m, nil, false
	}
	return m, &in, true
}

// setMissingMark stores a choice of the user: hide a collection or a row,
// mark it complete, accept a torrent of others, ignore a part, give it a
// minimum quality of its own.
func (s *Server) setMissingMark(w http.ResponseWriter, r *http.Request) {
	m, in, ok := readMark(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	row, err := s.store.GapGroup(ctx, m.Kind, m.Group)
	if err != nil {
		s.fail(w, err)
		return
	}
	switch m.Action {
	case gaps.ActionComplete:
		var g gaps.Group
		if err := json.Unmarshal(row.Data, &g); err != nil {
			s.fail(w, err)
			return
		}
		m.Value = gaps.CompleteValue(g)
	case gaps.ActionThreshold:
		if in.Threshold == nil {
			writeError(w, http.StatusBadRequest, "nessuna qualità minima indicata")
			return
		}
		t, err := gaps.ValidThreshold(*in.Threshold)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		data, _ := json.Marshal(t)
		m.Value = string(data)
	}
	if err := s.store.SetGapMark(ctx, m); err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// deleteMissingMark takes a choice back.
func (s *Server) deleteMissingMark(w http.ResponseWriter, r *http.Request) {
	m, _, ok := readMark(w, r)
	if !ok {
		return
	}
	if err := s.store.DeleteGapMark(r.Context(), m); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
