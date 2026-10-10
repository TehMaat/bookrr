package unit3d

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/tehmaat/bookrr/internal/store"
)

// Resolver gives torrents added without an info hash their real one, by
// looking their name up on the tracker in the background.
type Resolver struct {
	store  *store.Store
	client *Client
	// RetryAfter is how long a torrent that was not found waits before
	// being looked up again.
	RetryAfter time.Duration

	wake chan struct{}
	mu   sync.Mutex // one lookup at a time
}

func NewResolver(s *store.Store, c *Client) *Resolver {
	return &Resolver{store: s, client: c, RetryAfter: 24 * time.Hour, wake: make(chan struct{}, 1)}
}

// Wake starts a round now, e.g. right after an import.
func (r *Resolver) Wake() {
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

// Run looks up pending torrents at start, when woken and every hour.
func (r *Resolver) Run(ctx context.Context) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		r.runPending(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-r.wake:
		}
	}
}

func (r *Resolver) runPending(ctx context.Context) {
	found, failed := 0, 0
	defer func() {
		if found+failed > 0 {
			slog.Info("ricerca hash su UNIT3D", "trovati", found, "non trovati", failed)
		}
	}()
	for ctx.Err() == nil {
		pending, err := r.store.PendingHashLookups(ctx, time.Now().Add(-r.RetryAfter), 20)
		if err != nil {
			slog.Error("ricerca hash", "err", err)
			return
		}
		if len(pending) == 0 {
			return
		}
		for _, p := range pending {
			_, err := r.resolve(ctx, p)
			var rl *RateLimitError
			switch {
			case err == nil:
				found++
			case Final(err), errors.Is(err, store.ErrNotFound):
				failed++
			case errors.As(err, &rl):
				slog.Warn("ricerca hash: limite del tracker", "attesa", rl.RetryAfter)
				if !sleep(ctx, rl.RetryAfter) {
					return
				}
				// The same torrent is still pending: the next batch retries it.
			default:
				// Tracker unreachable or misconfigured: try again later.
				if ctx.Err() == nil {
					slog.Warn("ricerca hash interrotta", "err", err)
				}
				return
			}
		}
	}
}

// Resolve looks up one torrent now and returns its new hash.
func (r *Resolver) Resolve(ctx context.Context, hash string) (string, error) {
	t, err := r.store.GetTorrent(ctx, hash)
	if err != nil {
		return "", err
	}
	return r.resolve(ctx, store.HashLookup{Hash: t.Hash, Name: t.Name, Size: t.Size})
}

func (r *Resolver) resolve(ctx context.Context, p store.HashLookup) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	m, err := r.client.FindHash(ctx, p.Name, p.Size)
	if err != nil {
		var rl *RateLimitError
		if ctx.Err() == nil && !errors.As(err, &rl) {
			if serr := r.store.SetHashLookupError(ctx, p.Hash, err.Error(), Final(err)); serr != nil {
				return "", serr
			}
		}
		return "", err
	}
	merged, err := r.store.ReplaceHash(ctx, p.Hash, m.Hash)
	if err != nil {
		return "", err
	}
	slog.Info("hash trovato su UNIT3D", "name", p.Name, "hash", m.Hash, "trackerId", m.ID, "unito", merged)
	return m.Hash, nil
}

func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
