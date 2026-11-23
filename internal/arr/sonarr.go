package arr

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
)

type Sonarr struct{ client }

func NewSonarr(base, key string) *Sonarr {
	return &Sonarr{newClient("Sonarr", "BOOKRR_SONARR_API_KEY", base, key)}
}

type series struct {
	ID         int64  `json:"id"`
	Title      string `json:"title"`
	Year       int    `json:"year"`
	TitleSlug  string `json:"titleSlug"`
	Statistics struct {
		EpisodeFileCount int `json:"episodeFileCount"`
	} `json:"statistics"`
}

type episode struct {
	ID            int64 `json:"id"`
	SeasonNumber  int   `json:"seasonNumber"`
	HasFile       bool  `json:"hasFile"`
	EpisodeFileID int64 `json:"episodeFileId"`
}

type episodeFile struct {
	ID   int64  `json:"id"`
	Path string `json:"path"`
}

// Episodes are the episodes of a series of Sonarr a torrent brought in.
type Episodes struct {
	SeriesID int64  `json:"seriesId"`
	Title    string `json:"title"`
	Year     int    `json:"year"`
	// For the link to the series in Sonarr's interface.
	Slug    string `json:"slug"`
	URL     string `json:"url"`
	Seasons []int  `json:"seasons"`
	// Episodes imported from the torrent.
	Episodes int `json:"episodes"`
	// The episode files Sonarr has now from the torrent: deleted.
	Files []int64 `json:"files"`
	// The episodes still holding a file of the torrent, or none: no longer
	// monitored, so Sonarr does not download them again.
	Unmonitor []int64 `json:"unmonitor"`
	// Episodes imported from the torrent that have another file now
	// (upgraded since): left as they are.
	Upgraded int `json:"upgraded"`
	// Episode files of the whole series.
	SeriesFiles int `json:"seriesFiles"`
}

// Find returns the episodes the torrent brought in, nil when Sonarr has none.
func (s *Sonarr) Find(ctx context.Context, hash string) (*Episodes, error) {
	recs, err := s.history(ctx, hash)
	if err != nil {
		return nil, err
	}
	// The series of the torrent: the one with most events.
	count := map[int64]int{}
	for _, r := range recs {
		if r.SeriesID != 0 {
			count[r.SeriesID]++
		}
	}
	var id int64
	for sid, n := range count {
		if id == 0 || n > count[id] || n == count[id] && sid < id {
			id = sid
		}
	}
	if id == 0 {
		return nil, nil
	}
	imported := map[int64]string{} // episode → path imported
	grabbed := map[int64]bool{}
	for _, r := range recs {
		if r.SeriesID != id || r.EpisodeID == 0 {
			continue
		}
		switch r.EventType {
		case eventImported:
			imported[r.EpisodeID] = r.importedPath()
		case eventGrabbed:
			grabbed[r.EpisodeID] = true
		}
	}

	var se series
	if err := s.do(ctx, http.MethodGet, fmt.Sprintf("/api/v3/series/%d", id), nil, nil, &se); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	q := url.Values{"seriesId": {strconv.FormatInt(id, 10)}}
	var eps []episode
	if err := s.do(ctx, http.MethodGet, "/api/v3/episode", q, nil, &eps); err != nil {
		return nil, err
	}
	var files []episodeFile
	if err := s.do(ctx, http.MethodGet, "/api/v3/episodefile", q, nil, &files); err != nil {
		return nil, err
	}
	paths := map[int64]string{}
	for _, f := range files {
		paths[f.ID] = f.Path
	}
	out := &Episodes{SeriesID: se.ID, Title: se.Title, Year: se.Year, Slug: se.TitleSlug, SeriesFiles: len(files)}
	seasons, fileSet := map[int]bool{}, map[int64]bool{}
	for _, e := range eps {
		p, ok := imported[e.ID]
		if !ok && !grabbed[e.ID] {
			continue
		}
		out.Episodes++
		seasons[e.SeasonNumber] = true
		switch {
		case !e.HasFile || e.EpisodeFileID == 0:
			out.Unmonitor = append(out.Unmonitor, e.ID)
		case ok && samePath(paths[e.EpisodeFileID], p):
			out.Unmonitor = append(out.Unmonitor, e.ID)
			if !fileSet[e.EpisodeFileID] {
				fileSet[e.EpisodeFileID] = true
				out.Files = append(out.Files, e.EpisodeFileID)
			}
		default:
			out.Upgraded++
		}
	}
	for n := range seasons {
		out.Seasons = append(out.Seasons, n)
	}
	sort.Ints(out.Seasons)
	if out.Episodes == 0 {
		return nil, nil
	}
	return out, nil
}

// SeriesRemoval is what is removed from a series for the torrents archived.
type SeriesRemoval struct {
	SeriesID    int64
	Files       []int64
	Unmonitor   []int64
	SeriesFiles int
}

// Whole reports whether every file of the series goes: the series is
// deleted instead.
func (r SeriesRemoval) Whole() bool { return len(r.Files) > 0 && len(r.Files) >= r.SeriesFiles }

// Remove deletes the episode files and stops monitoring the episodes; when
// no file of the series is left, it deletes the series (without adding it
// to the import list exclusions). It reports whether the series was deleted.
func (s *Sonarr) Remove(ctx context.Context, r SeriesRemoval) (bool, error) {
	if r.Whole() {
		q := url.Values{"deleteFiles": {"true"}, "addImportListExclusion": {"false"}}
		err := s.do(ctx, http.MethodDelete, fmt.Sprintf("/api/v3/series/%d", r.SeriesID), q, nil, nil)
		if errors.Is(err, ErrNotFound) {
			err = nil
		}
		return err == nil, err
	}
	for _, id := range r.Files {
		if err := s.do(ctx, http.MethodDelete, fmt.Sprintf("/api/v3/episodefile/%d", id), nil, nil, nil); err != nil && !errors.Is(err, ErrNotFound) {
			return false, err
		}
	}
	if len(r.Unmonitor) > 0 {
		body := map[string]any{"episodeIds": r.Unmonitor, "monitored": false}
		if err := s.do(ctx, http.MethodPut, "/api/v3/episode/monitor", nil, body, nil); err != nil {
			return false, err
		}
	}
	return false, nil
}
