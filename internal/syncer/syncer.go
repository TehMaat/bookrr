// Package syncer periodically pulls torrents from every qBittorrent client.
package syncer

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/tehmaat/bookrr/internal/qbit"
	"github.com/tehmaat/bookrr/internal/store"
)

type Syncer struct {
	store      *store.Store
	releaseTag string
	interval   time.Duration

	mu       sync.Mutex // serializes runs
	clientMu sync.Mutex
	clients  map[int64]cachedClient

	stateMu sync.RWMutex
	state   State
}

type cachedClient struct {
	key string
	c   *qbit.Client
}

type State struct {
	Running    bool    `json:"running"`
	LastRunAt  *string `json:"lastRunAt"`
	LastResult string  `json:"lastResult"`
	NextRunAt  *string `json:"nextRunAt"`
}

func New(s *store.Store, releaseTag string, interval time.Duration) *Syncer {
	return &Syncer{store: s, releaseTag: releaseTag, interval: interval, clients: map[int64]cachedClient{}}
}

func (s *Syncer) State() State {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.state
}

// Run syncs immediately and then on every tick until ctx is done.
func (s *Syncer) Run(ctx context.Context) {
	t := time.NewTicker(s.interval)
	defer t.Stop()
	for {
		s.setNext(time.Now().Add(s.interval))
		if _, err := s.SyncAll(ctx); err != nil && ctx.Err() == nil {
			slog.Error("sync fallito", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (s *Syncer) setNext(t time.Time) {
	s.stateMu.Lock()
	v := t.UTC().Format(time.RFC3339)
	s.state.NextRunAt = &v
	s.stateMu.Unlock()
}

// Qbit returns a (cached) API client for a configured client.
func (s *Syncer) Qbit(c store.Client) *qbit.Client {
	key := fmt.Sprintf("%s\x00%s\x00%s\x00%t", c.URL, c.Username, c.Password, c.SkipTLSVerify)
	s.clientMu.Lock()
	defer s.clientMu.Unlock()
	if cc, ok := s.clients[c.ID]; ok && cc.key == key {
		return cc.c
	}
	q := qbit.New(c.URL, c.Username, c.Password, c.SkipTLSVerify)
	s.clients[c.ID] = cachedClient{key: key, c: q}
	return q
}

// SyncAll pulls every enabled client. Clients that fail keep their previous
// locations so a temporary outage never looks like a mass removal.
func (s *Syncer) SyncAll(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.stateMu.Lock()
	s.state.Running = true
	s.stateMu.Unlock()
	result, err := s.syncAll(ctx)
	s.stateMu.Lock()
	ts := time.Now().UTC().Format(time.RFC3339)
	s.state.Running = false
	s.state.LastRunAt = &ts
	if err != nil {
		s.state.LastResult = err.Error()
	} else {
		s.state.LastResult = result
	}
	s.stateMu.Unlock()
	return result, err
}

func (s *Syncer) syncAll(ctx context.Context) (string, error) {
	clients, err := s.store.ListClients(ctx)
	if err != nil {
		return "", err
	}
	var removed []store.Removal
	ok, failed, total := 0, 0, 0
	for _, c := range clients {
		if !c.Enabled {
			continue
		}
		cctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		ts, err := s.Qbit(c).Torrents(cctx)
		cancel()
		if err != nil {
			failed++
			slog.Warn("client non raggiungibile", "client", c.Name, "err", err)
			if err := s.store.SetClientSyncResult(ctx, c.ID, 0, err); err != nil {
				return "", err
			}
			continue
		}
		snap := make([]store.SnapshotTorrent, 0, len(ts))
		for _, t := range ts {
			hash := strings.ToLower(t.Hash)
			if hash == "" {
				continue
			}
			size := t.Size
			if t.TotalSize > size {
				size = t.TotalSize
			}
			snap = append(snap, store.SnapshotTorrent{
				Hash: hash, Name: t.Name, Size: size, Tags: store.NormalizeTags(t.Tags), Category: t.Category,
				Tracker: t.Tracker, SavePath: t.SavePath, State: t.State, Progress: t.Progress, Ratio: t.Ratio, AddedOn: t.AddedOn,
			})
		}
		r, err := s.store.ApplySnapshot(ctx, c, snap)
		if err != nil {
			return "", fmt.Errorf("salvataggio %s: %w", c.Name, err)
		}
		removed = append(removed, r...)
		ok++
		total += len(snap)
	}
	alerts, err := s.store.FinalizeSync(ctx, removed, s.releaseTag)
	if err != nil {
		return "", err
	}
	result := fmt.Sprintf("%d client sincronizzati, %d con errori, %d torrent, %d nuove segnalazioni", ok, failed, total, alerts)
	slog.Info("sync completato", "ok", ok, "failed", failed, "torrents", total, "removed", len(removed), "alerts", alerts)
	return result, nil
}
