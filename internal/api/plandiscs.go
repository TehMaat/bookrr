package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/tehmaat/bookrr/internal/prowlarr"
	"github.com/tehmaat/bookrr/internal/store"
	"github.com/tehmaat/bookrr/internal/unit3d"
)

// planDiscsEntry reads the film of the plan of the path, with the searcher
// of Prowlarr; false once it has answered with an error.
func (s *Server) planDiscsEntry(w http.ResponseWriter, r *http.Request) (store.PlanEntry, *prowlarr.Finder, bool) {
	id, ok := pathID(w, r)
	if !ok {
		return store.PlanEntry{}, nil, false
	}
	best := s.current().Best
	if best == nil {
		writeError(w, http.StatusBadRequest, s.planSearchDisabled())
		return store.PlanEntry{}, nil, false
	}
	e, err := s.store.PlanEntry(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return store.PlanEntry{}, nil, false
	}
	return e, best, true
}

// planDiscs returns the last search of the full discs of a film of the plan.
func (s *Server) planDiscs(w http.ResponseWriter, r *http.Request) {
	e, best, ok := s.planDiscsEntry(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, best.Discs(e))
}

// searchPlanDiscs searches the full discs of a film of the plan on
// Prowlarr, in the background (it can take minutes).
func (s *Server) searchPlanDiscs(w http.ResponseWriter, r *http.Request) {
	e, best, ok := s.planDiscsEntry(w, r)
	if !ok {
		return
	}
	if best.StartDiscs(e) {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
			defer cancel()
			best.SearchDiscs(ctx, e)
		}()
	}
	writeJSON(w, http.StatusAccepted, best.Discs(e))
}

// loadPlanDisc loads the .torrent of a full disc found with Prowlarr on
// the film of the plan, at the first free position, as "Load .torrent".
func (s *Server) loadPlanDisc(w http.ResponseWriter, r *http.Request) {
	e, best, ok := s.planDiscsEntry(w, r)
	if !ok {
		return
	}
	d, ok := best.Disc(e, r.PathValue("disc"))
	if !ok {
		writeError(w, http.StatusNotFound, "this disc is not among those found: search the full discs again")
		return
	}
	if !d.Loadable {
		writeError(w, http.StatusBadRequest, "Prowlarr has only a magnet link of this release, not a .torrent file")
		return
	}
	data, err := best.DownloadDisc(r.Context(), d, maxTorrentFile)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	meta, err := unit3d.ReadTorrent(data)
	if err != nil {
		writeError(w, http.StatusBadGateway, "Prowlarr did not give a valid .torrent file: "+err.Error())
		return
	}
	name := meta.Name
	if name == "" {
		name = d.Title
	}
	t, err := s.store.SetPlanTorrent(r.Context(), e.ID, 0, store.PlanTorrentInput{Hash: meta.Hash, Name: name, Size: meta.Size, Data: data})
	if err != nil {
		s.failPlanTorrent(w, err)
		return
	}
	slog.Info("full disc loaded on the plan", "plan", e.ID, "position", t.Position, "hash", t.Hash, "name", t.Name, "indexer", d.Indexer)
	writeJSON(w, http.StatusCreated, t)
}
