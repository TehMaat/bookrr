package prowlarr

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/tehmaat/bookrr/internal/store"
	"github.com/tehmaat/bookrr/internal/unit3d"
)

// MaxRank is the resolution past which a film is not searched: the user
// who has it in a 2160p Remux (or Full Disc) already has the best there is.
var MaxRank = unit3d.ResolutionRank("2160p")

// Ranking is the version of the order that chose the stored releases: the
// films searched with an older one are searched again first.
// 1: type, then resolution (a 1080p Remux is better than a 2160p WEB-DL).
const Ranking = 1

// grade is what makes a release of a film better than another: its type
// first (Remux > Encode > WEB-DL > WEBRip > HDTV), then its resolution.
type grade struct{ typ, res string }

func gradeOf(name, typ, resolution string) grade {
	q := unit3d.QualityOf(name, typ, resolution, "")
	return grade{q.Type, q.Resolution}
}

// typeTier is the rank of a type, a Full Disc counting as a Remux: same
// video and audio, and the names on the indexers do not tell them apart.
func typeTier(t string) int {
	return min(unit3d.TypeRank(t), unit3d.TypeRank("Remux"))
}

// better reports whether g is better than h.
func (g grade) better(h grade) bool {
	if x, y := typeTier(g.typ), typeTier(h.typ); x != y {
		return x > y
	}
	return unit3d.ResolutionRank(g.res) > unit3d.ResolutionRank(h.res)
}

// top reports whether nothing can be better than g.
func (g grade) top() bool {
	return typeTier(g.typ) >= typeTier("Remux") && unit3d.ResolutionRank(g.res) >= MaxRank
}

// An upscale is not a real release in that resolution.
var upscaleRe = regexp.MustCompile(`(?i)upscal`)

// Finder searches Prowlarr once a day for the films the user uploaded
// whose last search is older than recheck, slowly (see Client.MinInterval),
// and keeps the best resolution found in the store.
type Finder struct {
	store   *store.Store
	client  *Client
	recheck time.Duration
	// now is the current time, replaced by the tests.
	now func() time.Time
	// StartDelay is how long to wait after start before the first round
	// (the uploads are read first).
	StartDelay time.Duration

	mu sync.Mutex // one round at a time

	stateMu sync.RWMutex
	state   State
}

type State struct {
	Running    bool    `json:"running"`
	LastRunAt  *string `json:"lastRunAt"`
	LastResult string  `json:"lastResult"`
	LastError  string  `json:"lastError"`
	NextRunAt  *string `json:"nextRunAt"`
	// Progress of the running round: films searched out of those due.
	Done  int `json:"done"`
	Total int `json:"total"`
	// How often each film is searched again, as "7d" or "36h".
	Recheck string `json:"recheck"`
}

func NewFinder(s *store.Store, c *Client, recheck time.Duration) *Finder {
	f := &Finder{store: s, client: c, recheck: recheck, now: time.Now, StartDelay: 15 * time.Minute}
	f.state.Recheck = days(recheck)
	return f
}

func days(d time.Duration) string {
	if d%(24*time.Hour) == 0 {
		return strconv.Itoa(int(d/(24*time.Hour))) + "d"
	}
	return strconv.Itoa(int(d/time.Hour)) + "h"
}

func (f *Finder) State() State {
	f.stateMu.RLock()
	defer f.stateMu.RUnlock()
	return f.state
}

func (f *Finder) update(fn func(*State)) {
	f.stateMu.Lock()
	fn(&f.state)
	f.stateMu.Unlock()
}

// Run searches StartDelay after start, then once a day.
func (f *Finder) Run(ctx context.Context) {
	wait := f.StartDelay
	for {
		next := f.now().Add(wait).UTC().Format(time.RFC3339)
		f.update(func(s *State) { s.NextRunAt = &next })
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
		f.Check(ctx)
		wait = 24 * time.Hour
	}
}

// film is a film the user uploaded, with what is needed to search it and
// to tell its releases from those of other films.
type film struct {
	key        string
	imdb, tmdb int64
	titles     map[string]bool
	years      map[int]bool
	mine       grade // best of the user's uploads
}

// FilmKey identifies a film: by TMDB id, else by IMDb id; "" if neither.
func FilmKey(tmdbID, imdbID int64) string {
	switch {
	case tmdbID != 0:
		return "tmdb:" + strconv.FormatInt(tmdbID, 10)
	case imdbID != 0:
		return "imdb:" + strconv.FormatInt(imdbID, 10)
	}
	return ""
}

// films groups the user's uploads that are films by film.
func films(ups []store.TrackerUpload) []*film {
	byKey := map[string]*film{}
	var out []*film
	for _, u := range ups {
		key := FilmKey(u.TmdbID, u.ImdbID)
		if key == "" || !unit3d.IsMovie(u.Category, u.Name, u.TvdbID) {
			continue
		}
		f := byKey[key]
		if f == nil {
			f = &film{key: key, titles: map[string]bool{}, years: map[int]bool{}}
			byKey[key] = f
			out = append(out, f)
		}
		if f.imdb == 0 {
			f.imdb = u.ImdbID
		}
		if f.tmdb == 0 {
			f.tmdb = u.TmdbID
		}
		if t := unit3d.Title(u.Name); t != "" {
			f.titles[t] = true
		}
		if y, err := strconv.Atoi(unit3d.Year(u.Name)); err == nil {
			f.years[y] = true
		}
		if g := gradeOf(u.Name, u.Type, u.Resolution); g.better(f.mine) {
			f.mine = g
		}
	}
	return out
}

// matches reports whether a release found is of the film: by the ids the
// indexer gives or, without them, by title and year (an indexer that does
// not search by id may answer with anything).
func (f *film) matches(r Release) bool {
	if r.TmdbID != 0 && f.tmdb != 0 {
		return r.TmdbID == f.tmdb
	}
	if r.ImdbID != 0 && f.imdb != 0 {
		return r.ImdbID == f.imdb
	}
	if t := unit3d.Title(r.Title); t == "" || !f.titles[t] {
		return false
	}
	y, err := strconv.Atoi(unit3d.Year(r.Title))
	return err == nil && (f.years[y] || f.years[y-1] || f.years[y+1])
}

// best is the best release of the film with a known resolution (by type,
// then resolution; the most seeded among the equal ones), and how many
// releases of the film were found.
func (f *film) best(found []Release) store.BestResolution {
	b := store.BestResolution{Film: f.key, Ranking: Ranking}
	var top grade
	var seeders int64
	for _, r := range found {
		if !f.matches(r) || upscaleRe.MatchString(r.Title) {
			continue
		}
		b.Results++
		g := gradeOf(r.Title, "", "")
		if unit3d.ResolutionRank(g.res) == 0 {
			continue
		}
		if b.Resolution != "" && (top.better(g) || (!g.better(top) && r.Seeders <= seeders)) {
			continue
		}
		top, seeders = g, r.Seeders
		b.Resolution, b.Type, b.Name, b.Indexer, b.URL = g.res, g.typ, r.Title, r.Indexer, r.InfoURL
	}
	return b
}

// Check searches now the films whose last search is older than recheck,
// those never searched first. A failing search stops the round: the
// films left are searched in the next one.
func (f *Finder) Check(ctx context.Context) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.update(func(s *State) { s.Running, s.Done, s.Total = true, 0, 0 })
	res, err := f.check(ctx)
	ts := f.now().UTC().Format(time.RFC3339)
	f.update(func(s *State) {
		s.Running, s.LastRunAt, s.LastResult, s.LastError = false, &ts, res, ""
		if err != nil {
			s.LastError = err.Error()
		}
	})
	switch {
	case err != nil && ctx.Err() == nil:
		slog.Warn("risoluzione massima con Prowlarr", "err", err)
	case err == nil:
		slog.Info("risoluzione massima con Prowlarr", "result", res)
	}
}

func (f *Finder) check(ctx context.Context) (string, error) {
	ups, err := f.store.ActiveTrackerUploads(ctx)
	if err != nil {
		return "", err
	}
	prev, err := f.store.BestResolutions(ctx)
	if err != nil {
		return "", err
	}
	now := f.now()
	var due []*film
	for _, fm := range films(ups) {
		if fm.mine.top() {
			continue
		}
		p, ok := prev[fm.key]
		if t, err := time.Parse(time.RFC3339, p.CheckedAt); !ok || err != nil || p.Ranking < Ranking || now.Sub(t) >= f.recheck {
			due = append(due, fm)
		}
	}
	// Never searched first, then searched with an older ranking, then the
	// oldest searches.
	sort.SliceStable(due, func(i, j int) bool {
		a, b := prev[due[i].key], prev[due[j].key]
		if (a.CheckedAt == "") != (b.CheckedAt == "") {
			return a.CheckedAt == ""
		}
		if (a.Ranking < Ranking) != (b.Ranking < Ranking) {
			return a.Ranking < Ranking
		}
		return a.CheckedAt < b.CheckedAt
	})
	f.update(func(s *State) { s.Total = len(due) })
	better := 0
	for i, fm := range due {
		found, err := f.client.SearchMovie(ctx, fm.imdb, fm.tmdb)
		if err != nil {
			return summary(i, better), err
		}
		b := fm.best(found)
		b.CheckedAt = f.now().UTC().Format(time.RFC3339)
		if err := f.store.SaveBestResolution(ctx, b); err != nil {
			return summary(i, better), err
		}
		if fm.found(b).better(fm.mine) {
			better++
		}
		f.update(func(s *State) { s.Done = i + 1 })
	}
	return summary(len(due), better), nil
}

func summary(searched, better int) string {
	return fmt.Sprintf("%d film cercati, %d in una versione migliore della tua", searched, better)
}

// found is the grade of the release found, none if nothing was.
func (f *film) found(b store.BestResolution) grade {
	if b.Resolution == "" {
		return grade{}
	}
	return gradeOf(b.Name, "", b.Resolution)
}

// Annotate gives the releases that are films the best release found for
// them, and whether it is better than the best of the user's uploads of
// the film still on the tracker. A film the user has in a 2160p Remux is
// not searched: it gets its own resolution and type, with no CheckedAt.
func Annotate(releases []store.TrackerRelease, found map[string]store.BestResolution) {
	var active []store.TrackerUpload
	for _, r := range releases {
		if r.RemovedAt == nil {
			active = append(active, store.TrackerUpload{Name: r.Name, Category: r.Category, Type: r.Type, Resolution: r.Resolution,
				TmdbID: r.TmdbID, ImdbID: r.ImdbID, TvdbID: r.TvdbID})
		}
	}
	mine := map[string]*film{}
	for _, fm := range films(active) {
		mine[fm.key] = fm
	}
	for i, r := range releases {
		key := FilmKey(r.TmdbID, r.ImdbID)
		if key == "" || !unit3d.IsMovie(r.Category, r.Name, r.TvdbID) {
			continue
		}
		fm, ok := mine[key]
		if !ok {
			// Every upload of the film was removed from the tracker.
			fm = &film{key: key, mine: gradeOf(r.Name, r.Type, r.Resolution)}
		}
		b, ok := found[key]
		switch {
		case ok:
		case fm.mine.top():
			b = store.BestResolution{Film: key}
		default:
			continue
		}
		g := fm.found(b)
		b.Type = g.typ
		b.Mine, b.MineType = fm.mine.res, fm.mine.typ
		b.Better = g.better(fm.mine)
		releases[i].Best = &b
	}
}
