package store

import (
	"context"
	"testing"
	"time"
)

var (
	seed    = seedLoc{state: "uploading", progress: 1}
	stalled = seedLoc{state: "stalledUP", progress: 1}
	paused  = seedLoc{state: "pausedUP", progress: 1}
	stopped = seedLoc{state: "stoppedUP", progress: 1}
	queued  = seedLoc{state: "queuedUP", progress: 1}
	partial = seedLoc{state: "downloading", progress: 0.4}
)

func TestAssessArchivable(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	rule := ArchiveRule{MinOthers: 1, MinCompleted: 1}
	cases := []struct {
		name            string
		seeders, done   int64
		locs            []seedLoc
		rule            ArchiveRule
		mine, others    int64
		met, archivable bool
	}{
		{"only me", 1, 3, []seedLoc{seed}, rule, 1, 0, false, false},
		{"me and someone else", 2, 1, []seedLoc{seed}, rule, 1, 1, true, true},
		{"stalled still seeds", 2, 1, []seedLoc{stalled}, rule, 1, 1, true, true},
		// Two clients of mine seeding count twice among the seeders.
		{"two clients of mine", 2, 5, []seedLoc{seed, stalled}, rule, 2, 0, false, false},
		{"two clients of mine and another", 3, 5, []seedLoc{seed, seed}, rule, 2, 1, true, true},
		// A paused, stopped or queued client is not announced: not mine.
		{"one client paused", 2, 1, []seedLoc{seed, paused}, rule, 1, 1, true, true},
		{"stopped and queued", 2, 1, []seedLoc{stopped, queued}, rule, 0, 2, true, true},
		{"paused only, alone", 0, 1, []seedLoc{paused}, rule, 0, 0, false, false},
		// An incomplete copy is not seeding even if the tracker counts it as a peer.
		{"incomplete copy", 2, 1, []seedLoc{seed, partial}, rule, 1, 1, true, true},
		{"incomplete with seed state", 2, 1, []seedLoc{{state: "uploading", progress: 0.99}}, rule, 0, 2, true, true},
		// The tracker may lag behind the clients: never below zero.
		{"tracker behind", 1, 1, []seedLoc{seed, seed}, rule, 2, 0, false, false},
		{"not completed enough", 2, 0, []seedLoc{seed}, rule, 1, 1, false, false},
		{"higher thresholds met", 4, 3, []seedLoc{seed}, ArchiveRule{MinOthers: 3, MinCompleted: 3}, 1, 3, true, true},
		{"higher seeders not met", 3, 9, []seedLoc{seed}, ArchiveRule{MinOthers: 3, MinCompleted: 3}, 1, 2, false, false},
		{"higher completed not met", 9, 2, []seedLoc{seed}, ArchiveRule{MinOthers: 3, MinCompleted: 3}, 1, 8, false, false},
		{"zero thresholds", 0, 0, []seedLoc{partial}, ArchiveRule{}, 0, 0, true, true},
	}
	for _, c := range cases {
		r := releaseCounters{id: 7, hash: "h", seeders: c.seeders, completed: c.done, seenAt: "2026-10-11T11:00:00Z"}
		a := assessArchivable(r, c.locs, c.rule, now)
		if a == nil {
			t.Errorf("%s: not assessed", c.name)
			continue
		}
		if a.Mine != c.mine || a.Others != c.others || a.Met != c.met || a.Archivable != c.archivable {
			t.Errorf("%s: mine %d others %d met %t archivable %t; want %d %d %t %t",
				c.name, a.Mine, a.Others, a.Met, a.Archivable, c.mine, c.others, c.met, c.archivable)
		}
		if a.ReleaseID != 7 || a.Seeders != c.seeders || a.TimesCompleted != c.done || a.TrackerAt != r.seenAt {
			t.Errorf("%s: numbers not reported: %+v", c.name, a)
		}
		if c.met != (a.Since != nil) {
			t.Errorf("%s: since %v with met %t", c.name, a.Since, c.met)
		}
	}

	// Only uploads still on a client and on the tracker are assessed.
	if a := assessArchivable(releaseCounters{seeders: 5, completed: 5}, nil, rule, now); a != nil {
		t.Errorf("no client: %+v", a)
	}
	if a := assessArchivable(releaseCounters{seeders: 5, completed: 5, removed: true}, []seedLoc{seed}, rule, now); a != nil {
		t.Errorf("removed from the tracker: %+v", a)
	}
}

func TestAssessArchivableDays(t *testing.T) {
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	at := func(d time.Duration) *string {
		s := now.Add(-d).Format(time.RFC3339)
		return &s
	}
	met := func(since *string) releaseCounters { return releaseCounters{seeders: 2, completed: 1, since: since} }
	cases := []struct {
		name       string
		since      *string
		days       int64
		gotDays    int64
		archivable bool
		readyAt    string
	}{
		{"met right now, no wait", nil, 0, 0, true, ""},
		{"met right now, 3 days to wait", nil, 3, 0, false, "2026-10-14T12:00:00Z"},
		{"2 days and 23 hours of 3", at(3*day - time.Hour), 3, 2, false, "2026-10-11T13:00:00Z"},
		{"exactly 3 days of 3", at(3 * day), 3, 3, true, ""},
		{"10 days of 3", at(10*day + time.Hour), 3, 10, true, ""},
		{"since in the future is now", at(-day), 1, 0, false, "2026-10-12T12:00:00Z"},
		{"unreadable since is now", func() *string { s := "ieri"; return &s }(), 0, 0, true, ""},
	}
	for _, c := range cases {
		a := assessArchivable(met(c.since), []seedLoc{seed}, ArchiveRule{MinOthers: 1, MinCompleted: 1, Days: c.days}, now)
		if a == nil || !a.Met || a.Since == nil {
			t.Errorf("%s: not met: %+v", c.name, a)
			continue
		}
		if a.Days != c.gotDays || a.Archivable != c.archivable {
			t.Errorf("%s: days %d archivable %t; want %d %t", c.name, a.Days, a.Archivable, c.gotDays, c.archivable)
		}
		if got := deref(a.ReadyAt); got != c.readyAt {
			t.Errorf("%s: ready at %q, want %q", c.name, got, c.readyAt)
		}
		if c.since != nil && *c.since != "ieri" && c.gotDays > 0 && *a.Since != *c.since {
			t.Errorf("%s: since %s, want %s", c.name, *a.Since, *c.since)
		}
	}
	// Not met: no since, even if one is stored.
	if a := assessArchivable(releaseCounters{seeders: 1, completed: 1, since: at(5 * day)}, []seedLoc{seed},
		ArchiveRule{MinOthers: 1}, now); a.Met || a.Since != nil || a.Archivable || a.Days != 0 {
		t.Errorf("not met: %+v", a)
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// The day the thresholds are first met is remembered while they stay met,
// forgotten as soon as they are not, and counted again from scratch.
func TestRefreshArchivable(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	clock := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	s.clock = func() time.Time { return clock }
	a, b := mustClient(t, s, "a"), mustClient(t, s, "b")
	pr := SnapshotTorrent{Hash: "h1", Name: "Mia", Size: 10, Tags: tag, State: "uploading", Progress: 1}
	pausedPR := pr
	pausedPR.State = "pausedUP"
	other := SnapshotTorrent{Hash: "h2", Name: "Altra", Size: 20, Tags: tag, State: "uploading", Progress: 1}

	if _, err := s.SaveSettings(ctx, Settings{ArchiveMinSeeders: 1, ArchiveMinCompleted: 1, ArchiveDays: 2}); err != nil {
		t.Fatal(err)
	}
	tracker := func(seeders, completed int64) {
		t.Helper()
		ups := []TrackerUpload{{ID: 1, Name: "Mia", Seeders: seeders, TimesCompleted: completed}, {ID: 2, Name: "Altra", Seeders: 9, TimesCompleted: 9}}
		if _, err := s.SaveTrackerReleases(ctx, ups); err != nil {
			t.Fatal(err)
		}
		if err := s.RefreshArchivable(ctx); err != nil {
			t.Fatal(err)
		}
	}
	clients := func(snaps map[Client][]SnapshotTorrent) {
		t.Helper()
		sync(t, s, snaps)
		if err := s.RefreshArchivable(ctx); err != nil {
			t.Fatal(err)
		}
	}
	check := func(step string, archivable bool, days int64, since string) {
		t.Helper()
		tr, err := s.GetTorrent(ctx, "h1")
		if err != nil {
			t.Fatal(err)
		}
		rel, err := s.GetTrackerRelease(ctx, 1)
		if err != nil {
			t.Fatal(err)
		}
		var stored *string
		if err := s.db.QueryRowContext(ctx, `SELECT archivable_since FROM tracker_releases WHERE id = 1`).Scan(&stored); err != nil {
			t.Fatal(err)
		}
		if deref(stored) != since {
			t.Errorf("%s: stored since %q, want %q", step, deref(stored), since)
		}
		for name, a := range map[string]*Archivability{"torrent": tr.Archivable, "release": rel.Archivable} {
			if a == nil {
				t.Errorf("%s: %s not assessed", step, name)
				continue
			}
			if a.Archivable != archivable || a.Days != days || deref(a.Since) != since {
				t.Errorf("%s: %s archivable %t days %d since %q; want %t %d %q", step, name, a.Archivable, a.Days, deref(a.Since), archivable, days, since)
			}
		}
	}

	clients(map[Client][]SnapshotTorrent{a: {pr, other}, b: {pr}})
	tracker(2, 1)
	for id, h := range map[int64]string{1: "h1", 2: "h2"} {
		if err := s.SetTrackerReleaseHash(ctx, id, h, nil); err != nil {
			t.Fatal(err)
		}
	}
	// Two seeders on the tracker: both are my clients.
	tracker(2, 1)
	check("both seeders are mine", false, 0, "")

	// One of my clients pauses: the other seeder is someone else.
	day0 := clock.Format(time.RFC3339)
	clients(map[Client][]SnapshotTorrent{a: {pr, other}, b: {pausedPR}})
	check("met, 2 days to wait", false, 0, day0)
	if tr, _ := s.GetTorrent(ctx, "h1"); tr.Archivable.Mine != 1 || tr.Archivable.Others != 1 || tr.Archivable.ReadyAt == nil {
		t.Fatalf("numbers: %+v", tr.Archivable)
	}

	clock = clock.Add(day + time.Hour)
	tracker(2, 1)
	check("one day later", false, 1, day0)
	clock = clock.Add(day)
	tracker(3, 2)
	check("two days later", true, 2, day0)

	// Kept on the clients: never archivable, the days are still counted.
	if n, err := s.SetNeverArchive(ctx, []string{"h1", "nope"}, true); err != nil || n != 1 {
		t.Fatalf("never archive: %d %v", n, err)
	}
	check("never archive", false, 2, day0)
	if tr, _ := s.GetTorrent(ctx, "h1"); !tr.Archivable.NeverArchive || tr.Archivable.ReadyAt != nil {
		t.Fatalf("never archive: %+v", tr.Archivable)
	}
	if r, _ := s.GetTrackerRelease(ctx, 1); !r.Archivable.NeverArchive {
		t.Fatalf("release never archive: %+v", r.Archivable)
	}
	if tr, _ := s.GetTorrent(ctx, "h2"); tr.Archivable.NeverArchive || !tr.Archivable.Archivable {
		t.Fatalf("other upload kept too: %+v", tr.Archivable)
	}
	if _, err := s.SetNeverArchive(ctx, []string{"h1"}, false); err != nil {
		t.Fatal(err)
	}
	check("archivable again", true, 2, day0)

	// The other seeder leaves: the count starts over.
	clock = clock.Add(time.Hour)
	tracker(1, 2)
	check("not met anymore", false, 0, "")
	if tr, _ := s.GetTorrent(ctx, "h1"); tr.Archivable.Met || tr.Archivable.Since != nil {
		t.Fatalf("reset: %+v", tr.Archivable)
	}
	clock = clock.Add(time.Hour)
	day1 := clock.Format(time.RFC3339)
	tracker(2, 2)
	check("met again", false, 0, day1)

	// Stricter thresholds reset it, looser ones make it archivable at once.
	if _, err := s.SaveSettings(ctx, Settings{ArchiveMinSeeders: 1, ArchiveMinCompleted: 3, ArchiveDays: 2}); err != nil {
		t.Fatal(err)
	}
	check("stricter rule", false, 0, "")
	if _, err := s.SaveSettings(ctx, Settings{ArchiveMinSeeders: 1, ArchiveMinCompleted: 1, ArchiveDays: 0}); err != nil {
		t.Fatal(err)
	}
	check("no days to wait", true, 0, day1)

	// Removed from every client (archived): no longer assessed.
	clients(map[Client][]SnapshotTorrent{a: {other}, b: {}})
	tr, _ := s.GetTorrent(ctx, "h1")
	if tr.Archivable != nil {
		t.Fatalf("off the clients: %+v", tr.Archivable)
	}
	// The counters of the tracker are still shown.
	if p := tr.Peers; p == nil || p.ReleaseID != 1 || p.Seeders != 2 || p.TimesCompleted != 2 {
		t.Fatalf("tracker off the clients: %+v", p)
	}
	if r, _ := s.GetTrackerRelease(ctx, 1); r.Archivable != nil {
		t.Fatalf("release off the clients: %+v", r.Archivable)
	}
	var stored *string
	if err := s.db.QueryRowContext(ctx, `SELECT archivable_since FROM tracker_releases WHERE id = 1`).Scan(&stored); err != nil || stored != nil {
		t.Fatalf("since kept off the clients: %v %v", deref(stored), err)
	}
	// The other upload is unaffected.
	if tr, _ := s.GetTorrent(ctx, "h2"); tr.Archivable == nil || !tr.Archivable.Archivable || tr.Archivable.Others != 8 {
		t.Fatalf("other upload: %+v", tr.Archivable)
	}
	// A torrent that is not an upload is never assessed.
	if ts, _ := s.ListTorrents(ctx); len(ts) != 2 {
		t.Fatalf("torrents: %d", len(ts))
	}
}

func TestArchiveSettings(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	st, err := s.GetSettings(ctx)
	if err != nil || st != DefaultSettings {
		t.Fatalf("defaults: %+v %v", st, err)
	}
	in := Settings{PublicURL: "http://nas:8080", ArchiveMinSeeders: 2, ArchiveMinCompleted: 5, ArchiveDays: 7}
	if st, err := s.SaveSettings(ctx, in); err != nil || st != in {
		t.Fatalf("saved: %+v %v", st, err)
	}
	for _, bad := range []Settings{{ArchiveMinSeeders: -1}, {ArchiveMinCompleted: -1}, {ArchiveDays: -1}, {ArchiveDays: MaxArchiveDays + 1}} {
		if _, err := s.SaveSettings(ctx, bad); err != ErrInvalidSettings {
			t.Errorf("%+v: %v", bad, err)
		}
	}
	if st, _ := s.GetSettings(ctx); st != in {
		t.Fatalf("changed by invalid saves: %+v", st)
	}
}
