package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tehmaat/bookrr/internal/prowlarr"
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
	out := map[string]any{"enabled": s.history != nil, "state": nil, "syncedAt": nil, "notOnTracker": []notOnTracker{}, "best": nil}
	if s.history != nil {
		out["state"] = s.history.State()
	}
	releases, err := s.store.ListTrackerReleases(ctx)
	if err != nil {
		s.fail(w, err)
		return
	}
	if s.best != nil {
		out["best"] = s.best.State()
		found, err := s.store.BestResolutions(ctx)
		if err != nil {
			s.fail(w, err)
			return
		}
		prowlarr.Annotate(releases, found)
	}
	out["releases"] = releases
	synced, err := s.store.TrackerReleasesSyncedAt(ctx)
	if err != nil {
		s.fail(w, err)
		return
	}
	out["syncedAt"] = synced
	if synced != nil && s.history != nil {
		list, err := s.notOnTracker(ctx)
		if err != nil {
			s.fail(w, err)
			return
		}
		out["notOnTracker"] = list
	}
	writeJSON(w, 200, out)
}

// notOnTracker returns the personal releases of bookrr from this tracker
// (or from no known tracker) that are not among the uploads.
func (s *Server) notOnTracker(ctx context.Context) ([]notOnTracker, error) {
	missing, err := s.store.PersonalReleasesNotOnTracker(ctx)
	if err != nil {
		return nil, err
	}
	list := []notOnTracker{}
	for _, m := range missing {
		if sameSite(m.Tracker, s.cfg.Unit3DURL) {
			list = append(list, notOnTracker{Hash: m.Hash, Name: m.Name, Tracker: m.Tracker})
		}
	}
	return list, nil
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

// unflagReleases takes torrents out of the personal releases, removing the
// release tag on the clients too: {"hashes": ["…"]}, or {"all": true} for
// every personal release not among the uploads on the tracker. A client that
// fails keeps its tag, and the flag, on the torrents it has.
func (s *Server) unflagReleases(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Hashes []string `json:"hashes"`
		All    bool     `json:"all"`
	}
	if !decode(w, r, &in) {
		return
	}
	ctx := r.Context()
	if in.All {
		in.Hashes = nil
		if s.history != nil {
			list, err := s.notOnTracker(ctx)
			if err != nil {
				s.fail(w, err)
				return
			}
			for _, m := range list {
				in.Hashes = append(in.Hashes, m.Hash)
			}
		}
	}
	if len(in.Hashes) == 0 {
		writeError(w, http.StatusBadRequest, "nessun torrent indicato")
		return
	}

	tag := s.cfg.ReleaseTag
	locs, err := s.store.ReleaseTagLocations(ctx, in.Hashes, tag)
	if err != nil {
		s.fail(w, err)
		return
	}
	byClient := map[int64][]store.TaggedLocation{}
	order := []int64{}
	for _, l := range locs {
		if _, ok := byClient[l.ClientID]; !ok {
			order = append(order, l.ClientID)
		}
		byClient[l.ClientID] = append(byClient[l.ClientID], l)
	}
	done, failed := []store.TaggedLocation{}, []string{}
	for _, id := range order {
		c, err := s.store.GetClient(ctx, id)
		if err != nil {
			s.fail(w, err)
			return
		}
		hashes, tags, seen := []string{}, []string{}, map[string]bool{}
		for _, l := range byClient[id] {
			hashes = append(hashes, l.Hash)
			if !seen[l.Tag] {
				seen[l.Tag] = true
				tags = append(tags, l.Tag)
			}
		}
		qctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		err = s.syncer.Qbit(c).RemoveTags(qctx, hashes, tags)
		cancel()
		if err != nil {
			slog.Warn("tag non rimosso dal client", "client", c.Name, "count", len(hashes), "err", err)
			failed = append(failed, fmt.Sprintf("%s: %v", c.Name, err))
			continue
		}
		done = append(done, byClient[id]...)
	}

	n, err := s.store.UnflagPersonalReleases(ctx, in.Hashes, done, tag)
	if err != nil {
		s.fail(w, err)
		return
	}
	slog.Info("flag Personal Release tolto", "count", n, "fromClients", len(done))
	if n == 0 && len(failed) > 0 {
		writeError(w, http.StatusBadGateway, "tag non rimosso: "+strings.Join(failed, "; "))
		return
	}
	writeJSON(w, 200, map[string]any{"unflagged": n, "errors": failed})
}
