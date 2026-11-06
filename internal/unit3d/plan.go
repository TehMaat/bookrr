package unit3d

import (
	"context"
	"time"

	"github.com/tehmaat/bookrr/internal/store"
)

// CompletePlan marks done the open films of the release plan that the
// user uploaded after adding them (an upload from before is the version
// the plan is meant to replace), linked to the first such upload. It
// returns how many were completed.
func CompletePlan(ctx context.Context, s *store.Store, ups []store.TrackerUpload) (int, error) {
	entries, err := s.PlanEntries(ctx)
	if err != nil {
		return 0, err
	}
	done := 0
	for _, e := range entries {
		if !e.Open() {
			continue
		}
		added, err := time.Parse(time.RFC3339, e.CreatedAt)
		if err != nil {
			continue
		}
		var first store.TrackerUpload
		var firstAt time.Time
		for _, u := range ups {
			if u.TmdbID != e.TmdbID || !IsMovie(u.Category, u.Name, u.TvdbID) {
				continue
			}
			at, err := time.Parse(time.RFC3339, u.CreatedAt)
			if err != nil || at.Before(added) || (first.ID != 0 && !at.Before(firstAt)) {
				continue
			}
			first, firstAt = u, at
		}
		if first.ID == 0 {
			continue
		}
		if err := s.CompletePlanEntry(ctx, e.ID, first.ID); err != nil {
			return done, err
		}
		done++
	}
	return done, nil
}
