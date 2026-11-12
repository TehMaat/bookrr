package syncer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/tehmaat/bookrr/internal/store"
	"github.com/tehmaat/bookrr/internal/wayfinderr"
)

type fakePlan struct {
	ID      string                 `json:"id"`
	Status  string                 `json:"status"`
	PairID  *string                `json:"pairId"`
	Message *string                `json:"message"`
	Files   []wayfinderr.MergeSide `json:"-"`
	TmdbID  int64                  `json:"-"`
	Origin  string                 `json:"-"`
}

// fakeMerges is the planned pairs of a Wayfinderr.
type fakeMerges struct {
	mu      sync.Mutex
	plans   map[string]*fakePlan
	next    int
	deleted []string
	down    bool
}

func (f *fakeMerges) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("X-Api-Key") != "key" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	if f.down {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/merge/plans/")
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/merge/plans":
		var body struct {
			Files  []wayfinderr.MergeSide `json:"files"`
			TmdbID int64                  `json:"tmdbId"`
			Origin string                 `json:"origin"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		for _, p := range f.plans {
			if p.Status != "PAIRED" && p.Files[0].InfoHash == body.Files[0].InfoHash && p.Files[1].InfoHash == body.Files[1].InfoHash {
				json.NewEncoder(w).Encode(p)
				return
			}
		}
		f.next++
		p := &fakePlan{ID: fmt.Sprintf("p%d", f.next), Status: "WAITING", Files: body.Files, TmdbID: body.TmdbID, Origin: body.Origin}
		f.plans[p.ID] = p
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(p)
	case strings.HasSuffix(id, "/pair") && r.Method == http.MethodPost:
		p := f.plans[strings.TrimSuffix(id, "/pair")]
		pair := "pair-" + p.ID
		p.Status, p.PairID, p.Message = "PAIRED", &pair, nil
		json.NewEncoder(w).Encode(p)
	case r.Method == http.MethodGet && f.plans[id] != nil:
		json.NewEncoder(w).Encode(f.plans[id])
	case r.Method == http.MethodDelete && f.plans[id] != nil:
		delete(f.plans, id)
		f.deleted = append(f.deleted, id)
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"Planned pair not found"}`))
	}
}

func TestMergePlans(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	fake := &fakeMerges{plans: map[string]*fakePlan{}}
	srv := httptest.NewServer(fake)
	defer srv.Close()

	plain, err := st.CreateClient(ctx, store.Client{Name: "plain", URL: "http://plain", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	ripper, err := st.CreateClient(ctx, store.Client{Name: "ripper", URL: "http://ripper", Enabled: true,
		WayfinderrURL: srv.URL, WayfinderrAPIKey: "key"})
	if err != nil {
		t.Fatal(err)
	}
	// Another client of the same Wayfinderr.
	ripper2, err := st.CreateClient(ctx, store.Client{Name: "ripper2", URL: "http://ripper2", Enabled: true,
		WayfinderrURL: srv.URL + "/", WayfinderrAPIKey: "key"})
	if err != nil {
		t.Fatal(err)
	}
	e, err := st.AddPlanEntry(ctx, store.PlanEntry{TmdbID: 603, Title: "Matrix"})
	if err != nil {
		t.Fatal(err)
	}
	// A film with one torrent only is never planned.
	single, err := st.AddPlanEntry(ctx, store.PlanEntry{TmdbID: 604, Title: "Reloaded"})
	if err != nil {
		t.Fatal(err)
	}
	load := func(planID int64, position int, hash, label string) {
		t.Helper()
		if _, err := st.SetPlanTorrent(ctx, planID, position, store.PlanTorrentInput{Hash: hash, Name: "Name." + hash, Label: label, Data: []byte(hash)}); err != nil {
			t.Fatal(err)
		}
	}
	sent := func(planID int64, position int, c store.Client) {
		t.Helper()
		if err := st.MarkPlanTorrentSent(ctx, planID, position, c.ID); err != nil {
			t.Fatal(err)
		}
	}
	load(e.ID, 0, "aaa", "video")
	load(e.ID, 0, "bbb", "audio ITA")
	load(single.ID, 0, "zzz", "")
	sent(single.ID, 1, ripper)

	run := func(want int) {
		t.Helper()
		n, err := MergePlans(ctx, st)
		if err != nil || n != want {
			t.Fatalf("planned %d (want %d): %v", n, want, err)
		}
	}
	merge := func() store.PlanMerge {
		t.Helper()
		m, err := st.PlanMergeOf(ctx, e.ID)
		if err != nil {
			t.Fatal(err)
		}
		return m
	}

	// Not sent yet: nothing to plan.
	run(0)
	if _, err := st.PlanMergeOf(ctx, e.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("not sent: %v", err)
	}

	// One on a client with a Wayfinderr, the other on one without: said, not planned.
	sent(e.ID, 1, ripper)
	sent(e.ID, 2, plain)
	run(0)
	if m := merge(); m.RemoteID != "" || !strings.Contains(m.Error, "same Wayfinderr") {
		t.Fatalf("different Wayfinderr: %+v", m)
	}

	// On two clients of the same Wayfinderr: planned, with hashes, names, labels and film.
	sent(e.ID, 2, ripper2)
	run(1)
	m := merge()
	if m.RemoteID != "p1" || m.Status != "WAITING" || m.Error != "" || m.ClientID != ripper.ID || m.PlannedAt == nil {
		t.Fatalf("planned: %+v", m)
	}
	p := fake.plans["p1"]
	if p.TmdbID != 603 || p.Origin != "bookrr" || p.Files[0] != (wayfinderr.MergeSide{InfoHash: "aaa", Name: "Name.aaa", Label: "video"}) ||
		p.Files[1] != (wayfinderr.MergeSide{InfoHash: "bbb", Name: "Name.bbb", Label: "audio ITA"}) {
		t.Fatalf("on Wayfinderr: %+v", p)
	}
	run(0) // read again, not planned again
	if len(fake.plans) != 1 {
		t.Fatalf("plans: %v", fake.plans)
	}

	// A torrent replaced: the old plan goes from Wayfinderr at once, the new one once it is sent.
	load(e.ID, 2, "ccc", "audio ITA")
	if got, err := SyncMergeOf(ctx, st, e.ID); err != nil || got != nil {
		t.Fatalf("replaced: %+v %v", got, err)
	}
	if len(fake.deleted) != 1 || fake.deleted[0] != "p1" {
		t.Fatalf("deleted: %v", fake.deleted)
	}
	sent(e.ID, 2, ripper)
	run(1)
	if m := merge(); m.RemoteID != "p2" || m.Hashes != "aaa,ccc" {
		t.Fatalf("planned again: %+v", m)
	}

	// Wayfinderr pairs the two files once both arrived.
	pair := "pair9"
	fake.plans["p2"].Status, fake.plans["p2"].PairID = "PAIRED", &pair
	run(0)
	if m := merge(); m.Status != "PAIRED" || m.PairID != "pair9" || m.CheckedAt == nil {
		t.Fatalf("paired: %+v", m)
	}
	delete(fake.plans, "p2")
	run(0) // a paired plan is not read again
	if m := merge(); m.Status != "PAIRED" {
		t.Fatalf("paired, not read: %+v", m)
	}

	// Planned by hand again; then in error, created again.
	if m, err := PlanMergeNow(ctx, st, e.ID, 0); err != nil || m.RemoteID != "p3" || m.Status != "WAITING" {
		t.Fatalf("by hand: %+v %v", m, err)
	}
	msg := "file gone"
	fake.plans["p3"].Status, fake.plans["p3"].Message = "ERROR", &msg
	run(0)
	if m := merge(); m.Status != "ERROR" || m.Message != "file gone" {
		t.Fatalf("error: %+v", m)
	}
	if m, err := PairMergeAgain(ctx, st, e.ID); err != nil || m.Status != "PAIRED" || m.PairID != "pair-p3" || m.Message != "" {
		t.Fatalf("pair again: %+v %v", m, err)
	}
	var bad *MergeRequestError
	if _, err := PairMergeAgain(ctx, st, e.ID); !errors.As(err, &bad) {
		t.Fatalf("pair again, not in error: %v", err)
	}

	// Removed by the user: removed from Wayfinderr while open, and not planned again by itself.
	if _, err := PlanMergeNow(ctx, st, e.ID, ripper2.ID); err != nil {
		t.Fatal(err)
	}
	if m, err := RemoveMerge(ctx, st, e.ID); err != nil || m.Status != store.MergeRemoved {
		t.Fatalf("removed: %+v %v", m, err)
	}
	if fake.deleted[len(fake.deleted)-1] != "p4" {
		t.Fatalf("deleted: %v", fake.deleted)
	}
	run(0)
	if m := merge(); m.Status != store.MergeRemoved {
		t.Fatalf("still removed: %+v", m)
	}

	// Gone from Wayfinderr.
	if _, err := PlanMergeNow(ctx, st, e.ID, 0); err != nil {
		t.Fatal(err)
	}
	fake.plans = map[string]*fakePlan{}
	run(0)
	if m := merge(); m.Status != store.MergeGone {
		t.Fatalf("gone: %+v", m)
	}

	// Wayfinderr down: the error is kept and the next sync tries again.
	load(e.ID, 1, "ddd", "video")
	sent(e.ID, 1, ripper)
	fake.down = true
	run(0)
	if m := merge(); m.RemoteID != "" || m.Hashes != "ccc,ddd" || !strings.Contains(m.Error, "503") {
		t.Fatalf("down: %+v", m)
	}
	fake.down = false
	run(1)
	if m := merge(); m.RemoteID == "" || m.Error != "" {
		t.Fatalf("after down: %+v", m)
	}

	// By hand: a film with one torrent, a client without a Wayfinderr.
	if _, err := PlanMergeNow(ctx, st, single.ID, 0); !errors.As(err, &bad) {
		t.Fatalf("single: %v", err)
	}
	if _, err := PlanMergeNow(ctx, st, e.ID, plain.ID); !errors.As(err, &bad) {
		t.Fatalf("plain: %v", err)
	}
	if _, err := st.PlanMergeOf(ctx, single.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("single planned: %v", err)
	}

	// The film removed from the plan: its plan goes from Wayfinderr.
	open := len(fake.plans)
	ForgetMergeOf(ctx, st, e.ID)
	if len(fake.plans) != open-1 {
		t.Fatalf("forgotten: %v", fake.plans)
	}
}
