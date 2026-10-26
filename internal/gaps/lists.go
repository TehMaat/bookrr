package gaps

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/tehmaat/bookrr/internal/mdblist"
	"github.com/tehmaat/bookrr/internal/store"
	"github.com/tehmaat/bookrr/internal/tmdb"
	"github.com/tehmaat/bookrr/internal/unit3d"
)

// checkLists finds, for every list of films the user follows (a universe,
// a studio: added by hand, from MDBList), which films are missing in each
// quality they uploaded some of the list in, and whether others uploaded
// them. A list is shown even if the user has none of its films. Without
// an MDBList key the lists are not checked.
func (c *Checker) checkLists(ctx context.Context) (string, error) {
	if c.MDBList == nil {
		return "", nil
	}
	lists, err := c.store.GapLists(ctx)
	if err != nil {
		return "", err
	}
	ups, err := c.store.ActiveTrackerUploads(ctx)
	if err != nil {
		return "", err
	}
	byMovie, err := c.myMovies(ctx, ups, nil)
	if err != nil {
		return "", err
	}

	// A film in several lists is read once.
	movies := map[int64]tmdb.Movie{}
	torrents := map[int64][]unit3d.Upload{}
	c.progress("lists", len(lists))
	var groups []*Group
	for i, l := range lists {
		g, err := c.list(ctx, l, byMovie, movies, torrents)
		if err != nil {
			return "", err
		}
		groups = append(groups, g)
		c.update(func(s *State) { s.Done = i + 1 })
	}

	// Leave out the lists removed while this check ran.
	now, err := c.store.GapLists(ctx)
	if err != nil {
		return "", err
	}
	still := map[string]bool{}
	for _, l := range now {
		still[l.Key()] = true
	}
	kept := groups[:0]
	for _, g := range groups {
		if still[g.Key] {
			kept = append(kept, g)
		}
	}
	missing, unver, err := c.save(ctx, KindList, kept)
	if err != nil {
		return "", err
	}
	n := len(kept) - unver
	res := fmt.Sprintf("%d %s, %d film mancanti", n, plural(n, "collezione", "collezioni"), missing)
	switch {
	case unver == 1:
		res += ", 1 lista non trovata"
	case unver > 1:
		res += fmt.Sprintf(", %d liste non trovate", unver)
	}
	return res, nil
}

// list checks one list: its films on MDBList and TMDB, their torrents on
// the tracker. mine are the user's films by TMDB id; movies and torrents
// keep what was read for the other lists.
func (c *Checker) list(ctx context.Context, l store.GapList, mine map[int64][]store.TrackerUpload,
	movies map[int64]tmdb.Movie, torrents map[int64][]unit3d.Upload) (*Group, error) {
	g := &Group{Kind: KindList, Key: l.Key(), Title: l.Title, Status: StatusOK, Units: []Unit{}, Rows: []Row{}, Mine: []Ref{},
		SourceURL: mdblist.PageURL(l.Path)}
	if g.Title == "" {
		g.Title = l.Path
	}
	items, err := c.MDBList.Items(ctx, l.Path)
	if errors.Is(err, mdblist.ErrNotFound) {
		g.Status, g.Note = StatusUnverifiable, "lista non trovata su MDBList: è stata cancellata o resa privata?"
		return g, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lista %s su MDBList: %w", l.Path, err)
	}

	// The films already out, and the user's even if TMDB does not have
	// them as out, in the order of the list.
	today := c.now().Format("2006-01-02")
	unitOf := map[int64]int{}
	var myUploads []movieUpload
	for _, it := range items {
		id := it.TmdbID
		if id == 0 && it.Imdb() != 0 {
			if id, err = c.fromIMDB(ctx, it.Imdb()); err != nil {
				return nil, err
			}
		}
		if id == 0 || unitOf[id] != 0 {
			continue
		}
		m, ok := movies[id]
		if !ok {
			m, err = tmdbRetry(ctx, func() (tmdb.Movie, error) { return c.tmdb.Movie(ctx, id) })
			switch {
			case errors.Is(err, tmdb.ErrNotFound):
				m = tmdb.Movie{}
			case err != nil:
				return nil, fmt.Errorf("film %d su TMDB: %w", id, err)
			}
			m.ID = id
			if m.Title == "" {
				m.Title = it.Title
			}
			movies[id] = m
		}
		ups := mine[id]
		if len(ups) == 0 && !m.Released(today) {
			continue
		}
		name := m.Title
		if name == "" {
			name = "Film " + strconv.FormatInt(id, 10)
		}
		g.Units = append(g.Units, Unit{Number: len(g.Units) + 1, Name: name, AirDate: m.ReleaseDate, TmdbID: id})
		unitOf[id] = len(g.Units)
		if y := m.Year(); y != "" && (g.Year == "" || y < g.Year) {
			g.Year = y
		}
		for _, u := range ups {
			myUploads = append(myUploads, movieUpload{TrackerUpload: u, movie: m})
			g.Mine = append(g.Mine, c.ref(u.ID, u.Name, u.DetailsLink, unit3d.QualityOf(u.Name, u.Type, u.Resolution, ""), true))
		}
	}

	// The tracker searches by saga, not by list: one search per film.
	user, err := c.user(ctx)
	if err != nil {
		return nil, fmt.Errorf("nome utente sul tracker: %w", err)
	}
	var listed []unit3d.Upload
	for _, u := range g.Units {
		ts, ok := torrents[u.TmdbID]
		if !ok {
			ts, err = unit3d.WithRetry(ctx, func() ([]unit3d.Upload, error) { return c.tracker.ByTmdb(ctx, u.TmdbID, user) })
			if err != nil {
				return nil, fmt.Errorf("torrent del film %d sul tracker: %w", u.TmdbID, err)
			}
			for i := range ts {
				ts[i].TmdbID = u.TmdbID // what the search asked for
			}
			torrents[u.TmdbID] = ts
		}
		listed = append(listed, ts...)
	}
	c.fillMovies(g, unitOf, listed, myUploads)
	return g, nil
}
