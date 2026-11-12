package syncer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/tehmaat/bookrr/internal/store"
	"github.com/tehmaat/bookrr/internal/wayfinderr"
)

// The pair of a merge film (two .torrent files) on Wayfinderr: once both
// torrents are sent to clients with the same Wayfinderr, bookrr plans the
// pair there (POST /api/merge/plans, by info hash and name), so that
// Wayfinderr holds each file as it arrives and pairs the two once both are
// there. The merge itself still waits for a confirmation on Wayfinderr.

// MergeRequestError is a request about the pair of a film that cannot be
// done as asked (no two torrents, no Wayfinderr to plan it on...).
type MergeRequestError struct{ msg string }

func (e *MergeRequestError) Error() string { return e.msg }

func badMerge(format string, a ...any) error {
	return &MergeRequestError{msg: fmt.Sprintf(format, a...)}
}

func stamp() *string {
	s := time.Now().UTC().Format(time.RFC3339)
	return &s
}

// wayfinderrCandidates are the enabled clients with a Wayfinderr a torrent
// is on, or was sent to.
func wayfinderrCandidates(t store.PlanTorrent, clients map[int64]store.Client) []store.Client {
	var out []store.Client
	seen := map[int64]bool{}
	ids := []int64{t.SentClientID, t.WayfinderrClientID}
	for _, d := range t.Downloads {
		ids = append(ids, d.ClientID)
	}
	for _, id := range ids {
		c, ok := clients[id]
		if id == 0 || seen[id] || !ok || !c.Enabled || !c.HasWayfinderr() {
			continue
		}
		seen[id] = true
		out = append(out, c)
	}
	return out
}

// placed reports whether a torrent was sent to a client, or is on one.
func placed(t store.PlanTorrent) bool { return t.SentClientID != 0 || len(t.Downloads) > 0 }

// mergeClient is the client whose Wayfinderr plans the pair of ts: one with
// a Wayfinderr both torrents are on (or were sent to), or two clients with
// the same Wayfinderr. ok is false, with no error, when there is nothing to
// plan yet (a torrent not sent, or none on a client with a Wayfinderr);
// an error says why the two cannot be planned together.
func mergeClient(ts []store.PlanTorrent, clients map[int64]store.Client) (c store.Client, ok bool, err error) {
	if !placed(ts[0]) || !placed(ts[1]) {
		return store.Client{}, false, nil
	}
	a, b := wayfinderrCandidates(ts[0], clients), wayfinderrCandidates(ts[1], clients)
	if len(a) == 0 && len(b) == 0 {
		return store.Client{}, false, nil
	}
	for _, x := range a {
		for _, y := range b {
			if x.ID == y.ID {
				return x, true, nil
			}
		}
	}
	for _, x := range a {
		for _, y := range b {
			if wayfinderr.SameWayfinderr(x.WayfinderrURL, y.WayfinderrURL) {
				return x, true, nil
			}
		}
	}
	return store.Client{}, false, errors.New("the two .torrent files are not on clients with the same Wayfinderr: " +
		"send them to the same one, or plan the pair by hand")
}

func clientsByID(ctx context.Context, st *store.Store) (map[int64]store.Client, error) {
	cs, err := st.ListClients(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]store.Client, len(cs))
	for _, c := range cs {
		out[c.ID] = c
	}
	return out, nil
}

// forgetMerge removes from Wayfinderr a plan still open there (waiting or in
// error), so that its downloads can be planned again. A plan whose client is
// gone, or no longer has a Wayfinderr, cannot be reached: it is let go.
func forgetMerge(ctx context.Context, m store.PlanMerge, clients map[int64]store.Client) error {
	if !m.Planned() {
		return nil
	}
	c, ok := clients[m.ClientID]
	if !ok || !c.HasWayfinderr() {
		return nil
	}
	wctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := wayfinderr.New(c.WayfinderrURL, c.WayfinderrAPIKey).DeleteMergePlan(wctx, m.RemoteID); err != nil {
		return err
	}
	slog.Info("merge plan removed from Wayfinderr", "plan", m.PlanID, "wayfinderrPlan", m.RemoteID, "client", c.Name)
	return nil
}

// planOn plans the pair of film e on the Wayfinderr of client c. The row is
// written either way: with the plan, or with why it failed.
func planOn(ctx context.Context, st *store.Store, e store.PlanEntry, ts []store.PlanTorrent, c store.Client,
	old store.PlanMerge) (store.PlanMerge, error) {
	hashes := store.MergeHashes(ts)
	m := store.PlanMerge{PlanID: e.ID, Hashes: hashes, ClientID: c.ID}
	if old.Hashes == hashes {
		m = old
		m.ClientID = c.ID
	}
	plan := wayfinderr.MergePlan{TmdbID: e.TmdbID}
	for i, t := range ts {
		plan.Files[i] = wayfinderr.MergeSide{InfoHash: t.Hash, Name: t.Name, Label: t.Label}
	}
	wctx, cancel := context.WithTimeout(ctx, time.Minute)
	p, created, err := wayfinderr.New(c.WayfinderrURL, c.WayfinderrAPIKey).PlanMerge(wctx, plan)
	cancel()
	if err != nil {
		m.Error = err.Error()
		if _, serr := st.SavePlanMerge(ctx, m); serr != nil {
			return m, serr
		}
		slog.Warn("planning the merge pair on Wayfinderr failed", "plan", e.ID, "client", c.Name, "err", err)
		return m, err
	}
	m.RemoteID, m.Error = p.ID, ""
	m.PlannedAt, m.CheckedAt = stamp(), stamp()
	setRemote(&m, p)
	slog.Info("merge pair planned on Wayfinderr", "plan", e.ID, "wayfinderrPlan", p.ID, "status", p.Status,
		"client", c.Name, "new", created)
	return st.SavePlanMerge(ctx, m)
}

func setRemote(m *store.PlanMerge, p wayfinderr.PlannedPair) {
	m.Status, m.PairID, m.Message = p.Status, "", ""
	if p.PairID != nil {
		m.PairID = *p.PairID
	}
	if p.Message != nil {
		m.Message = *p.Message
	}
}

// refreshMerge reads again from Wayfinderr how a plan still open goes.
func refreshMerge(ctx context.Context, st *store.Store, m store.PlanMerge, clients map[int64]store.Client) (store.PlanMerge, error) {
	c, ok := clients[m.ClientID]
	if !ok || !c.HasWayfinderr() {
		m.Error = "the client the pair was planned through no longer has a Wayfinderr"
		return st.SavePlanMerge(ctx, m)
	}
	wctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	p, err := wayfinderr.New(c.WayfinderrURL, c.WayfinderrAPIKey).MergePlanOf(wctx, m.RemoteID)
	cancel()
	switch {
	case errors.Is(err, wayfinderr.ErrPlanNotFound):
		m.Status, m.Error = store.MergeGone, ""
		slog.Info("merge plan no longer on Wayfinderr", "plan", m.PlanID, "wayfinderrPlan", m.RemoteID, "client", c.Name)
	case err != nil:
		m.Error = err.Error()
	default:
		if p.Status != m.Status {
			slog.Info("merge plan on Wayfinderr changed", "plan", m.PlanID, "wayfinderrPlan", m.RemoteID,
				"from", m.Status, "to", p.Status)
		}
		setRemote(&m, p)
		m.Error = ""
	}
	m.CheckedAt = stamp()
	return st.SavePlanMerge(ctx, m)
}

// syncMerge brings the pair of film e on Wayfinderr up to date: a plan for
// torrents the film no longer has is removed, an open one is read again, and
// a film with two torrents sent to clients with the same Wayfinderr is
// planned. planned is true when it was planned now.
func syncMerge(ctx context.Context, st *store.Store, e store.PlanEntry, ts []store.PlanTorrent, m store.PlanMerge, has bool,
	clients map[int64]store.Client) (planned bool, err error) {
	hashes := store.MergeHashes(ts)
	if has && m.Hashes != hashes {
		// The torrents of the film changed: the old plan goes.
		if ferr := forgetMerge(ctx, m, clients); ferr != nil {
			m.Error = "removing the old plan from Wayfinderr: " + ferr.Error()
			_, err := st.SavePlanMerge(ctx, m)
			return false, err
		}
		if err := st.DeletePlanMerge(ctx, e.ID); err != nil {
			return false, err
		}
		has, m = false, store.PlanMerge{}
	}
	if hashes == "" {
		return false, nil
	}
	if has && m.RemoteID != "" {
		if m.Planned() {
			_, err := refreshMerge(ctx, st, m, clients)
			return false, err
		}
		// Paired, gone or removed: not planned again by itself.
		return false, nil
	}
	if has && m.Status == store.MergeRemoved {
		return false, nil
	}
	if !e.Open() {
		return false, nil
	}
	c, ok, why := mergeClient(ts, clients)
	if why != nil {
		if !has || m.Error != why.Error() {
			m.PlanID, m.Hashes, m.Error = e.ID, hashes, why.Error()
			_, err := st.SavePlanMerge(ctx, m)
			return false, err
		}
		return false, nil
	}
	if !ok {
		if has {
			// Nothing to plan any more (a torrent no longer on a client): the old reason goes.
			return false, st.DeletePlanMerge(ctx, e.ID)
		}
		return false, nil
	}
	if _, err := planOn(ctx, st, e, ts, c, m); err != nil {
		// Kept on the film, tried again at the next sync.
		return false, nil
	}
	return true, nil
}

// MergePlans brings the pairs of the merge films up to date on Wayfinderr
// (see syncMerge). A Wayfinderr that does not answer never fails it: the
// error is kept on the film and tried again at the next sync.
func MergePlans(ctx context.Context, st *store.Store) (planned int, err error) {
	entries, err := st.PlanEntries(ctx)
	if err != nil {
		return 0, err
	}
	torrents, err := st.PlanTorrents(ctx)
	if err != nil {
		return 0, err
	}
	merges, err := st.PlanMerges(ctx)
	if err != nil {
		return 0, err
	}
	clients, err := clientsByID(ctx, st)
	if err != nil {
		return 0, err
	}
	for _, e := range entries {
		m, has := merges[e.ID]
		if !has && store.MergeHashes(torrents[e.ID]) == "" {
			continue
		}
		ok, err := syncMerge(ctx, st, e, torrents[e.ID], m, has, clients)
		if ctx.Err() != nil {
			return planned, ctx.Err()
		}
		if err != nil {
			return planned, err
		}
		if ok {
			planned++
		}
	}
	return planned, nil
}

// SyncMergeOf brings the pair of one film up to date (see syncMerge), as
// soon as its torrents are sent, replaced or removed. It returns the pair
// as it is now, nil if none.
func SyncMergeOf(ctx context.Context, st *store.Store, planID int64) (*store.PlanMerge, error) {
	e, err := st.PlanEntry(ctx, planID)
	if err != nil {
		return nil, err
	}
	ts, err := st.PlanTorrentsOf(ctx, planID)
	if err != nil {
		return nil, err
	}
	m, err := st.PlanMergeOf(ctx, planID)
	has := err == nil
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	clients, err := clientsByID(ctx, st)
	if err != nil {
		return nil, err
	}
	if _, err := syncMerge(ctx, st, e, ts, m, has, clients); err != nil {
		return nil, err
	}
	m, err = st.PlanMergeOf(ctx, planID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	return &m, err
}

// PlanMergeNow plans the pair of a film by hand, also again (one removed, or
// gone from Wayfinderr): on the Wayfinderr of client clientID, or with 0 of
// the client the torrents are on, else the only one with a Wayfinderr.
func PlanMergeNow(ctx context.Context, st *store.Store, planID, clientID int64) (store.PlanMerge, error) {
	e, err := st.PlanEntry(ctx, planID)
	if err != nil {
		return store.PlanMerge{}, err
	}
	ts, err := st.PlanTorrentsOf(ctx, planID)
	if err != nil {
		return store.PlanMerge{}, err
	}
	if store.MergeHashes(ts) == "" {
		return store.PlanMerge{}, badMerge("the film has no two .torrent files to merge")
	}
	clients, err := clientsByID(ctx, st)
	if err != nil {
		return store.PlanMerge{}, err
	}
	var c store.Client
	if clientID != 0 {
		var ok bool
		if c, ok = clients[clientID]; !ok {
			return store.PlanMerge{}, badMerge("the client no longer exists")
		}
		if !c.HasWayfinderr() {
			return store.PlanMerge{}, badMerge("%s has no Wayfinderr: set it in Clients", c.Name)
		}
	} else {
		var ok bool
		c, ok, _ = mergeClient(ts, clients)
		if !ok {
			var with []store.Client
			for _, c := range clients {
				if c.Enabled && c.HasWayfinderr() {
					with = append(with, c)
				}
			}
			switch len(with) {
			case 0:
				return store.PlanMerge{}, badMerge("no client has a Wayfinderr: set it in Clients")
			case 1:
				c = with[0]
			default:
				return store.PlanMerge{}, badMerge("choose the client whose Wayfinderr to plan the pair on")
			}
		}
	}
	old, err := st.PlanMergeOf(ctx, planID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return store.PlanMerge{}, err
	}
	// A plan on another Wayfinderr, or for other torrents, goes first.
	if err == nil && (old.Hashes != store.MergeHashes(ts) ||
		(old.Planned() && !wayfinderr.SameWayfinderr(clients[old.ClientID].WayfinderrURL, c.WayfinderrURL))) {
		if err := forgetMerge(ctx, old, clients); err != nil {
			return old, fmt.Errorf("removing the old plan from Wayfinderr: %w", err)
		}
		old = store.PlanMerge{}
	}
	return planOn(ctx, st, e, ts, c, old)
}

// RemoveMerge removes the pair of a film from Wayfinderr (the files it held
// stay held there) and keeps it from being planned again by itself, until
// it is planned by hand or the torrents of the film change.
func RemoveMerge(ctx context.Context, st *store.Store, planID int64) (store.PlanMerge, error) {
	if _, err := st.PlanEntry(ctx, planID); err != nil {
		return store.PlanMerge{}, err
	}
	ts, err := st.PlanTorrentsOf(ctx, planID)
	if err != nil {
		return store.PlanMerge{}, err
	}
	hashes := store.MergeHashes(ts)
	if hashes == "" {
		return store.PlanMerge{}, badMerge("the film has no two .torrent files to merge")
	}
	m, err := st.PlanMergeOf(ctx, planID)
	if errors.Is(err, store.ErrNotFound) || (err == nil && m.Hashes != hashes) {
		m, err = store.PlanMerge{PlanID: planID, Hashes: hashes}, nil
	}
	if err != nil {
		return store.PlanMerge{}, err
	}
	clients, err := clientsByID(ctx, st)
	if err != nil {
		return store.PlanMerge{}, err
	}
	if err := forgetMerge(ctx, m, clients); err != nil {
		return m, err
	}
	m.Status, m.Error, m.Message = store.MergeRemoved, "", ""
	return st.SavePlanMerge(ctx, m)
}

// PairMergeAgain asks Wayfinderr to create again the pair of a plan that
// ended in error, once both its files are there.
func PairMergeAgain(ctx context.Context, st *store.Store, planID int64) (store.PlanMerge, error) {
	m, err := st.PlanMergeOf(ctx, planID)
	if errors.Is(err, store.ErrNotFound) || (err == nil && m.Status != wayfinderr.MergeError) {
		return m, badMerge("the pair of the film is not in error on Wayfinderr")
	}
	if err != nil {
		return m, err
	}
	c, err := st.GetClient(ctx, m.ClientID)
	if errors.Is(err, store.ErrNotFound) || (err == nil && !c.HasWayfinderr()) {
		return m, badMerge("the client the pair was planned through no longer has a Wayfinderr")
	}
	if err != nil {
		return m, err
	}
	wctx, cancel := context.WithTimeout(ctx, time.Minute)
	p, perr := wayfinderr.New(c.WayfinderrURL, c.WayfinderrAPIKey).PairMerge(wctx, m.RemoteID)
	cancel()
	switch {
	case errors.Is(perr, wayfinderr.ErrPlanNotFound):
		m.Status, m.Error = store.MergeGone, ""
	case perr != nil:
		m.Error = perr.Error()
	default:
		setRemote(&m, p)
		m.Error = ""
		slog.Info("merge pair created again on Wayfinderr", "plan", planID, "wayfinderrPlan", m.RemoteID, "status", p.Status)
	}
	m.CheckedAt = stamp()
	saved, err := st.SavePlanMerge(ctx, m)
	if err != nil {
		return saved, err
	}
	return saved, perr
}

// ForgetMergeOf removes from Wayfinderr the plan of a film about to be
// removed from the plan; a Wayfinderr that does not answer is let go.
func ForgetMergeOf(ctx context.Context, st *store.Store, planID int64) {
	m, err := st.PlanMergeOf(ctx, planID)
	if err != nil {
		return
	}
	clients, err := clientsByID(ctx, st)
	if err != nil {
		return
	}
	if err := forgetMerge(ctx, m, clients); err != nil {
		slog.Warn("removing the merge plan from Wayfinderr failed", "plan", planID, "err", err)
	}
}
