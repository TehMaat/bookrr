package gaps

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"sort"
	"strconv"

	"github.com/tehmaat/bookrr/internal/store"
	"github.com/tehmaat/bookrr/internal/tmdb"
	"github.com/tehmaat/bookrr/internal/unit3d"
)

// isMovie reports whether an upload is a film (see unit3d.IsMovie).
func isMovie(u store.TrackerUpload) bool { return unit3d.IsMovie(u.Category, u.Name, u.TvdbID) }

// movieUpload is an upload of the user that is a film of a saga.
type movieUpload struct {
	store.TrackerUpload
	movie tmdb.Movie
}

// errNoCollectionFilter means the tracker answered the search by saga with
// torrents of other films: it does not know the filter.
var errNoCollectionFilter = errors.New("il tracker non filtra per saga")

// checkSagas finds, for every saga (TMDB collection) the user uploaded at
// least a film of, which films are missing in each quality they uploaded
// the saga in, and whether others uploaded them.
func (c *Checker) checkSagas(ctx context.Context) (string, error) {
	ups, err := c.store.ActiveTrackerUploads(ctx)
	if err != nil {
		return "", err
	}
	// By TMDB id of the film; the rest cannot be checked.
	var unverifiable []*Group
	unknown := map[string]*Group{}
	addUnknown := func(key, title, note string, u store.TrackerUpload) {
		g := unknown[key]
		if g == nil {
			g = &Group{Kind: KindSaga, Key: key, Title: title, Status: StatusUnverifiable, Note: note, Units: []Unit{}, Rows: []Row{}}
			unknown[key] = g
			unverifiable = append(unverifiable, g)
		}
		g.Mine = append(g.Mine, c.ref(u.ID, u.Name, u.DetailsLink, unit3d.QualityOf(u.Name, u.Type, u.Resolution, ""), true))
	}
	byMovie, err := c.myMovies(ctx, ups, addUnknown)
	if err != nil {
		return "", err
	}

	// The saga of every film, from TMDB.
	movieIDs := sortedKeys(byMovie)
	c.progress("movies", len(movieIDs))
	bySaga := map[int64][]movieUpload{}
	for i, id := range movieIDs {
		m, err := tmdbRetry(ctx, func() (tmdb.Movie, error) { return c.tmdb.Movie(ctx, id) })
		switch {
		case errors.Is(err, tmdb.ErrNotFound):
			for _, u := range byMovie[id] {
				addUnknown("tmdb:"+strconv.FormatInt(id, 10), displayTitle(u.Name), fmt.Sprintf("film con id TMDB %d non trovato su TMDB", id), u)
			}
		case err != nil:
			return "", fmt.Errorf("film %d su TMDB: %w", id, err)
		case m.Collection != nil && m.Collection.ID != 0:
			if m.ID == 0 {
				m.ID = id
			}
			for _, u := range byMovie[id] {
				bySaga[m.Collection.ID] = append(bySaga[m.Collection.ID], movieUpload{TrackerUpload: u, movie: m})
			}
		}
		c.update(func(s *State) { s.Done = i + 1 })
	}

	sagaIDs := sortedKeys(bySaga)
	c.progress("sagas", len(sagaIDs))
	var groups []*Group
	for i, id := range sagaIDs {
		g, err := c.saga(ctx, id, bySaga[id])
		if err != nil {
			return "", err
		}
		groups = append(groups, g)
		c.update(func(s *State) { s.Done = i + 1 })
	}
	groups = append(groups, unverifiable...)

	missing, unver, err := c.save(ctx, KindSaga, groups)
	if err != nil {
		return "", err
	}
	sagas := len(groups) - unver
	res := fmt.Sprintf("%d %s, %d film mancanti", sagas, plural(sagas, "saga", "saghe"), missing)
	switch {
	case unver == 1:
		res += ", 1 film non verificabile"
	case unver > 1:
		res += fmt.Sprintf(", %d film non verificabili", unver)
	}
	return res, nil
}

// saga checks one saga: its films on TMDB, their torrents on the tracker.
func (c *Checker) saga(ctx context.Context, id int64, mine []movieUpload) (*Group, error) {
	g := &Group{Kind: KindSaga, Key: "collection:" + strconv.FormatInt(id, 10), TmdbID: id, Title: mine[0].movie.Collection.Name,
		Status: StatusOK, Units: []Unit{}, Rows: []Row{}, Mine: []Ref{},
		SearchURL: c.site + "/torrents?" + url.Values{"collectionId": {strconv.FormatInt(id, 10)}}.Encode()}
	myMovies := map[int64]tmdb.Movie{}
	for _, u := range mine {
		myMovies[u.movie.ID] = u.movie
		g.Mine = append(g.Mine, c.ref(u.ID, u.Name, u.DetailsLink, unit3d.QualityOf(u.Name, u.Type, u.Resolution, ""), true))
	}
	if g.Title == "" {
		g.Title = displayTitle(mine[0].Name)
	}

	col, err := tmdbRetry(ctx, func() (tmdb.Collection, error) { return c.tmdb.Collection(ctx, id) })
	if errors.Is(err, tmdb.ErrNotFound) {
		g.Status, g.Note = StatusUnverifiable, fmt.Sprintf("saga con id TMDB %d non trovata su TMDB", id)
		return g, nil
	}
	if err != nil {
		return nil, fmt.Errorf("saga %d su TMDB: %w", id, err)
	}
	if col.Name != "" {
		g.Title = col.Name
	}

	// The films already out, and the user's even if TMDB does not list
	// them in the saga or as out, in order of release.
	today := c.now().Format("2006-01-02")
	known := map[int64]bool{}
	var parts []tmdb.Part
	for _, p := range col.Parts {
		if p.ID == 0 || known[p.ID] {
			continue
		}
		known[p.ID] = true
		if _, isMine := myMovies[p.ID]; isMine || p.Released(today) {
			parts = append(parts, p)
		}
	}
	for mid, m := range myMovies {
		if !known[mid] {
			known[mid] = true
			parts = append(parts, tmdb.Part{ID: mid, Title: m.Title, ReleaseDate: m.ReleaseDate})
		}
	}
	sort.Slice(parts, func(i, j int) bool {
		a, b := parts[i].ReleaseDate, parts[j].ReleaseDate
		if (a == "") != (b == "") {
			return b == ""
		}
		if a != b {
			return a < b
		}
		return parts[i].ID < parts[j].ID
	})
	unitOf := map[int64]int{}
	for i, p := range parts {
		name := p.Title
		if name == "" {
			name = "Film " + strconv.FormatInt(p.ID, 10)
		}
		g.Units = append(g.Units, Unit{Number: i + 1, Name: name, AirDate: p.ReleaseDate, TmdbID: p.ID})
		unitOf[p.ID] = i + 1
	}
	if len(parts) > 0 {
		g.Year = parts[0].Year()
	}

	listed, err := c.sagaTorrents(ctx, id, g.Units, known)
	if err != nil {
		return nil, err
	}

	c.fillMovies(g, unitOf, listed, mine)
	return g, nil
}

// myMovies sorts the user's films by TMDB id, found from the IMDb id when
// the tracker lacks it; the others go to unknown, with the reason.
func (c *Checker) myMovies(ctx context.Context, ups []store.TrackerUpload, unknown func(key, title, note string, u store.TrackerUpload)) (map[int64][]store.TrackerUpload, error) {
	byMovie := map[int64][]store.TrackerUpload{}
	for _, u := range ups {
		if !isMovie(u) {
			continue
		}
		id := u.TmdbID
		if id == 0 && u.ImdbID != 0 {
			found, err := c.fromIMDB(ctx, u.ImdbID)
			if err != nil {
				return nil, err
			}
			id = found
		}
		if id != 0 {
			byMovie[id] = append(byMovie[id], u)
			continue
		}
		if unknown == nil {
			continue
		}
		key, note := "title:"+unit3d.Title(u.Name), "nessun id TMDB né IMDb sul tracker"
		if u.ImdbID != 0 {
			key, note = "imdb:"+strconv.FormatInt(u.ImdbID, 10), fmt.Sprintf("l'id IMDb tt%07d non corrisponde a nessun film su TMDB", u.ImdbID)
		}
		unknown(key, displayTitle(u.Name), note, u)
	}
	return byMovie, nil
}

// fromIMDB is the TMDB id of the film with this IMDb id, 0 if none: asked
// to TMDB once per check.
func (c *Checker) fromIMDB(ctx context.Context, imdbID int64) (int64, error) {
	if id, ok := c.imdb[imdbID]; ok {
		return id, nil
	}
	found, err := tmdbRetry(ctx, func() (int64, error) { return c.tmdb.FindMovieByIMDB(ctx, imdbID) })
	if err != nil && !errors.Is(err, tmdb.ErrNotFound) {
		return 0, fmt.Errorf("ricerca su TMDB dell'id IMDb tt%07d: %w", imdbID, err)
	}
	if c.imdb == nil {
		c.imdb = map[int64]int64{}
	}
	c.imdb[imdbID] = found
	return found, nil
}

// fillMovies makes the grid of a group of films (a saga, a list) from the
// tracker's torrents of its films and the user's uploads of them; unitOf
// numbers the films by TMDB id.
func (c *Checker) fillMovies(g *Group, unitOf map[int64]int, listed []unit3d.Upload, mine []movieUpload) {
	myIDs := map[int64]movieUpload{}
	for _, u := range mine {
		myIDs[u.ID] = u
	}
	var items []item
	seen := map[int64]bool{}
	for _, t := range listed {
		if seen[t.ID] {
			continue
		}
		seen[t.ID] = true
		_, isMine := myIDs[t.ID]
		n, ok := unitOf[t.TmdbID]
		if !ok {
			continue // a film of the saga not out yet
		}
		// TMDB numbers films and series separately: only films count.
		if !isMine && (!unit3d.MovieCategory(t.Category) || unit3d.Episodic(t.Name)) {
			continue
		}
		q := unit3d.QualityOf(t.Name, t.Type, t.Resolution, t.MediaInfo)
		items = append(items, item{ref: c.ref(t.ID, t.Name, t.DetailsLink, q, isMine), quality: q, units: map[int]bool{n: true}})
	}
	// The user's uploads the search did not return (just uploaded, or a
	// different TMDB id on the tracker).
	for _, u := range mine {
		if seen[u.ID] {
			continue
		}
		q := unit3d.QualityOf(u.Name, u.Type, u.Resolution, "")
		items = append(items, item{ref: c.ref(u.ID, u.Name, u.DetailsLink, q, true), quality: q, units: map[int]bool{unitOf[u.movie.ID]: true}})
	}
	for i, m := range g.Mine {
		for _, it := range items {
			if it.ref.ID == m.ID {
				g.Mine[i].Quality = it.quality.Label
			}
		}
	}
	g.buildRows(items)
}

// sagaTorrents lists the tracker's torrents of the films of a saga: one
// search by saga or, if the tracker gives nothing for it (or does not know
// the filter), one search per film. known is every film of the saga.
func (c *Checker) sagaTorrents(ctx context.Context, id int64, units []Unit, known map[int64]bool) ([]unit3d.Upload, error) {
	user, err := c.user(ctx)
	if err != nil {
		return nil, fmt.Errorf("nome utente sul tracker: %w", err)
	}
	check := func(page []unit3d.Upload) error {
		for _, t := range page {
			if !known[t.TmdbID] {
				return errNoCollectionFilter
			}
		}
		return nil
	}
	listed, err := unit3d.WithRetry(ctx, func() ([]unit3d.Upload, error) { return c.tracker.ByCollection(ctx, id, user, check) })
	switch {
	case errors.Is(err, errNoCollectionFilter):
		slog.Debug("ricerca per saga ignorata dal tracker, cerco film per film", "saga", id)
		listed = nil
	case err != nil:
		return nil, fmt.Errorf("torrent della saga %d sul tracker: %w", id, err)
	}
	if len(listed) > 0 {
		return listed, nil
	}
	for _, u := range units {
		ts, err := unit3d.WithRetry(ctx, func() ([]unit3d.Upload, error) { return c.tracker.ByTmdb(ctx, u.TmdbID, user) })
		if err != nil {
			return nil, fmt.Errorf("torrent del film %d sul tracker: %w", u.TmdbID, err)
		}
		for _, t := range ts {
			t.TmdbID = u.TmdbID // what the search asked for
			listed = append(listed, t)
		}
	}
	return listed, nil
}

func sortedKeys[V any](m map[int64]V) []int64 {
	out := make([]int64, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
