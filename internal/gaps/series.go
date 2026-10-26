package gaps

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/tehmaat/bookrr/internal/store"
	"github.com/tehmaat/bookrr/internal/tmdb"
	"github.com/tehmaat/bookrr/internal/unit3d"
)

// tvCategoryRe tells the tracker categories of series ("TV", "Serie TV",
// "TV Show", "Anime").
var tvCategoryRe = regexp.MustCompile(`(?i)\btv\b|serie|show|anime`)

// seriesUpload is an upload of the user that is a season pack or a pack
// of several seasons.
type seriesUpload struct {
	store.TrackerUpload
	pack unit3d.Pack
}

// isSeries reports whether an upload is a pack of a series: it names its
// seasons, or says "Complete" in a category of series or with a TVDB id.
func isSeries(u store.TrackerUpload) (seriesUpload, bool) {
	p, ok := unit3d.ParsePack(u.Name)
	if !ok {
		return seriesUpload{}, false
	}
	if len(p.Seasons) == 0 && u.TvdbID == 0 && !tvCategoryRe.MatchString(u.Category) {
		return seriesUpload{}, false
	}
	return seriesUpload{TrackerUpload: u, pack: p}, true
}

// checkSeries finds, for every series the user uploaded at least a season
// pack of, which seasons are missing in each quality they uploaded it in
// and whether others uploaded them. Episodes are ignored, the user's and
// the others'.
func (c *Checker) checkSeries(ctx context.Context) (string, error) {
	ups, err := c.store.ActiveTrackerUploads(ctx)
	if err != nil {
		return "", err
	}
	// By TMDB id; the rest cannot be checked.
	byTmdb := map[int64][]seriesUpload{}
	var unverifiable []*Group
	unknown := map[string]*Group{}
	tvdbCache := map[int64]int64{}
	for _, u := range ups {
		su, ok := isSeries(u)
		if !ok {
			continue
		}
		id := u.TmdbID
		if id == 0 && u.TvdbID != 0 {
			found, ok := tvdbCache[u.TvdbID]
			if !ok {
				found, err = tmdbRetry(ctx, func() (int64, error) { return c.tmdb.FindTVByTVDB(ctx, u.TvdbID) })
				if err != nil && !errors.Is(err, tmdb.ErrNotFound) {
					return "", fmt.Errorf("ricerca su TMDB dell'id TVDB %d: %w", u.TvdbID, err)
				}
				tvdbCache[u.TvdbID] = found
			}
			id = found
		}
		if id != 0 {
			byTmdb[id] = append(byTmdb[id], su)
			continue
		}
		key, note := "title:"+unit3d.Title(u.Name), "nessun id TMDB né TVDB sul tracker"
		if u.TvdbID != 0 {
			key, note = "tvdb:"+strconv.FormatInt(u.TvdbID, 10), fmt.Sprintf("l'id TVDB %d non corrisponde a nessuna serie su TMDB", u.TvdbID)
		}
		g := unknown[key]
		if g == nil {
			g = &Group{Kind: KindSeries, Key: key, Title: displayTitle(u.Name), TvdbID: u.TvdbID, Status: StatusUnverifiable, Note: note,
				Units: []Unit{}, Rows: []Row{}}
			unknown[key] = g
			unverifiable = append(unverifiable, g)
		}
		g.Mine = append(g.Mine, c.ref(u.ID, u.Name, u.DetailsLink, unit3d.QualityOf(u.Name, u.Type, u.Resolution, ""), true))
	}

	ids := make([]int64, 0, len(byTmdb))
	for id := range byTmdb {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	c.progress("series", len(ids))
	var groups []*Group
	for i, id := range ids {
		g, err := c.series(ctx, id, byTmdb[id])
		if err != nil {
			return "", err
		}
		groups = append(groups, g)
		c.update(func(s *State) { s.Done = i + 1 })
	}
	groups = append(groups, unverifiable...)

	missing, unver, err := c.save(ctx, KindSeries, groups)
	if err != nil {
		return "", err
	}
	res := fmt.Sprintf("%d serie, %d stagioni mancanti", len(groups), missing)
	switch {
	case unver == 1:
		res += ", 1 non verificabile"
	case unver > 1:
		res += fmt.Sprintf(", %d non verificabili", unver)
	}
	return res, nil
}

// series checks one series: its seasons on TMDB, its packs on the tracker.
func (c *Checker) series(ctx context.Context, id int64, mine []seriesUpload) (*Group, error) {
	g := &Group{Kind: KindSeries, Key: "tmdb:" + strconv.FormatInt(id, 10), TmdbID: id, Title: displayTitle(mine[0].Name),
		Status: StatusOK, Units: []Unit{}, Rows: []Row{}, Mine: []Ref{},
		SearchURL: c.site + "/torrents?" + url.Values{"tmdbId": {strconv.FormatInt(id, 10)}}.Encode()}
	for _, u := range mine {
		if u.TvdbID != 0 {
			g.TvdbID = u.TvdbID
		}
		g.Mine = append(g.Mine, c.ref(u.ID, u.Name, u.DetailsLink, unit3d.QualityOf(u.Name, u.Type, u.Resolution, ""), true))
	}

	show, err := tmdbRetry(ctx, func() (tmdb.Show, error) { return c.tmdb.TV(ctx, id) })
	if errors.Is(err, tmdb.ErrNotFound) {
		g.Status, g.Note = StatusUnverifiable, fmt.Sprintf("serie con id TMDB %d non trovata su TMDB", id)
		return g, nil
	}
	if err != nil {
		return nil, fmt.Errorf("serie %d su TMDB: %w", id, err)
	}
	if show.Name != "" {
		g.Title = show.Name
	}
	g.Year = show.Year()
	today := c.now().Format("2006-01-02")
	for _, s := range show.Seasons {
		if s.Number > 0 && s.Aired(today) {
			name := s.Name
			if name == "" {
				name = "Stagione " + strconv.Itoa(s.Number)
			}
			g.Units = append(g.Units, Unit{Number: s.Number, Name: name, AirDate: s.AirDate, Episodes: s.Episodes})
		}
	}
	sort.Slice(g.Units, func(i, j int) bool { return g.Units[i].Number < g.Units[j].Number })
	if len(g.Units) == 0 {
		g.Status, g.Note = StatusUnverifiable, "nessuna stagione già uscita secondo TMDB"
		return g, nil
	}

	user, err := c.user(ctx)
	if err != nil {
		return nil, fmt.Errorf("nome utente sul tracker: %w", err)
	}
	listed, err := unit3d.WithRetry(ctx, func() ([]unit3d.Upload, error) { return c.tracker.ByTmdb(ctx, id, user) })
	if err != nil {
		return nil, fmt.Errorf("torrent della serie %d sul tracker: %w", id, err)
	}

	// TMDB numbers movies and series separately: keep the torrents in the
	// categories of the user's uploads of the series.
	cats := map[string]bool{}
	myIDs := map[int64]seriesUpload{}
	for _, u := range mine {
		if cat := strings.ToLower(strings.TrimSpace(u.Category)); cat != "" {
			cats[cat] = true
		}
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
		if !isMine && len(cats) > 0 && !cats[strings.ToLower(strings.TrimSpace(t.Category))] {
			continue
		}
		p, ok := unit3d.ParsePack(t.Name)
		if !ok {
			continue // an episode, or no season in the name
		}
		q := unit3d.QualityOf(t.Name, t.Type, t.Resolution, t.MediaInfo)
		items = append(items, item{ref: c.ref(t.ID, t.Name, t.DetailsLink, q, isMine), quality: q, units: covered(p, t.CreatedAt, g.Units)})
	}
	// The user's uploads the search did not return (just uploaded, or a
	// different TMDB id on the tracker).
	for _, u := range mine {
		if seen[u.ID] {
			continue
		}
		q := unit3d.QualityOf(u.Name, u.Type, u.Resolution, "")
		items = append(items, item{ref: c.ref(u.ID, u.Name, u.DetailsLink, q, true), quality: q, units: covered(u.pack, u.CreatedAt, g.Units)})
	}
	// The quality of the user's uploads is the one the search gave (with
	// the languages of the media info).
	for i, m := range g.Mine {
		for _, it := range items {
			if it.ref.ID == m.ID {
				g.Mine[i].Quality = it.quality.Label
			}
		}
	}
	g.buildRows(items)
	return g, nil
}

// covered is the seasons a pack holds among those of the series. A
// "Complete" pack holds those already out when it was uploaded.
func covered(p unit3d.Pack, createdAt string, units []Unit) map[int]bool {
	out := map[int]bool{}
	if p.Complete {
		day := ""
		if len(createdAt) >= 10 {
			day = createdAt[:10]
		}
		for _, u := range units {
			if day == "" || u.AirDate <= day {
				out[u.Number] = true
			}
		}
		return out
	}
	for _, n := range p.Seasons {
		out[n] = true
	}
	return out
}

func (c *Checker) ref(id int64, name, link string, q unit3d.Quality, mine bool) Ref {
	if link == "" && c.site != "" {
		link = c.site + "/torrents/" + strconv.FormatInt(id, 10)
	}
	return Ref{ID: id, Name: name, URL: link, Quality: q.Label, Mine: mine}
}

// displayTitle is the title of a release name with capital initials, for
// the series TMDB does not know.
func displayTitle(name string) string {
	words := strings.Fields(unit3d.Title(name))
	if len(words) == 0 {
		return name
	}
	for i, w := range words {
		r := []rune(w)
		words[i] = string(unicode.ToUpper(r[0])) + string(r[1:])
	}
	return strings.Join(words, " ")
}
