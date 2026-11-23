package arr

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type Radarr struct{ client }

func NewRadarr(base, key string) *Radarr {
	return &Radarr{newClient("Radarr", "BOOKRR_RADARR_API_KEY", base, key)}
}

type movie struct {
	ID      int64      `json:"id"`
	Title   string     `json:"title"`
	Year    int        `json:"year"`
	HasFile bool       `json:"hasFile"`
	File    *movieFile `json:"movieFile"`
}

type movieFile struct {
	Size             int64  `json:"size"`
	Path             string `json:"path"`
	RelativePath     string `json:"relativePath"`
	SceneName        string `json:"sceneName"`
	OriginalFilePath string `json:"originalFilePath"`
}

// fromTorrent reports whether the file was imported from a torrent named name.
func (f *movieFile) fromTorrent(name string) bool {
	if f == nil {
		return false
	}
	n := strings.ToLower(strings.TrimSpace(name))
	s := stem(name)
	for _, c := range []string{strings.ToLower(strings.TrimSpace(f.SceneName)), firstDir(f.OriginalFilePath)} {
		if c != "" && (c == n || c == s) {
			return true
		}
	}
	for _, c := range []string{stem(f.OriginalFilePath), stem(f.RelativePath)} {
		if c != "" && c == s {
			return true
		}
	}
	return false
}

// Movie is the film of Radarr a torrent brought in.
type Movie struct {
	ID      int64  `json:"id"`
	Title   string `json:"title"`
	Year    int    `json:"year"`
	HasFile bool   `json:"hasFile"`
	// The file Radarr has now is the one imported from the torrent: false
	// when the film was upgraded since, and deleting it would throw away a
	// file that is not on the disk.
	SameFile bool `json:"sameFile"`
	// "history" (the download of the torrent) or "name" (the name of the file).
	Via string `json:"via"`
}

// Deletable reports whether the film can be deleted with its files without
// losing anything that is not archived.
func (m Movie) Deletable() bool { return !m.HasFile || m.SameFile }

// RadarrLookup finds the films of many torrents, reading the whole library
// at most once.
type RadarrLookup struct {
	r      *Radarr
	movies []movie
	loaded bool
}

func (r *Radarr) Lookup() *RadarrLookup { return &RadarrLookup{r: r} }

// Find returns the film the torrent brought in, nil when Radarr has none.
func (l *RadarrLookup) Find(ctx context.Context, hash, name string) (*Movie, error) {
	recs, err := l.r.history(ctx, hash)
	if err != nil {
		return nil, err
	}
	var id int64
	var imported string
	for _, rec := range recs {
		if rec.MovieID == 0 {
			continue
		}
		switch {
		case rec.EventType == eventImported && imported == "":
			id, imported = rec.MovieID, rec.importedPath()
		case id == 0 && rec.EventType == eventGrabbed:
			id = rec.MovieID
		}
	}
	if id != 0 {
		var m movie
		err := l.r.do(ctx, http.MethodGet, fmt.Sprintf("/api/v3/movie/%d", id), nil, nil, &m)
		switch {
		case err == nil:
			same := m.HasFile && m.File != nil && (samePath(m.File.Path, imported) || m.File.fromTorrent(name))
			return &Movie{ID: m.ID, Title: m.Title, Year: m.Year, HasFile: m.HasFile, SameFile: same, Via: "history"}, nil
		case !errors.Is(err, ErrNotFound):
			return nil, err
		}
		// Deleted from Radarr since: look for it by name.
	}
	if err := l.load(ctx); err != nil {
		return nil, err
	}
	for _, m := range l.movies {
		if m.HasFile && m.File.fromTorrent(name) {
			return &Movie{ID: m.ID, Title: m.Title, Year: m.Year, HasFile: true, SameFile: true, Via: "name"}, nil
		}
	}
	return nil, nil
}

// DeleteMovie deletes the film with its files, without adding it to the
// import exclusions. A film already gone is not an error.
func (r *Radarr) DeleteMovie(ctx context.Context, id int64) error {
	q := url.Values{"deleteFiles": {"true"}, "addImportExclusion": {"false"}}
	err := r.do(ctx, http.MethodDelete, fmt.Sprintf("/api/v3/movie/%d", id), q, nil, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

func (l *RadarrLookup) load(ctx context.Context) error {
	if l.loaded {
		return nil
	}
	if err := l.r.do(ctx, http.MethodGet, "/api/v3/movie", nil, nil, &l.movies); err != nil {
		return err
	}
	l.loaded = true
	return nil
}

// Download is the torrent a file of Radarr's library was imported from.
type Download struct {
	Hash string // lowercase
	Size int64  // of the file
}

// DownloadsBySize finds the torrents of the films whose file has one of the
// sizes: for the copies of Radarr's files, named as Radarr names them.
func (l *RadarrLookup) DownloadsBySize(ctx context.Context, sizes map[int64]bool) ([]Download, error) {
	if err := l.load(ctx); err != nil {
		return nil, err
	}
	var out []Download
	for _, m := range l.movies {
		if !m.HasFile || m.File == nil || !sizes[m.File.Size] {
			continue
		}
		var recs []record
		q := url.Values{"movieId": {strconv.FormatInt(m.ID, 10)}}
		if err := l.r.do(ctx, http.MethodGet, "/api/v3/history/movie", q, nil, &recs); err != nil {
			return nil, err
		}
		// The import of the file Radarr has now, or else the last import.
		var hash string
		for _, r := range recs {
			id := strings.ToLower(strings.TrimSpace(r.DownloadID))
			if r.EventType != eventImported || id == "" {
				continue
			}
			if samePath(r.importedPath(), m.File.Path) {
				hash = id
				break
			}
			if hash == "" {
				hash = id
			}
		}
		if hash != "" {
			out = append(out, Download{Hash: hash, Size: m.File.Size})
		}
	}
	return out, nil
}
