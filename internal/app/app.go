// Package app runs what depends on the configuration (the readings of the
// clients, of the buckets and of the tracker, the checks) and starts again
// the parts whose options changed when the configuration is saved from the
// web UI, without restarting bookrr.
package app

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/tehmaat/bookrr/internal/api"
	"github.com/tehmaat/bookrr/internal/bucket"
	"github.com/tehmaat/bookrr/internal/config"
	"github.com/tehmaat/bookrr/internal/gaps"
	"github.com/tehmaat/bookrr/internal/mdblist"
	"github.com/tehmaat/bookrr/internal/prowlarr"
	"github.com/tehmaat/bookrr/internal/store"
	"github.com/tehmaat/bookrr/internal/syncer"
	"github.com/tehmaat/bookrr/internal/tmdb"
	"github.com/tehmaat/bookrr/internal/unit3d"
)

// stopWait is how long a part is given to stop before the new one starts
// anyway (it was asked to stop: it only finishes what it was writing).
const stopWait = 30 * time.Second

type Runner struct {
	ctx   context.Context
	store *store.Store

	mu      sync.Mutex
	svc     api.Services
	started bool
	// One client for every part that reads the tracker, so together they
	// respect its rate limit.
	tracker *unit3d.Client
	stops   map[string]func()
}

// New makes a runner whose parts stop with ctx.
func New(ctx context.Context, s *store.Store) *Runner {
	return &Runner{ctx: ctx, store: s, stops: map[string]func(){}}
}

// Apply starts the parts with cfg: at the first call all of them, then only
// those whose options changed. It returns the services now running.
func (r *Runner) Apply(cfg config.Config) api.Services {
	r.mu.Lock()
	defer r.mu.Unlock()
	old, svc, first := r.svc.Config, r.svc, !r.started
	r.started = true
	svc.Config = cfg
	changed := func(f func(config.Config) any) bool { return first || f(cfg) != f(old) }

	if changed(func(c config.Config) any { return [2]any{c.ReleaseTag, c.SyncInterval} }) {
		svc.Syncer = syncer.New(r.store, cfg.ReleaseTag, cfg.SyncInterval)
		r.start("sync", svc.Syncer.Run)
	}
	if changed(func(c config.Config) any { return [2]any{c.ReleaseTag, c.S3ScanInterval} }) {
		svc.Buckets = bucket.New(r.store, cfg.ReleaseTag, cfg.S3ScanInterval)
		r.start("buckets", svc.Buckets.Run)
	}

	tracker := changed(func(c config.Config) any { return [2]string{c.Unit3DURL, c.Unit3DAPIKey} })
	if tracker {
		r.tracker = nil
		if cfg.Unit3DURL != "" {
			r.tracker = unit3d.New(cfg.Unit3DURL, cfg.Unit3DAPIKey)
		}
	}
	if tracker || changed(func(c config.Config) any { return c.Unit3DReleasesInterval }) {
		r.stop("hashes")
		r.stop("history")
		svc.Hashes, svc.History = nil, nil
		if r.tracker != nil {
			svc.Hashes = unit3d.NewResolver(r.store, r.tracker)
			r.start("hashes", svc.Hashes.Run)
			svc.History = unit3d.NewHistory(r.store, r.tracker, cfg.Unit3DReleasesInterval)
			r.start("history", svc.History.Run)
		}
	}
	if tracker || changed(func(c config.Config) any { return [3]any{c.TMDBAPIKey, c.MDBListAPIKey, c.MissingCheckAt} }) {
		r.stop("missing")
		svc.Missing = nil
		if r.tracker != nil && cfg.TMDBAPIKey != "" {
			svc.Missing = gaps.New(r.store, r.tracker, tmdb.New(cfg.TMDBAPIKey), cfg.Unit3DURL, cfg.MissingCheckAt)
			if cfg.MDBListAPIKey != "" {
				svc.Missing.MDBList = mdblist.New(cfg.MDBListAPIKey)
			}
			r.start("missing", svc.Missing.Run)
		}
	}
	// The films searched are the user's uploads on the tracker.
	if tracker || changed(func(c config.Config) any { return [3]any{c.ProwlarrURL, c.ProwlarrAPIKey, c.ProwlarrRecheck} }) {
		r.stop("best")
		svc.Best = nil
		if r.tracker != nil && cfg.ProwlarrURL != "" {
			svc.Best = prowlarr.NewFinder(r.store, prowlarr.New(cfg.ProwlarrURL, cfg.ProwlarrAPIKey), cfg.ProwlarrRecheck)
			r.start("best", svc.Best.Run)
		}
	}
	r.svc = svc
	return svc
}

// start runs a part until it is stopped (or ctx is done), stopping the one
// of the same name first.
func (r *Runner) start(name string, run func(context.Context)) {
	r.stop(name)
	ctx, cancel := context.WithCancel(r.ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		run(ctx)
	}()
	r.stops[name] = func() {
		cancel()
		t := time.NewTimer(stopWait)
		defer t.Stop()
		select {
		case <-done:
		case <-t.C:
			slog.Warn("servizio non fermato in tempo, avvio comunque il nuovo", "servizio", name)
		}
	}
}

func (r *Runner) stop(name string) {
	if f := r.stops[name]; f != nil {
		f()
		delete(r.stops, name)
	}
}
