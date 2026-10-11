package unit3d

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/tehmaat/bookrr/internal/store"
)

// History keeps the list of the user's uploads on the tracker up to date
// and links each of them to the torrent of bookrr with the same hash.
type History struct {
	store    *store.Store
	client   *Client
	interval time.Duration

	wake chan struct{}
	mu   sync.Mutex // one round at a time

	stateMu  sync.RWMutex
	state    HistoryState
	username string
}

type HistoryState struct {
	Running    bool    `json:"running"`
	LastRunAt  *string `json:"lastRunAt"`
	LastResult string  `json:"lastResult"`
	LastError  string  `json:"lastError"`
	NextRunAt  *string `json:"nextRunAt"`
	Username   string  `json:"username"`
	// Uploads whose .torrent file is still to be downloaded in this round.
	Pending int `json:"pending"`
}

func NewHistory(s *store.Store, c *Client, interval time.Duration) *History {
	return &History{store: s, client: c, interval: interval, wake: make(chan struct{}, 1)}
}

func (h *History) State() HistoryState {
	h.stateMu.RLock()
	defer h.stateMu.RUnlock()
	return h.state
}

func (h *History) update(f func(*HistoryState)) {
	h.stateMu.Lock()
	f(&h.state)
	h.stateMu.Unlock()
}

// Wake starts a round now ("Aggiorna ora").
func (h *History) Wake() {
	h.update(func(s *HistoryState) { s.Running = true })
	select {
	case h.wake <- struct{}{}:
	default:
	}
}

// Run reads the uploads at start, when woken and at every interval.
func (h *History) Run(ctx context.Context) {
	t := time.NewTimer(0)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-h.wake:
			if !t.Stop() {
				select {
				case <-t.C:
				default:
				}
			}
		}
		h.Sync(ctx)
		next := time.Now().Add(h.interval).UTC().Format(time.RFC3339)
		h.update(func(s *HistoryState) { s.NextRunAt = &next })
		t.Reset(h.interval)
	}
}

// Sync runs one round: list the uploads, refresh their counters, compute
// the hash of the new ones and link the torrents added without a hash.
func (h *History) Sync(ctx context.Context) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.update(func(s *HistoryState) { s.Running = true; s.LastError = "" })
	res, err := h.sync(ctx)
	ts := time.Now().UTC().Format(time.RFC3339)
	h.update(func(s *HistoryState) {
		s.Running, s.Pending, s.LastRunAt, s.LastResult = false, 0, &ts, res
		if err != nil {
			s.LastError = err.Error()
		}
	})
	switch {
	case err != nil && ctx.Err() == nil:
		slog.Warn("storico release UNIT3D", "err", err)
	case err == nil:
		slog.Info("storico release UNIT3D", "result", res)
	}
}

func (h *History) sync(ctx context.Context) (string, error) {
	name := h.State().Username
	if name == "" {
		var err error
		if name, err = withRetry(ctx, func() (string, error) { return h.client.Username(ctx) }); err != nil {
			return "", fmt.Errorf("nome utente sul tracker: %w", err)
		}
		h.update(func(s *HistoryState) { s.Username = name })
	}
	uploads, err := withRetry(ctx, func() ([]Upload, error) { return h.client.Uploads(ctx, name) })
	if err != nil {
		return "", fmt.Errorf("elenco degli upload: %w", err)
	}
	rows := make([]store.TrackerUpload, len(uploads))
	byID := make(map[int64]Upload, len(uploads))
	for i, u := range uploads {
		rows[i] = store.TrackerUpload{
			ID: u.ID, Name: u.Name, Category: u.Category, Type: u.Type, TypeID: u.TypeID, Resolution: u.Resolution,
			ResolutionID: u.ResolutionID, Size: u.Size, Seeders: u.Seeders, Leechers: u.Leechers, TimesCompleted: u.TimesCompleted,
			TmdbID: u.TmdbID, ImdbID: u.ImdbID, TvdbID: u.TvdbID, PersonalRelease: u.PersonalRelease,
			CreatedAt: u.CreatedAt, DetailsLink: u.DetailsLink,
		}
		byID[u.ID] = u
	}
	added, err := h.store.SaveTrackerReleases(ctx, rows)
	if err != nil {
		return "", err
	}
	parts := []string{fmt.Sprintf("%d release", len(uploads))}
	if added > 0 {
		parts = append(parts, fmt.Sprintf("%d nuove", added))
	}

	hashed, failed, herr := h.fetchHashes(ctx, byID)
	if hashed > 0 {
		parts = append(parts, fmt.Sprintf("%d hash calcolati", hashed))
	}
	if failed > 0 {
		parts = append(parts, fmt.Sprintf("%d hash non calcolati", failed))
	}
	if herr != nil {
		return strings.Join(parts, ", "), herr
	}

	linked, err := h.linkByName(ctx)
	if err != nil {
		return strings.Join(parts, ", "), err
	}
	if linked > 0 {
		parts = append(parts, fmt.Sprintf("%d torrent senza hash collegati per nome", linked))
	}
	return strings.Join(parts, ", "), h.store.LinkTrackerReleases(ctx)
}

// fetchHashes downloads the .torrent file of the uploads whose hash is not
// known yet. A file that cannot be downloaded is retried in the next round;
// a tracker that keeps failing stops the round.
func (h *History) fetchHashes(ctx context.Context, byID map[int64]Upload) (hashed, failed int, err error) {
	ids, err := h.store.TrackerReleasesWithoutHash(ctx)
	if err != nil {
		return 0, 0, err
	}
	inARow := 0
	for i, id := range ids {
		u, ok := byID[id]
		if !ok {
			continue
		}
		h.update(func(s *HistoryState) { s.Pending = len(ids) - i })
		type result struct {
			hash string
			file []byte
		}
		r, err := withRetry(ctx, func() (result, error) {
			hash, file, err := h.client.UploadHash(ctx, u)
			return result{hash, file}, err
		})
		if ctx.Err() != nil {
			return hashed, failed, ctx.Err()
		}
		if err != nil {
			failed++
			if serr := h.store.SetTrackerReleaseHashError(ctx, id, err.Error()); serr != nil {
				return hashed, failed, serr
			}
			if !Final(err) {
				if inARow++; inARow >= 3 {
					return hashed, failed, fmt.Errorf("download dei .torrent interrotto: %w", err)
				}
			}
			continue
		}
		inARow = 0
		if err := h.store.SetTrackerReleaseHash(ctx, id, r.hash, r.file); err != nil {
			return hashed, failed, err
		}
		hashed++
	}
	return hashed, failed, nil
}

// linkByName gives the torrents added without a hash the hash of the
// upload with the same name (or the same release written differently,
// as the Resolver does), and returns how many were linked.
func (h *History) linkByName(ctx context.Context) (int, error) {
	manual, err := h.store.ManualTorrents(ctx)
	if err != nil || len(manual) == 0 {
		return 0, err
	}
	cands, err := h.store.ReleaseCandidates(ctx)
	if err != nil {
		return 0, err
	}
	linked := 0
	for _, m := range manual {
		c, ok := matchRelease(m.Name, m.Size, cands)
		if !ok {
			continue
		}
		merged, err := h.store.ReplaceHash(ctx, m.Hash, store.TrackerTorrent{Hash: c.Hash, URL: c.DetailsLink})
		if errors.Is(err, store.ErrNotFound) {
			continue // just resolved by the Resolver
		}
		if err != nil {
			return linked, err
		}
		linked++
		slog.Info("torrent collegato a una release del tracker", "name", m.Name, "tracker", c.Name, "hash", c.Hash, "trackerId", c.ID, "unito", merged)
	}
	return linked, nil
}

// matchRelease picks the upload a torrent without hash refers to: the one
// with the same name, otherwise the only one that is the same release
// (see sameRelease) with a size within 5%. Equal candidates are told apart
// by size, then by being still on the tracker; otherwise nothing is chosen.
func matchRelease(name string, size int64, cs []store.ReleaseCandidate) (store.ReleaseCandidate, bool) {
	want := normName(name)
	var same []store.ReleaseCandidate
	for _, c := range cs {
		if normName(c.Name) == want {
			same = append(same, c)
		}
	}
	if len(same) == 0 {
		for _, c := range cs {
			if sameRelease(name, c.Name) && sizeGap(c.Size, size) <= 0.05 {
				same = append(same, c)
			}
		}
	}
	narrow := func(keep func(store.ReleaseCandidate) bool) {
		var out []store.ReleaseCandidate
		for _, c := range same {
			if keep(c) {
				out = append(out, c)
			}
		}
		if len(out) > 0 {
			same = out
		}
	}
	if len(same) > 1 && size > 0 {
		narrow(func(c store.ReleaseCandidate) bool { return sizeGap(c.Size, size) <= 0.01 })
	}
	if len(same) > 1 {
		narrow(func(c store.ReleaseCandidate) bool { return !c.Removed })
	}
	if len(same) != 1 {
		return store.ReleaseCandidate{}, false
	}
	return same[0], true
}

// withRetry waits and tries again while the tracker asks to slow down.
func withRetry[T any](ctx context.Context, f func() (T, error)) (T, error) {
	for {
		v, err := f()
		var rl *RateLimitError
		if !errors.As(err, &rl) {
			return v, err
		}
		slog.Warn("storico release: limite del tracker", "attesa", rl.RetryAfter)
		if !sleep(ctx, rl.RetryAfter) {
			return v, ctx.Err()
		}
	}
}
