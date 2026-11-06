package unit3d

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/tehmaat/bookrr/internal/store"
)

func TestCompletePlan(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(filepath.Join(t.TempDir(), "bookrr.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var ids []int64
	for _, tmdb := range []int64{603, 604, 605, 606} {
		e, err := s.AddPlanEntry(ctx, store.PlanEntry{TmdbID: tmdb, Title: "Film"})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, e.ID)
	}
	added, _ := s.PlanEntry(ctx, ids[0])
	at, _ := time.Parse(time.RFC3339, added.CreatedAt)
	before := at.Add(-time.Hour).Format(time.RFC3339)
	after := at.Add(time.Hour).Format(time.RFC3339)
	later := at.Add(2 * time.Hour).Format(time.RFC3339)
	ups := []store.TrackerUpload{
		// 603: uploaded after being added, twice: the first upload.
		{ID: 1, Name: "The.Matrix.1999.2160p.REMUX-ME", Category: "Movie", TmdbID: 603, CreatedAt: later},
		{ID: 2, Name: "The.Matrix.1999.1080p.REMUX-ME", Category: "Movie", TmdbID: 603, CreatedAt: after},
		// 604: only an older upload, the one to redo.
		{ID: 3, Name: "Film.2003.1080p-ME", Category: "Movie", TmdbID: 604, CreatedAt: before},
		// 605: a series with the same TMDB id.
		{ID: 4, Name: "Show.S01.1080p-ME", Category: "TV", TmdbID: 605, CreatedAt: after},
	}
	n, err := CompletePlan(ctx, s, ups)
	if err != nil || n != 1 {
		t.Fatalf("completed %d %v", n, err)
	}
	for i, want := range []struct {
		status  string
		release int64
	}{{store.PlanDone, 2}, {store.PlanTodo, 0}, {store.PlanTodo, 0}, {store.PlanTodo, 0}} {
		e, _ := s.PlanEntry(ctx, ids[i])
		if e.Status != want.status || e.ReleaseID != want.release {
			t.Errorf("%d: %+v", e.TmdbID, e)
		}
	}
	// Already done: not completed again.
	if n, err := CompletePlan(ctx, s, ups); err != nil || n != 0 {
		t.Fatalf("again %d %v", n, err)
	}
}
