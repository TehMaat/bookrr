package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tehmaat/bookrr/internal/gaps"
	"github.com/tehmaat/bookrr/internal/prowlarr"
	"github.com/tehmaat/bookrr/internal/store"
	"github.com/tehmaat/bookrr/internal/syncer"
	"github.com/tehmaat/bookrr/internal/tmdb"
	"github.com/tehmaat/bookrr/internal/unit3d"
)

// maxPlanBulk is how many films can be added to the plan at once.
const maxPlanBulk = 200

// planDisabled says why films cannot be added to the release plan, "" if they can.
func (s *Server) planDisabled() string {
	if s.current().TMDB == nil {
		return "A TMDB key is needed: set it in Settings, Missing tab."
	}
	return ""
}

// planSearchDisabled says why the films of the plan are not searched on Prowlarr, "" if they are.
func (s *Server) planSearchDisabled() string {
	svc := s.current()
	switch {
	case svc.Best != nil:
		return ""
	case svc.Config.ProwlarrURL == "":
		return "Connect Prowlarr in Settings (Prowlarr tab) for the max resolution of each film."
	default:
		return "Prowlarr searches films only with a UNIT3D tracker: set it in Settings, Tracker tab."
	}
}

// planUpload is an upload of the user of a film of the plan.
type planUpload struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	URL     string `json:"url"`
	Quality string `json:"quality"`
}

type planItem struct {
	store.PlanEntry
	Released bool `json:"released"`
	// The best release found with Prowlarr, nil if not searched yet.
	Best      *store.BestResolution `json:"best"`
	Searching bool                  `json:"searching"`
	// The user's uploads of the film still on the tracker.
	Uploads []planUpload `json:"uploads"`
	// The torrents of others on the tracker, best first, as the last check
	// of the missing films found them (only for films of sagas and lists).
	Others []gaps.Ref `json:"others"`
	// The film on the tracker, "" without tracker.
	TrackerURL string `json:"trackerUrl"`
	// The .torrent files loaded on the film (two for a merge), with how
	// their download goes.
	Torrents []store.PlanTorrent `json:"torrents"`
	// The pair of the two torrents of a merge on Wayfinderr, nil if none.
	Merge *store.PlanMerge `json:"merge"`
}

// listPlan returns the films of the release plan, with the best release
// found for each with Prowlarr and how they stand on the tracker.
func (s *Server) listPlan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	svc := s.current()
	entries, err := s.store.PlanEntries(ctx)
	if err != nil {
		s.fail(w, err)
		return
	}
	ups, err := s.store.ActiveTrackerUploads(ctx)
	if err != nil {
		s.fail(w, err)
		return
	}
	found, err := s.store.BestResolutions(ctx)
	if err != nil {
		s.fail(w, err)
		return
	}
	others, err := s.trackerOthers(ctx)
	if err != nil {
		s.fail(w, err)
		return
	}
	torrents, err := s.store.PlanTorrents(ctx)
	if err != nil {
		s.fail(w, err)
		return
	}
	merges, err := s.store.PlanMerges(ctx)
	if err != nil {
		s.fail(w, err)
		return
	}
	best := prowlarr.AnnotatePlan(entries, ups, found)
	today := time.Now().Format("2006-01-02")
	items := make([]planItem, len(entries))
	for i, e := range entries {
		it := planItem{PlanEntry: e, Released: e.Released(today), Best: best[e.ID], Uploads: []planUpload{}, Others: others[e.TmdbID]}
		if it.Others == nil {
			it.Others = []gaps.Ref{}
		}
		if it.Torrents = torrents[e.ID]; it.Torrents == nil {
			it.Torrents = []store.PlanTorrent{}
		}
		// A pair planned for torrents the film no longer has is about to go.
		if m, ok := merges[e.ID]; ok && m.Hashes == store.MergeHashes(it.Torrents) {
			it.Merge = &m
		}
		if svc.Best != nil {
			it.Searching = svc.Best.Searching(e)
		}
		for _, u := range ups {
			if u.TmdbID == e.TmdbID && unit3d.IsMovie(u.Category, u.Name, u.TvdbID) {
				it.Uploads = append(it.Uploads, planUpload{ID: u.ID, Name: u.Name, URL: u.DetailsLink,
					Quality: unit3d.QualityOf(u.Name, u.Type, u.Resolution, "").Label})
			}
		}
		if site := strings.TrimRight(svc.Config.Unit3DURL, "/"); site != "" {
			it.TrackerURL = site + "/torrents?" + url.Values{"tmdbId": {strconv.FormatInt(e.TmdbID, 10)}}.Encode()
		}
		items[i] = it
	}
	out := map[string]any{
		"enabled":              s.planDisabled() == "",
		"disabledReason":       s.planDisabled(),
		"searchEnabled":        svc.Best != nil,
		"searchDisabledReason": s.planSearchDisabled(),
		"best":                 nil,
		"entries":              items,
	}
	if svc.Best != nil {
		out["best"] = svc.Best.State()
	}
	writeJSON(w, http.StatusOK, out)
}

// trackerOthers is, by TMDB id of a film, the torrents of others on the
// tracker that the last checks of the sagas and of the lists found.
func (s *Server) trackerOthers(ctx context.Context) (map[int64][]gaps.Ref, error) {
	out := map[int64][]gaps.Ref{}
	seen := map[int64]map[int64]bool{}
	for _, kind := range []string{gaps.KindSaga, gaps.KindList} {
		groups, err := s.store.GapGroups(ctx, kind)
		if err != nil {
			return nil, err
		}
		for _, sg := range groups {
			var g gaps.Group
			if json.Unmarshal(sg.Data, &g) != nil {
				continue
			}
			for _, u := range g.Units {
				if u.TmdbID == 0 {
					continue
				}
				if seen[u.TmdbID] == nil {
					seen[u.TmdbID] = map[int64]bool{}
				}
				for _, v := range u.Versions {
					if v.Mine || seen[u.TmdbID][v.ID] {
						continue
					}
					seen[u.TmdbID][v.ID] = true
					out[u.TmdbID] = append(out[u.TmdbID], v.Ref)
				}
			}
		}
	}
	return out, nil
}

var (
	tmdbLinkRe = regexp.MustCompile(`themoviedb\.org/(?:[a-z]{2}(?:-[A-Z]{2})?/)?(movie|tv)/(\d+)`)
	imdbIDRe   = regexp.MustCompile(`(?i)\btt(\d{5,10})\b`)
)

// planTmdbID reads the TMDB id of a film from what the user typed: the id,
// the link to the film on TMDB, or its IMDb id ("tt0133093", or the link).
func planTmdbID(ctx context.Context, c *tmdb.Client, q string) (int64, error) {
	q = strings.TrimSpace(q)
	if m := tmdbLinkRe.FindStringSubmatch(q); m != nil {
		if m[1] == "tv" {
			return 0, errors.New("it's a TV series: the plan only takes films for now")
		}
		return strconv.ParseInt(m[2], 10, 64)
	}
	if id, err := strconv.ParseInt(q, 10, 64); err == nil && id > 0 {
		return id, nil
	}
	if m := imdbIDRe.FindStringSubmatch(q); m != nil {
		n, _ := strconv.ParseInt(m[1], 10, 64)
		id, err := c.FindMovieByIMDB(ctx, n)
		if errors.Is(err, tmdb.ErrNotFound) {
			return 0, errors.New("no film on TMDB with this IMDb id")
		}
		return id, err
	}
	return 0, errors.New("enter the film's TMDB id, its themoviedb.org link or its IMDb id (tt…)")
}

// planMovie reads a film from TMDB, as an entry of the plan.
func planMovie(ctx context.Context, c *tmdb.Client, id int64) (store.PlanEntry, error) {
	m, err := c.Movie(ctx, id)
	if errors.Is(err, tmdb.ErrNotFound) {
		return store.PlanEntry{}, fmt.Errorf("no film on TMDB with id %d", id)
	}
	if err != nil {
		return store.PlanEntry{}, err
	}
	return store.PlanEntry{TmdbID: m.ID, ImdbID: m.IMDbNumber(), Title: m.Title, OriginalTitle: m.OriginalTitle,
		ReleaseDate: m.ReleaseDate, PosterPath: m.PosterPath}, nil
}

// lookupPlan shows a film before it is added to the plan: ?q= as for addPlan.
func (s *Server) lookupPlan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := s.current().TMDB
	if c == nil {
		writeError(w, http.StatusBadRequest, s.planDisabled())
		return
	}
	id, err := planTmdbID(ctx, c, r.URL.Query().Get("q"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	e, err := planMovie(ctx, c, id)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var inPlan int64
	if p, err := s.store.PlanEntryByTmdb(ctx, id); err == nil {
		inPlan = p.ID
	} else if !errors.Is(err, store.ErrNotFound) {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"movie": e, "inPlan": inPlan})
}

type planInput struct {
	TMDB        string `json:"tmdb"`
	Source      string `json:"source"`
	SourceTitle string `json:"sourceTitle"`
	Note        string `json:"note"`
}

// addPlan adds a film to the plan, by TMDB id, link or IMDb id, and
// searches it on Prowlarr now when it is out.
func (s *Server) addPlan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := s.current().TMDB
	if c == nil {
		writeError(w, http.StatusBadRequest, s.planDisabled())
		return
	}
	var in planInput
	if !decode(w, r, &in) {
		return
	}
	id, err := planTmdbID(ctx, c, in.TMDB)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	e, err := planMovie(ctx, c, id)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	e.Source, e.SourceTitle, e.Note = in.Source, in.SourceTitle, in.Note
	e, err = s.store.AddPlanEntry(ctx, e)
	if errors.Is(err, store.ErrExists) {
		writeError(w, http.StatusConflict, "the film is already in the plan")
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	s.searchLater(e)
	writeJSON(w, http.StatusCreated, e)
}

type planBulkInput struct {
	TmdbIDs     []int64 `json:"tmdbIds"`
	Source      string  `json:"source"`
	SourceTitle string  `json:"sourceTitle"`
}

// addPlanBulk adds films to the plan by TMDB id (from the missing films
// of a saga or of a list), leaving out those already there.
func (s *Server) addPlanBulk(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := s.current().TMDB
	if c == nil {
		writeError(w, http.StatusBadRequest, s.planDisabled())
		return
	}
	var in planBulkInput
	if !decode(w, r, &in) {
		return
	}
	if len(in.TmdbIDs) == 0 || len(in.TmdbIDs) > maxPlanBulk {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("from 1 to %d films at a time", maxPlanBulk))
		return
	}
	added := []store.PlanEntry{}
	skipped := 0
	errs := []string{}
	seen := map[int64]bool{}
	for _, id := range in.TmdbIDs {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		if _, err := s.store.PlanEntryByTmdb(ctx, id); err == nil {
			skipped++
			continue
		} else if !errors.Is(err, store.ErrNotFound) {
			s.fail(w, err)
			return
		}
		e, err := planMovie(ctx, c, id)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		e.Source, e.SourceTitle = in.Source, in.SourceTitle
		e, err = s.store.AddPlanEntry(ctx, e)
		if errors.Is(err, store.ErrExists) {
			skipped++
			continue
		}
		if err != nil {
			s.fail(w, err)
			return
		}
		added = append(added, e)
	}
	s.searchLater(added...)
	writeJSON(w, http.StatusOK, map[string]any{"added": added, "skipped": skipped, "errors": errs})
}

// searchLater searches on Prowlarr, in the background, the films added to
// the plan that are out (the others are searched once out).
func (s *Server) searchLater(entries ...store.PlanEntry) {
	best := s.current().Best
	today := time.Now().Format("2006-01-02")
	var due []store.PlanEntry
	for _, e := range entries {
		if e.Released(today) {
			due = append(due, e)
		}
	}
	if best == nil || len(due) == 0 {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
		defer cancel()
		best.SearchPlan(ctx, due...)
	}()
}

// searchPlanNow searches a film of the plan on Prowlarr now, in the
// background, even if it is not out yet.
func (s *Server) searchPlanNow(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	best := s.current().Best
	if best == nil {
		writeError(w, http.StatusBadRequest, s.planSearchDisabled())
		return
	}
	e, err := s.store.PlanEntry(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		best.SearchPlan(ctx, e)
	}()
	writeJSON(w, http.StatusAccepted, map[string]bool{"searching": true})
}

func (s *Server) updatePlan(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in store.PlanUpdate
	if !decode(w, r, &in) {
		return
	}
	if in.Status != nil && !store.ValidPlanStatus(*in.Status) {
		writeError(w, http.StatusBadRequest, "invalid status")
		return
	}
	e, err := s.store.UpdatePlanEntry(r.Context(), id, in)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (s *Server) deletePlan(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	// Its pair on Wayfinderr goes too, if Wayfinderr answers.
	syncer.ForgetMergeOf(r.Context(), s.store, id)
	if err := s.store.DeletePlanEntry(r.Context(), id); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
