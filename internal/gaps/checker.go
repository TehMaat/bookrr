package gaps

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/tehmaat/bookrr/internal/mdblist"
	"github.com/tehmaat/bookrr/internal/store"
	"github.com/tehmaat/bookrr/internal/tmdb"
	"github.com/tehmaat/bookrr/internal/unit3d"
)

// Checker runs the checks every night at a set time and when asked
// ("Controlla ora"), one at a time, and keeps their results in the store.
type Checker struct {
	store   *store.Store
	tracker *unit3d.Client
	tmdb    *tmdb.Client
	// MDBList reads the lists the user follows; nil without a key (the
	// lists are not checked).
	MDBList *mdblist.Client
	site    string // address of the tracker, for the links
	// at is the time of day of the nightly check, minutes after midnight.
	at int
	// now is the current time, replaced by the tests.
	now func() time.Time
	// StartDelay is how long to wait after start before catching up on a
	// check missed while bookrr was off (the uploads are read first).
	StartDelay time.Duration

	wake chan struct{}
	mu   sync.Mutex // one check at a time
	// Under mu: the outcome of the last check of each kind, and the TMDB
	// ids of the IMDb ids asked during a check.
	results, errs map[string]string
	imdb          map[int64]int64

	stateMu  sync.RWMutex
	state    State
	username string
	// The kinds to check when woken, under stateMu.
	pending map[string]bool
}

type State struct {
	Running    bool    `json:"running"`
	LastRunAt  *string `json:"lastRunAt"`
	LastResult string  `json:"lastResult"`
	LastError  string  `json:"lastError"`
	NextRunAt  *string `json:"nextRunAt"`
	// Progress of the running check: what it is reading ("series",
	// "movies", "sagas", "lists") and how many of them are done out of the
	// total.
	Phase string `json:"phase"`
	Done  int    `json:"done"`
	Total int    `json:"total"`
}

// New makes a checker that runs every day at hh:mm local time (at is in
// minutes after midnight). site is the address of the tracker.
func New(s *store.Store, tracker *unit3d.Client, t *tmdb.Client, site string, at int) *Checker {
	return &Checker{store: s, tracker: tracker, tmdb: t, site: site, at: at, now: time.Now, StartDelay: 10 * time.Minute,
		wake: make(chan struct{}, 1), results: map[string]string{}, errs: map[string]string{}}
}

func (c *Checker) State() State {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.state
}

func (c *Checker) update(f func(*State)) {
	c.stateMu.Lock()
	f(&c.state)
	c.stateMu.Unlock()
}

// Wake starts a check of every kind now ("Controlla ora").
func (c *Checker) Wake() { c.WakeKinds(KindSeries, KindSaga, KindList) }

// WakeKinds starts a check of these kinds now (a list just added).
func (c *Checker) WakeKinds(kinds ...string) {
	c.update(func(s *State) {
		s.Running = true
		if c.pending == nil {
			c.pending = map[string]bool{}
		}
		for _, k := range kinds {
			c.pending[k] = true
		}
	})
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

// nextRun is the next time of the nightly check after t.
func (c *Checker) nextRun(t time.Time) time.Time {
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	next := day.Add(time.Duration(c.at) * time.Minute)
	if !next.After(t) {
		next = day.AddDate(0, 0, 1).Add(time.Duration(c.at) * time.Minute)
	}
	return next
}

// Run checks every night, when woken and, at start, if the last check is
// more than a day old (bookrr was off at night).
func (c *Checker) Run(ctx context.Context) {
	next := c.nextRun(c.now())
	kinds := []string{KindSeries, KindSaga}
	if c.MDBList != nil {
		kinds = append(kinds, KindList)
	}
	for _, kind := range kinds {
		last, err := c.store.GapsCheckedAt(ctx, kind)
		if err != nil {
			continue
		}
		t, perr := time.Parse(time.RFC3339, deref(last))
		if last == nil || perr != nil || c.now().Sub(t) > 26*time.Hour || c.outdated(ctx, kind) {
			if soon := c.now().Add(c.StartDelay); soon.Before(next) {
				next = soon
			}
		}
	}
	for {
		at := next.UTC().Format(time.RFC3339)
		c.update(func(s *State) { s.NextRunAt = &at })
		timer := time.NewTimer(time.Until(next))
		var only map[string]bool
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		case <-c.wake:
			timer.Stop()
			c.update(func(s *State) { only, c.pending = c.pending, nil })
		}
		c.check(ctx, only)
		// A check of some kinds only (a list just added) leaves the next one
		// as it was, catching up on the others.
		if only == nil || len(only) == len(checks) {
			next = c.nextRun(c.now())
		}
	}
}

// outdated reports whether the stored results of a kind come from an
// older shape of the check (see Format).
func (c *Checker) outdated(ctx context.Context, kind string) bool {
	groups, err := c.store.GapGroups(ctx, kind)
	if err != nil {
		return false
	}
	for _, g := range groups {
		var f struct {
			Format int    `json:"format"`
			Status string `json:"status"`
		}
		if json.Unmarshal(g.Data, &f) == nil && f.Status == StatusOK && f.Format < Format {
			return true
		}
	}
	return false
}

// checks are the checks of each kind, in the order they run.
var checks = []struct {
	kind, name string
	run        func(*Checker, context.Context) (string, error)
}{
	{KindSeries, "stagioni mancanti", (*Checker).checkSeries},
	{KindSaga, "film mancanti delle saghe", (*Checker).checkSagas},
	{KindList, "film mancanti delle collezioni", (*Checker).checkLists},
}

// Check runs every check now and stores the results.
func (c *Checker) Check(ctx context.Context) { c.check(ctx, nil) }

// check runs the checks of these kinds (every kind if nil) and stores the
// results. Each check keeps its last results if it fails, the others go on.
func (c *Checker) check(ctx context.Context, kinds map[string]bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.update(func(s *State) { s.Running, s.Phase, s.Done, s.Total = true, "", 0, 0 })
	c.imdb = nil
	for _, ch := range checks {
		if kinds != nil && !kinds[ch.kind] {
			continue
		}
		res, err := ch.run(c, ctx)
		if res != "" {
			c.results[ch.kind] = res
		}
		switch {
		case err != nil && ctx.Err() == nil:
			slog.Warn("controllo "+ch.name, "err", err)
			c.errs[ch.kind] = err.Error()
		case err == nil:
			if res != "" {
				slog.Info("controllo "+ch.name, "result", res)
			}
			delete(c.errs, ch.kind)
		}
		if ctx.Err() != nil {
			break
		}
	}
	var results, errs []string
	for _, ch := range checks {
		if r := c.results[ch.kind]; r != "" {
			results = append(results, r)
		}
		if e := c.errs[ch.kind]; e != "" {
			errs = append(errs, e)
		}
	}
	ts := c.now().UTC().Format(time.RFC3339)
	c.update(func(s *State) {
		s.Running, s.LastRunAt, s.LastResult, s.Phase = false, &ts, strings.Join(results, " · "), ""
		s.LastError = strings.Join(errs, "; ")
		// Asked for while this one ran: it runs next.
		if len(c.pending) > 0 {
			s.Running = true
		}
	})
}

// save stores the groups of a check of a kind, and counts the parts
// missing and the groups that cannot be checked.
func (c *Checker) save(ctx context.Context, kind string, groups []*Group) (missing, unverifiable int, err error) {
	ts := c.now().UTC().Format(time.RFC3339)
	rows := make([]store.GapGroup, len(groups))
	for i, g := range groups {
		g.CheckedAt = ts
		data, err := json.Marshal(g)
		if err != nil {
			return 0, 0, err
		}
		rows[i] = store.GapGroup{Key: g.Key, Title: g.Title, Missing: g.Missing, Data: data, CheckedAt: ts}
		missing += g.Missing
		if g.Status == StatusUnverifiable {
			unverifiable++
		}
	}
	return missing, unverifiable, c.store.SaveGapGroups(ctx, kind, rows)
}

// progress starts a phase of the check with total things to read.
func (c *Checker) progress(phase string, total int) {
	c.update(func(s *State) { s.Phase, s.Done, s.Total = phase, 0, total })
}

// user is the name the tracker's API key belongs to, asked once.
func (c *Checker) user(ctx context.Context) (string, error) {
	if c.username != "" {
		return c.username, nil
	}
	name, err := unit3d.WithRetry(ctx, func() (string, error) { return c.tracker.Username(ctx) })
	if err != nil {
		return "", err
	}
	c.username = name
	return name, nil
}

// tmdbRetry waits and tries again while TMDB asks to slow down.
func tmdbRetry[T any](ctx context.Context, f func() (T, error)) (T, error) {
	for {
		v, err := f()
		var rl *tmdb.RateLimitError
		if !errors.As(err, &rl) {
			return v, err
		}
		t := time.NewTimer(rl.RetryAfter)
		select {
		case <-ctx.Done():
			t.Stop()
			return v, ctx.Err()
		case <-t.C:
		}
	}
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
