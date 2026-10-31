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
// who has it in 2160p already has the best there is.
var MaxRank = unit3d.ResolutionRank("2160p")

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
	mine       string // best resolution of the user's uploads
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

func resolutionOf(name, resolution string) string {
	return unit3d.QualityOf(name, "", resolution, "").Resolution
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
		if r := resolutionOf(u.Name, u.Resolution); unit3d.ResolutionRank(r) > unit3d.ResolutionRank(f.mine) {
			f.mine = r
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

// best is the release of the film with the highest resolution (the most
// seeded among them), and how many releases of the film were found.
func (f *film) best(found []Release) store.BestResolution {
	b := store.BestResolution{Film: f.key}
	var seeders int64
	for _, r := range found {
		if !f.matches(r) || upscaleRe.MatchString(r.Title) {
			continue
		}
		b.Results++
		res := resolutionOf(r.Title, "")
		x, y := unit3d.ResolutionRank(res), unit3d.ResolutionRank(b.Resolution)
		if x == 0 || x < y || (x == y && r.Seeders <= seeders) {
			continue
		}
		b.Resolution, b.Name, b.Indexer, b.URL, seeders = res, r.Title, r.Indexer, r.InfoURL, r.Seeders
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
		if unit3d.ResolutionRank(fm.mine) >= MaxRank {
			continue
		}
		p, ok := prev[fm.key]
		if t, err := time.Parse(time.RFC3339, p.CheckedAt); !ok || err != nil || now.Sub(t) >= f.recheck {
			due = append(due, fm)
		}
	}
	sort.SliceStable(due, func(i, j int) bool { return prev[due[i].key].CheckedAt < prev[due[j].key].CheckedAt })
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
		if unit3d.ResolutionRank(b.Resolution) > unit3d.ResolutionRank(fm.mine) {
			better++
		}
		f.update(func(s *State) { s.Done = i + 1 })
	}
	return summary(len(due), better), nil
}

func summary(searched, better int) string {
	return fmt.Sprintf("%d film cercati, %d in una risoluzione migliore della tua", searched, better)
}

// Annotate gives the releases that are films the best resolution found
// for them, and whether it is better than the best of the user's uploads
// of the film still on the tracker. A film the user has in 2160p is not
// searched: it gets its own resolution, with no CheckedAt.
func Annotate(releases []store.TrackerRelease, found map[string]store.BestResolution) {
	var active []store.TrackerUpload
	for _, r := range releases {
		if r.RemovedAt == nil {
			active = append(active, store.TrackerUpload{Name: r.Name, Category: r.Category, Resolution: r.Resolution,
				TmdbID: r.TmdbID, ImdbID: r.ImdbID, TvdbID: r.TvdbID})
		}
	}
	mine := map[string]string{}
	for _, fm := range films(active) {
		mine[fm.key] = fm.mine
	}
	for i, r := range releases {
		key := FilmKey(r.TmdbID, r.ImdbID)
		if key == "" || !unit3d.IsMovie(r.Category, r.Name, r.TvdbID) {
			continue
		}
		m, ok := mine[key]
		if !ok {
			// Every upload of the film was removed from the tracker.
			m = resolutionOf(r.Name, r.Resolution)
		}
		b, ok := found[key]
		switch {
		case ok:
		case unit3d.ResolutionRank(m) >= MaxRank:
			b = store.BestResolution{Film: key}
		default:
			continue
		}
		b.Mine = m
		b.Better = unit3d.ResolutionRank(b.Resolution) > unit3d.ResolutionRank(m)
		releases[i].Best = &b
	}
}
