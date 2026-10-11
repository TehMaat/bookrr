// Package tmdb reads series (and their seasons) and movies (and their
// sagas) from The Movie Database.
package tmdb

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

var ErrNotFound = errors.New("non trovato su TMDB")

// RateLimitError means TMDB asked to slow down (HTTP 429).
type RateLimitError struct{ RetryAfter time.Duration }

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("limite di richieste di TMDB raggiunto, riprovo tra %s", e.RetryAfter)
}

type Client struct {
	// BaseURL is the API root, replaced by the tests.
	BaseURL string
	key     string
	http    *http.Client
	// Language of titles and season names.
	Language string
	// MinInterval spaces out requests, well below TMDB's limits.
	MinInterval time.Duration

	mu   sync.Mutex
	last time.Time
}

// New takes either an API key (v3) or an API read access token (v4).
func New(key string) *Client {
	return &Client{
		BaseURL:     "https://api.themoviedb.org/3",
		key:         strings.TrimSpace(key),
		http:        &http.Client{Timeout: 30 * time.Second},
		Language:    "it-IT",
		MinInterval: 100 * time.Millisecond,
	}
}

// Season of a series. Specials are season 0.
type Season struct {
	Number   int    `json:"season_number"`
	Name     string `json:"name"`
	AirDate  string `json:"air_date"` // "2019-04-14", empty when unknown
	Episodes int    `json:"episode_count"`
}

// Aired reports whether the season had started airing on day (YYYY-MM-DD).
func (s Season) Aired(day string) bool { return s.AirDate != "" && s.AirDate <= day }

type Show struct {
	ID           int64    `json:"id"`
	Name         string   `json:"name"`
	OriginalName string   `json:"original_name"`
	FirstAirDate string   `json:"first_air_date"`
	Status       string   `json:"status"`
	PosterPath   string   `json:"poster_path"`
	Seasons      []Season `json:"seasons"`
}

// Year of the first episode, "" if unknown.
func (s Show) Year() string {
	if len(s.FirstAirDate) >= 4 {
		return s.FirstAirDate[:4]
	}
	return ""
}

// TV returns a series with its seasons: GET /tv/{id}.
func (c *Client) TV(ctx context.Context, id int64) (Show, error) {
	var s Show
	err := c.get(ctx, "/tv/"+strconv.FormatInt(id, 10), url.Values{"language": {c.Language}}, &s)
	return s, err
}

// FindTVByTVDB returns the TMDB id of the series with this TheTVDB id:
// GET /find/{id}?external_source=tvdb_id.
func (c *Client) FindTVByTVDB(ctx context.Context, tvdbID int64) (int64, error) {
	var res struct {
		TVResults []struct {
			ID int64 `json:"id"`
		} `json:"tv_results"`
	}
	if err := c.get(ctx, "/find/"+strconv.FormatInt(tvdbID, 10), url.Values{"external_source": {"tvdb_id"}}, &res); err != nil {
		return 0, err
	}
	if len(res.TVResults) == 0 || res.TVResults[0].ID == 0 {
		return 0, ErrNotFound
	}
	return res.TVResults[0].ID, nil
}

// Movie is a film, with the saga it belongs to (nil if none).
type Movie struct {
	ID            int64  `json:"id"`
	Title         string `json:"title"`
	OriginalTitle string `json:"original_title"`
	ReleaseDate   string `json:"release_date"`
	Collection    *struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"belongs_to_collection"`
}

// Movie returns a film: GET /movie/{id}.
func (c *Client) Movie(ctx context.Context, id int64) (Movie, error) {
	var m Movie
	err := c.get(ctx, "/movie/"+strconv.FormatInt(id, 10), url.Values{"language": {c.Language}}, &m)
	return m, err
}

// Part is a film of a saga.
type Part struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	ReleaseDate string `json:"release_date"` // "2001-12-19", empty when unknown
}

// Released reports whether the film was out on day (YYYY-MM-DD).
func (p Part) Released(day string) bool { return p.ReleaseDate != "" && p.ReleaseDate <= day }

// Year of release, "" if unknown.
func (p Part) Year() string {
	if len(p.ReleaseDate) >= 4 {
		return p.ReleaseDate[:4]
	}
	return ""
}

// Collection is a saga: its films, in no set order.
type Collection struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Parts []Part `json:"parts"`
}

// Collection returns a saga with its films: GET /collection/{id}.
func (c *Client) Collection(ctx context.Context, id int64) (Collection, error) {
	var col Collection
	err := c.get(ctx, "/collection/"+strconv.FormatInt(id, 10), url.Values{"language": {c.Language}}, &col)
	return col, err
}

// FindMovieByIMDB returns the TMDB id of the film with this IMDb id (the
// number of "tt0111161"): GET /find/tt…?external_source=imdb_id.
func (c *Client) FindMovieByIMDB(ctx context.Context, imdbID int64) (int64, error) {
	var res struct {
		MovieResults []struct {
			ID int64 `json:"id"`
		} `json:"movie_results"`
	}
	if err := c.get(ctx, fmt.Sprintf("/find/tt%07d", imdbID), url.Values{"external_source": {"imdb_id"}}, &res); err != nil {
		return 0, err
	}
	if len(res.MovieResults) == 0 || res.MovieResults[0].ID == 0 {
		return 0, ErrNotFound
	}
	return res.MovieResults[0].ID, nil
}

// bearer: v4 read access tokens are JWTs, sent as a header; v3 keys go in the query.
func (c *Client) bearer() bool { return strings.Count(c.key, ".") == 2 && len(c.key) > 60 }

func (c *Client) get(ctx context.Context, path string, q url.Values, out any) error {
	if err := c.wait(ctx); err != nil {
		return err
	}
	if q == nil {
		q = url.Values{}
	}
	if !c.bearer() {
		q.Set("api_key", c.key)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.BaseURL, "/")+path+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	if c.bearer() {
		req.Header.Set("Authorization", "Bearer "+c.key)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "bookrr")
	resp, err := c.http.Do(req)
	if err != nil {
		// The URL carries the API key: don't let it end up in the logs.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return fmt.Errorf("TMDB: %w", err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		wait := 10 * time.Second
		if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s > 0 {
			wait = time.Duration(s) * time.Second
		}
		return &RateLimitError{RetryAfter: wait}
	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case resp.StatusCode == http.StatusUnauthorized:
		return errors.New("TMDB ha rifiutato la chiave: controlla BOOKRR_TMDB_API_KEY")
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("TMDB ha risposto HTTP %d", resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(out); err != nil {
		return fmt.Errorf("risposta di TMDB non valida: %w", err)
	}
	return nil
}

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
