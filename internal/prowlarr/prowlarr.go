// Package prowlarr searches the indexers of a Prowlarr for the releases of
// the films the user uploaded, to know the best resolution that exists of
// each of them.
package prowlarr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tehmaat/bookrr/internal/unit3d"
)

type Client struct {
	base string
	key  string
	http *http.Client
	// MinInterval spaces out the searches: each one goes to every indexer
	// of Prowlarr, that is to every tracker.
	MinInterval time.Duration

	mu   sync.Mutex
	last time.Time
}

func New(base, key string) *Client {
	return &Client{
		base: strings.TrimRight(base, "/"),
		key:  strings.TrimSpace(key),
		// Prowlarr answers when the slowest indexer has.
		http:        &http.Client{Timeout: 3 * time.Minute},
		MinInterval: 30 * time.Second,
	}
}

// Release is a torrent (or NZB) found by an indexer.
type Release struct {
	Title   string
	Indexer string
	InfoURL string
	Size    int64
	Seeders int64
	// Ids of the film, when the indexer gives them (0 otherwise).
	ImdbID int64
	TmdbID int64
	// GUID identifies the release on its indexer.
	GUID string
	// DownloadURL is the .torrent through Prowlarr: it holds the key of
	// Prowlarr, never shown to the user. "" when there is none (a magnet
	// link only).
	DownloadURL string
	// Torrent is false for an NZB.
	Torrent bool
	// BluRay is true when the indexer files it under Movies/BluRay, where
	// the full discs go.
	BluRay bool
}

// FullDisc reports whether the release is a full disc.
func (r Release) FullDisc() bool { return unit3d.FullDisc(r.Title, r.Size, r.BluRay) }

// The category of the full discs for Newznab and Torznab.
const bluRayCategory = 2050

type release struct {
	Title       string  `json:"title"`
	Indexer     string  `json:"indexer"`
	InfoURL     string  `json:"infoUrl"`
	GUID        string  `json:"guid"`
	DownloadURL string  `json:"downloadUrl"`
	Protocol    string  `json:"protocol"`
	Size        flexInt `json:"size"`
	Seeders     flexInt `json:"seeders"`
	ImdbID      flexInt `json:"imdbId"`
	TmdbID      flexInt `json:"tmdbId"`
	Categories  []struct {
		ID flexInt `json:"id"`
	} `json:"categories"`
}

// flexInt accepts a number, a numeric string ("tt0133093" too), or null.
type flexInt int64

func (f *flexInt) UnmarshalJSON(b []byte) error {
	var n json.Number
	if err := json.Unmarshal(b, &n); err == nil {
		v, _ := strconv.ParseFloat(n.String(), 64)
		*f = flexInt(v)
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		v, _ := strconv.ParseInt(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(s)), "tt"), 10, 64)
		*f = flexInt(v)
	}
	return nil
}

// SearchMovie asks every indexer for the releases of a film by its ids
// (0 for an unknown one), in the categories of films.
func (c *Client) SearchMovie(ctx context.Context, imdbID, tmdbID int64) ([]Release, error) {
	return c.searchMovie(ctx, imdbID, tmdbID, 100)
}

// searchMovie is SearchMovie with at most limit releases.
func (c *Client) searchMovie(ctx context.Context, imdbID, tmdbID int64, limit int) ([]Release, error) {
	var q string
	if imdbID != 0 {
		q += fmt.Sprintf("{ImdbId:tt%07d}", imdbID)
	}
	if tmdbID != 0 {
		q += fmt.Sprintf("{TmdbId:%d}", tmdbID)
	}
	if q == "" {
		return nil, errors.New("film with neither IMDb nor TMDB id")
	}
	var res []release
	if err := c.get(ctx, "/api/v1/search", url.Values{"query": {q}, "type": {"movie"}, "categories": {"2000"}, "limit": {strconv.Itoa(limit)}}, &res); err != nil {
		return nil, err
	}
	out := make([]Release, 0, len(res))
	for _, r := range res {
		link := r.InfoURL
		if link == "" && strings.HasPrefix(r.GUID, "http") {
			link = r.GUID
		}
		rel := Release{Title: r.Title, Indexer: r.Indexer, InfoURL: link, Size: int64(r.Size),
			Seeders: int64(r.Seeders), ImdbID: int64(r.ImdbID), TmdbID: int64(r.TmdbID),
			GUID: r.GUID, DownloadURL: r.DownloadURL, Torrent: !strings.EqualFold(r.Protocol, "usenet")}
		for _, c := range r.Categories {
			rel.BluRay = rel.BluRay || c.ID == bluRayCategory
		}
		out = append(out, rel)
	}
	return out, nil
}

// errMagnet is a release Prowlarr has only as a magnet link.
var errMagnet = errors.New("Prowlarr has only a magnet link of this release, not a .torrent file")

// Download fetches the .torrent of a release through Prowlarr, at most max
// bytes. The address is always on the host of Prowlarr as configured,
// whatever host Prowlarr wrote in it.
func (c *Client) Download(ctx context.Context, downloadURL string, max int64) ([]byte, error) {
	if downloadURL == "" {
		return nil, errMagnet
	}
	u, err := url.Parse(downloadURL)
	if err != nil {
		return nil, fmt.Errorf("invalid download address from Prowlarr: %w", err)
	}
	base, err := url.Parse(c.base)
	if err != nil {
		return nil, err
	}
	// Not spaced out as the searches: it goes to one indexer only.
	u.Scheme, u.Host, u.User = base.Scheme, base.Host, nil
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Api-Key", c.key)
	req.Header.Set("User-Agent", "bookrr")
	hc := *c.http
	hc.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if r.URL.Scheme != "http" && r.URL.Scheme != "https" {
			return errMagnet
		}
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		return nil
	}
	resp, err := hc.Do(req)
	if errors.Is(err, errMagnet) {
		return nil, errMagnet
	}
	if err != nil {
		// The address holds the key: not in the message.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return nil, fmt.Errorf("Prowlarr: %w", err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, errors.New("Prowlarr rejected the key: check BOOKRR_PROWLARR_API_KEY")
	case resp.StatusCode != http.StatusOK:
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return nil, fmt.Errorf("Prowlarr answered HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return nil, fmt.Errorf("Prowlarr: %w", err)
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("the .torrent file is over %d MB", max>>20)
	}
	return data, nil
}

// Version verifies address and key: GET /api/v1/system/status.
func (c *Client) Version(ctx context.Context) (string, error) {
	var res struct {
		Version string `json:"version"`
	}
	if err := c.get(ctx, "/api/v1/system/status", nil, &res); err != nil {
		return "", err
	}
	return res.Version, nil
}

func (c *Client) get(ctx context.Context, path string, q url.Values, out any) error {
	if err := c.wait(ctx); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Api-Key", c.key)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "bookrr")
	defer c.done()
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("Prowlarr: %w", err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return errors.New("Prowlarr rejected the key: check BOOKRR_PROWLARR_API_KEY")
	case resp.StatusCode != http.StatusOK:
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("Prowlarr answered HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(out); err != nil {
		return fmt.Errorf("invalid Prowlarr response: %w", err)
	}
	return nil
}

// wait keeps MinInterval between the end of a search and the next one.
func (c *Client) wait(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if d := time.Until(c.last.Add(c.MinInterval)); d > 0 {
		t := time.NewTimer(d)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
	c.last = time.Now()
	return nil
}

func (c *Client) done() {
	c.mu.Lock()
	c.last = time.Now()
	c.mu.Unlock()
}
