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
}

type release struct {
	Title   string  `json:"title"`
	Indexer string  `json:"indexer"`
	InfoURL string  `json:"infoUrl"`
	GUID    string  `json:"guid"`
	Size    flexInt `json:"size"`
	Seeders flexInt `json:"seeders"`
	ImdbID  flexInt `json:"imdbId"`
	TmdbID  flexInt `json:"tmdbId"`
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
	var q string
	if imdbID != 0 {
		q += fmt.Sprintf("{ImdbId:tt%07d}", imdbID)
	}
	if tmdbID != 0 {
		q += fmt.Sprintf("{TmdbId:%d}", tmdbID)
	}
	if q == "" {
		return nil, errors.New("film senza id IMDb né TMDB")
	}
	var res []release
	if err := c.get(ctx, "/api/v1/search", url.Values{"query": {q}, "type": {"movie"}, "categories": {"2000"}, "limit": {"100"}}, &res); err != nil {
		return nil, err
	}
	out := make([]Release, 0, len(res))
	for _, r := range res {
		link := r.InfoURL
		if link == "" && strings.HasPrefix(r.GUID, "http") {
			link = r.GUID
		}
		out = append(out, Release{Title: r.Title, Indexer: r.Indexer, InfoURL: link, Size: int64(r.Size),
			Seeders: int64(r.Seeders), ImdbID: int64(r.ImdbID), TmdbID: int64(r.TmdbID)})
	}
	return out, nil
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
		return errors.New("Prowlarr ha rifiutato la chiave: controlla BOOKRR_PROWLARR_API_KEY")
	case resp.StatusCode != http.StatusOK:
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("Prowlarr ha risposto HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(out); err != nil {
		return fmt.Errorf("risposta di Prowlarr non valida: %w", err)
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
