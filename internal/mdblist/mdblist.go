// Package mdblist reads lists of films from MDBList (mdblist.com): the
// collections the user follows (a universe, a studio) beyond the sagas of
// TMDB.
package mdblist

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var ErrNotFound = errors.New("lista non trovata su MDBList")

type Client struct {
	// BaseURL is the API root, replaced by the tests.
	BaseURL string
	key     string
	http    *http.Client
	// MinInterval spaces out requests, as MDBList asks of free accounts.
	MinInterval time.Duration

	mu   sync.Mutex
	last time.Time
}

func New(key string) *Client {
	return &Client{
		BaseURL:     "https://api.mdblist.com",
		key:         strings.TrimSpace(key),
		http:        &http.Client{Timeout: 30 * time.Second},
		MinInterval: time.Second,
	}
}

// pathRe is the path of a list after "/lists/": "user/slug" or
// "user/external/123" (a list of Trakt or IMDb mirrored by MDBList).
var pathRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*/(?:external/[0-9]+|[A-Za-z0-9][A-Za-z0-9._-]*)$`)

// ParseURL returns the path of a list from its address on MDBList
// ("https://mdblist.com/lists/user/slug", "/json" at the end too) or from
// the path itself ("user/slug").
func ParseURL(s string) (string, error) {
	p := strings.TrimSpace(s)
	if strings.Contains(p, "://") {
		u, err := url.Parse(p)
		if err != nil || !strings.HasSuffix(strings.ToLower(u.Hostname()), "mdblist.com") {
			return "", fmt.Errorf("%q non è l'indirizzo di una lista di MDBList", s)
		}
		p = u.Path
	}
	p = strings.Trim(p, "/")
	p = strings.TrimPrefix(p, "lists/")
	p = strings.TrimSuffix(p, "/json")
	if !pathRe.MatchString(p) || strings.HasPrefix(p, "official/") {
		return "", fmt.Errorf("%q non è l'indirizzo di una lista di MDBList (es. https://mdblist.com/lists/utente/nome-lista)", s)
	}
	return p, nil
}

// User verifies the key and returns the name of its account: GET /user.
func (c *Client) User(ctx context.Context) (string, error) {
	var res struct {
		Username string `json:"username"`
		Name     string `json:"name"`
	}
	if _, err := c.get(ctx, "user", nil, &res); err != nil {
		return "", err
	}
	if res.Username != "" {
		return res.Username, nil
	}
	return res.Name, nil
}

// PageURL is the address of the list on the site.
func PageURL(path string) string { return "https://mdblist.com/lists/" + path }

// apiPath is the list in the API: "lists/user/slug" or "external/lists/123".
func apiPath(path string) string {
	if user, id, ok := strings.Cut(path, "/external/"); ok && user != "" {
		return "external/lists/" + id
	}
	return "lists/" + path
}

// List is a list of MDBList.
type List struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Slug  string `json:"slug"`
	Items int    `json:"items"`
	User  string `json:"user_name"`
}

// List returns the name and size of a list.
func (c *Client) List(ctx context.Context, path string) (List, error) {
	var raw json.RawMessage
	if _, err := c.get(ctx, apiPath(path), nil, &raw); err != nil {
		return List{}, err
	}
	// An array with the list, or the list.
	var ls []List
	if json.Unmarshal(raw, &ls) == nil {
		if len(ls) == 0 {
			return List{}, ErrNotFound
		}
		return ls[0], nil
	}
	var l List
	if err := json.Unmarshal(raw, &l); err != nil {
		return List{}, fmt.Errorf("risposta di MDBList non valida: %w", err)
	}
	return l, nil
}

// Item is a film of a list.
type Item struct {
	// The film on TMDB; 0 when MDBList does not know it.
	TmdbID    int64  `json:"id"`
	ImdbID    string `json:"imdb_id"`
	Title     string `json:"title"`
	Mediatype string `json:"mediatype"`
	// Position in the list.
	Rank int `json:"rank"`
}

// Imdb is the number of the IMDb id ("tt0111161" → 111161), 0 if none.
func (it Item) Imdb() int64 {
	n, _ := strconv.ParseInt(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(it.ImdbID)), "tt"), 10, 64)
	return n
}

// pageSize is the most items MDBList gives in one answer.
const pageSize = 1000

// maxPages stops a list that keeps answering with more.
const maxPages = 50

// Items returns the films of a list, in its order.
func (c *Client) Items(ctx context.Context, path string) ([]Item, error) {
	itemsPath := apiPath(path) + "/items/"
	if !strings.HasPrefix(itemsPath, "external/") {
		itemsPath += "movie"
	}
	var out []Item
	for page := 0; ; page++ {
		if page >= maxPages {
			return nil, fmt.Errorf("lista troppo lunga (oltre %d film)", maxPages*pageSize)
		}
		var raw json.RawMessage
		h, err := c.get(ctx, itemsPath, url.Values{"limit": {strconv.Itoa(pageSize)}, "offset": {strconv.Itoa(len(out))}}, &raw)
		if err != nil {
			return nil, err
		}
		items, err := parseItems(raw)
		if err != nil {
			return nil, err
		}
		for _, it := range items {
			if it.Mediatype == "" || strings.EqualFold(it.Mediatype, "movie") {
				out = append(out, it)
			}
		}
		if len(items) == 0 || !strings.EqualFold(h.Get("X-Has-More"), "true") {
			break
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i].Rank, out[j].Rank
		if (a == 0) != (b == 0) {
			return b == 0
		}
		return a < b
	})
	return out, nil
}

// parseItems reads an answer of items: {"movies": […], "shows": […]}, or
// an array.
func parseItems(raw json.RawMessage) ([]Item, error) {
	var items []Item
	if json.Unmarshal(raw, &items) == nil {
		return items, nil
	}
	var res struct {
		Movies []Item `json:"movies"`
		Items  []Item `json:"items"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("risposta di MDBList non valida: %w", err)
	}
	if len(res.Movies) == 0 {
		return res.Items, nil
	}
	return res.Movies, nil
}

func (c *Client) get(ctx context.Context, path string, q url.Values, out any) (http.Header, error) {
	if err := c.wait(ctx); err != nil {
		return nil, err
	}
	if q == nil {
		q = url.Values{}
	}
	q.Set("apikey", c.key)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.BaseURL, "/")+"/"+path+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
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
		return nil, fmt.Errorf("MDBList: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, fmt.Errorf("MDBList: %w", err)
	}
	// Errors come as {"response": false, "error": "…"}, with any status.
	var fail struct {
		Response *bool  `json:"response"`
		Error    string `json:"error"`
	}
	_ = json.Unmarshal(body, &fail)
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, errors.New("limite di richieste di MDBList raggiunto: riprova più tardi")
	case resp.StatusCode == http.StatusNotFound:
		return nil, ErrNotFound
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, errors.New("MDBList ha rifiutato la chiave: controlla BOOKRR_MDBLIST_API_KEY")
	case resp.StatusCode != http.StatusOK:
		if fail.Error != "" {
			return nil, fmt.Errorf("MDBList ha risposto HTTP %d: %s", resp.StatusCode, fail.Error)
		}
		return nil, fmt.Errorf("MDBList ha risposto HTTP %d", resp.StatusCode)
	case fail.Response != nil && !*fail.Response:
		if strings.Contains(strings.ToLower(fail.Error), "not found") {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("MDBList: %s", fail.Error)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return nil, fmt.Errorf("risposta di MDBList non valida: %w", err)
	}
	return resp.Header, nil
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
