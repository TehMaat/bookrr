package store

import (
	"context"
	"errors"
	"testing"
)

func TestPlan(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	e, err := s.AddPlanEntry(ctx, PlanEntry{TmdbID: 603, ImdbID: 133093, Title: "Matrix", OriginalTitle: "The Matrix",
		ReleaseDate: "1999-03-31", Source: "saga:collection:2344", SourceTitle: "Matrix - La saga", Note: " rifare "})
	if err != nil {
		t.Fatal(err)
	}
	if e.ID == 0 || e.Status != PlanTodo || e.Note != "rifare" || e.CreatedAt == "" || e.DoneAt != nil || e.Year() != "1999" {
		t.Fatalf("added %+v", e)
	}
	if _, err := s.AddPlanEntry(ctx, PlanEntry{TmdbID: 603}); !errors.Is(err, ErrExists) {
		t.Fatalf("duplicate: %v", err)
	}
	if _, err := s.AddPlanEntry(ctx, PlanEntry{}); err == nil {
		t.Fatal("added a film with no TMDB id")
	}
	if got, err := s.PlanEntryByTmdb(ctx, 603); err != nil || got.ID != e.ID {
		t.Fatalf("by tmdb %+v %v", got, err)
	}

	if err := s.CompletePlanEntry(ctx, e.ID, 42); err != nil {
		t.Fatal(err)
	}
	done, _ := s.PlanEntry(ctx, e.ID)
	if done.Status != PlanDone || done.ReleaseID != 42 || done.DoneAt == nil {
		t.Fatalf("completed %+v", done)
	}
	// Back to do: the upload is forgotten.
	todo, note := PlanDoing, "nuova nota"
	got, err := s.UpdatePlanEntry(ctx, e.ID, PlanUpdate{Status: &todo, Note: &note})
	if err != nil || got.Status != PlanDoing || got.ReleaseID != 0 || got.DoneAt != nil || got.Note != "nuova nota" {
		t.Fatalf("updated %+v %v", got, err)
	}
	bad := "boh"
	if _, err := s.UpdatePlanEntry(ctx, e.ID, PlanUpdate{Status: &bad}); err == nil {
		t.Fatal("accepted an invalid status")
	}
	// A dropped film is not completed by an upload.
	dropped := PlanDropped
	if _, err := s.UpdatePlanEntry(ctx, e.ID, PlanUpdate{Status: &dropped}); err != nil {
		t.Fatal(err)
	}
	s.CompletePlanEntry(ctx, e.ID, 43)
	if got, _ := s.PlanEntry(ctx, e.ID); got.Status != PlanDropped || got.ReleaseID != 0 {
		t.Fatalf("dropped %+v", got)
	}

	if list, err := s.PlanEntries(ctx); err != nil || len(list) != 1 {
		t.Fatalf("list %+v %v", list, err)
	}
	if err := s.DeletePlanEntry(ctx, e.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeletePlanEntry(ctx, e.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted twice: %v", err)
	}
}
