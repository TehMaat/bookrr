package gaps

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

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

	stateMu  sync.RWMutex
	state    State
	username string
}

type State struct {
	Running    bool    `json:"running"`
	LastRunAt  *string `json:"lastRunAt"`
	LastResult string  `json:"lastResult"`
	LastError  string  `json:"lastError"`
	NextRunAt  *string `json:"nextRunAt"`
	// Progress of the running check: what it is reading ("series",
	// "movies", "sagas") and how many of them are done out of the total.
	Phase string `json:"phase"`
	Done  int    `json:"done"`
	Total int    `json:"total"`
}

// New makes a checker that runs every day at hh:mm local time (at is in
// minutes after midnight). site is the address of the tracker.
func New(s *store.Store, tracker *unit3d.Client, t *tmdb.Client, site string, at int) *Checker {
	return &Checker{store: s, tracker: tracker, tmdb: t, site: site, at: at, now: time.Now, StartDelay: 10 * time.Minute,
		wake: make(chan struct{}, 1)}
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

// Wake starts a check now ("Controlla ora").
func (c *Checker) Wake() {
	c.update(func(s *State) { s.Running = true })
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
	for _, kind := range []string{KindSeries, KindSaga} {
		last, err := c.store.GapsCheckedAt(ctx, kind)
		if err != nil {
			continue
		}
		t, perr := time.Parse(time.RFC3339, deref(last))
		if last == nil || perr != nil || c.now().Sub(t) > 26*time.Hour {
			if soon := c.now().Add(c.StartDelay); soon.Before(next) {
				next = soon
			}
		}
	}
	for {
		at := next.UTC().Format(time.RFC3339)
		c.update(func(s *State) { s.NextRunAt = &at })
		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		case <-c.wake:
			timer.Stop()
		}
		c.Check(ctx)
		next = c.nextRun(c.now())
	}
}

// Check runs every check now and stores the results.
func (c *Checker) Check(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.update(func(s *State) { s.Running, s.LastError, s.Phase, s.Done, s.Total = true, "", "", 0, 0 })
	// Each check keeps its last results if it fails, the other goes on.
	var results, errs []string
	for _, ch := range []struct {
		name string
		run  func(context.Context) (string, error)
	}{{"stagioni mancanti", c.checkSeries}, {"film mancanti delle saghe", c.checkSagas}} {
		res, err := ch.run(ctx)
		if res != "" {
			results = append(results, res)
		}
		switch {
		case err != nil && ctx.Err() == nil:
			slog.Warn("controllo "+ch.name, "err", err)
			errs = append(errs, err.Error())
		case err == nil:
			slog.Info("controllo "+ch.name, "result", res)
		}
		if ctx.Err() != nil {
			break
		}
	}
	ts := c.now().UTC().Format(time.RFC3339)
	c.update(func(s *State) {
		s.Running, s.LastRunAt, s.LastResult, s.Phase = false, &ts, strings.Join(results, " · "), ""
		s.LastError = strings.Join(errs, "; ")
	})
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
