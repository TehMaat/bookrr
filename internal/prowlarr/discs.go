package prowlarr

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"log/slog"
	"sort"
	"time"

	"github.com/tehmaat/bookrr/internal/store"
	"github.com/tehmaat/bookrr/internal/unit3d"
)

// discLimit is how many releases a search of the full discs reads: more
// than the search of the best, the discs may be far down the list.
const discLimit = 500

// Disc is a full disc of a film found with Prowlarr.
type Disc struct {
	// ID tells it from the other discs of the film, to load it.
	ID      string `json:"id"`
	Title   string `json:"title"`
	Indexer string `json:"indexer"`
	// The page of the release on its indexer.
	URL        string `json:"url"`
	Size       int64  `json:"size"`
	Seeders    int64  `json:"seeders"`
	Resolution string `json:"resolution"`
	// "2160p Full Disc DV HDR ITA ENG"
	Quality string `json:"quality"`
	// Its .torrent can be loaded on the plan: false for a magnet link only.
	Loadable bool `json:"loadable"`

	// The .torrent through Prowlarr, with its key: never sent to the user.
	download string
}

// DiscSearch is the last search of the full discs of a film.
type DiscSearch struct {
	Searching bool `json:"searching"`
	// "" when not searched yet (since bookrr started).
	CheckedAt string `json:"checkedAt"`
	// Why the last search failed; the discs are those of the one before.
	Error string `json:"error"`
	// Best first: resolution, then seeders.
	Discs []Disc `json:"discs"`
}

// discs are the full discs of the film among the releases found.
func (f *film) discs(found []Release) []Disc {
	out := []Disc{}
	seen := map[string]bool{}
	for _, r := range found {
		if !r.Torrent || !f.matches(r) || !r.FullDisc() {
			continue
		}
		id := discID(r)
		if seen[id] {
			continue
		}
		seen[id] = true
		q := unit3d.DiscQuality(r.Title)
		out = append(out, Disc{ID: id, Title: r.Title, Indexer: r.Indexer, URL: r.InfoURL, Size: r.Size,
			Seeders: r.Seeders, Resolution: q.Resolution, Quality: q.Label, Loadable: r.DownloadURL != "", download: r.DownloadURL})
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if x, y := unit3d.ResolutionRank(a.Resolution), unit3d.ResolutionRank(b.Resolution); x != y {
			return x > y
		}
		if a.Seeders != b.Seeders {
			return a.Seeders > b.Seeders
		}
		return a.Size > b.Size
	})
	return out
}

// discID identifies a release by its indexer and GUID (its title without).
func discID(r Release) string {
	k := r.GUID
	if k == "" {
		k = r.Title
	}
	h := sha1.Sum([]byte(r.Indexer + "\x00" + k))
	return hex.EncodeToString(h[:8])
}

// Discs returns the last search of the full discs of a film of the plan.
func (f *Finder) Discs(e store.PlanEntry) DiscSearch {
	f.stateMu.RLock()
	defer f.stateMu.RUnlock()
	d := f.discs[FilmKey(e.TmdbID, e.ImdbID)]
	if d.Discs == nil {
		d.Discs = []Disc{}
	}
	return d
}

// StartDiscs marks the full discs of a film of the plan as being searched;
// false if they are already.
func (f *Finder) StartDiscs(e store.PlanEntry) bool {
	key := FilmKey(e.TmdbID, e.ImdbID)
	f.stateMu.Lock()
	defer f.stateMu.Unlock()
	d := f.discs[key]
	if d.Searching {
		return false
	}
	d.Searching, d.Error = true, ""
	f.discs[key] = d
	return true
}

// SearchDiscs searches the full discs of a film of the plan on Prowlarr,
// after StartDiscs, whether it is out or not. The client keeps it spaced
// out from the other searches.
func (f *Finder) SearchDiscs(ctx context.Context, e store.PlanEntry) error {
	fm := planFilm(e)
	found, err := f.client.searchMovie(ctx, fm.imdb, fm.tmdb, discLimit)
	ts := f.now().UTC().Format(time.RFC3339)
	f.stateMu.Lock()
	d := f.discs[fm.key]
	d.Searching = false
	if err != nil {
		d.Error = err.Error()
	} else {
		d.Discs, d.CheckedAt = fm.discs(found), ts
	}
	f.discs[fm.key] = d
	f.stateMu.Unlock()
	if err != nil {
		if ctx.Err() == nil {
			slog.Warn("Prowlarr search of the full discs", "film", e.Title, "err", err)
		}
		return err
	}
	slog.Info("Prowlarr search of the full discs", "film", e.Title, "found", len(d.Discs))
	return nil
}

// Disc returns a full disc of the last search of a film of the plan.
func (f *Finder) Disc(e store.PlanEntry, id string) (Disc, bool) {
	for _, d := range f.Discs(e).Discs {
		if d.ID == id {
			return d, true
		}
	}
	return Disc{}, false
}

// DownloadDisc fetches the .torrent of a full disc through Prowlarr, at
// most max bytes.
func (f *Finder) DownloadDisc(ctx context.Context, d Disc, max int64) ([]byte, error) {
	return f.client.Download(ctx, d.download, max)
}
