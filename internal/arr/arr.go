// Package arr talks to Radarr and Sonarr: it finds the film or the episodes
// a torrent brought into them (through their history of downloads, where
// the download id of a qBittorrent torrent is its hash) and removes them
// once the torrent is archived.
package arr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

// ErrNotFound is a film, series or file Radarr or Sonarr does not have.
var ErrNotFound = errors.New("not found")

type client struct {
	name string // "Radarr" or "Sonarr"
	env  string // the environment variable of the key, for the messages
	base string
	key  string
	http *http.Client
}

func newClient(name, env, base, key string) client {
	return client{name: name, env: env, base: strings.TrimRight(strings.TrimSpace(base), "/"), key: strings.TrimSpace(key),
		http: &http.Client{Timeout: time.Minute}}
}

func (c *client) do(ctx context.Context, method, p string, q url.Values, body, out any) error {
	u := c.base + p
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return err
	}
	req.Header.Set("X-Api-Key", c.key)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "bookrr")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", c.name, err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("%s rejected the key: check %s", c.name, c.env)
	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("%s answered HTTP %d: %s", c.name, resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(out); err != nil {
		return fmt.Errorf("invalid %s response: %w", c.name, err)
	}
	return nil
}

// Version verifies address and key.
func (c *client) Version(ctx context.Context) (string, error) {
	var res struct {
		Version string `json:"version"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/v3/system/status", nil, nil, &res); err != nil {
		return "", err
	}
	return res.Version, nil
}

// record is an event of the history of downloads.
type record struct {
	EventType string            `json:"eventType"`
	MovieID   int64             `json:"movieId"`
	SeriesID  int64             `json:"seriesId"`
	EpisodeID int64             `json:"episodeId"`
	Data      map[string]string `json:"data"`
}

const (
	eventGrabbed  = "grabbed"
	eventImported = "downloadFolderImported"
)

func (r record) importedPath() string {
	for k, v := range r.Data {
		if strings.EqualFold(k, "importedPath") {
			return v
		}
	}
	return ""
}

// history returns the events of the download with this torrent hash.
func (c *client) history(ctx context.Context, hash string) ([]record, error) {
	var res struct {
		Records []record `json:"records"`
	}
	q := url.Values{"page": {"1"}, "pageSize": {"500"}, "downloadId": {strings.ToUpper(hash)}}
	if err := c.do(ctx, http.MethodGet, "/api/v3/history", q, nil, &res); err != nil {
		return nil, err
	}
	return res.Records, nil
}

// samePath compares two paths of the same system, with any separator.
func samePath(a, b string) bool {
	norm := func(p string) string {
		return strings.TrimRight(strings.ReplaceAll(strings.TrimSpace(p), "\\", "/"), "/")
	}
	return a != "" && strings.EqualFold(norm(a), norm(b))
}

// stem is a file name without folders and without extension, lowercased.
func stem(p string) string {
	p = path.Base(strings.ReplaceAll(p, "\\", "/"))
	if ext := path.Ext(p); len(ext) >= 2 && len(ext) <= 5 {
		p = strings.TrimSuffix(p, ext)
	}
	return strings.ToLower(strings.TrimSpace(p))
}

// firstDir is the first folder of a relative path, lowercased ("" if none).
func firstDir(p string) string {
	p = strings.TrimLeft(strings.ReplaceAll(p, "\\", "/"), "/")
	if i := strings.Index(p, "/"); i > 0 {
		return strings.ToLower(p[:i])
	}
	return ""
}
