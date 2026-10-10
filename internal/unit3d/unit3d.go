// Package unit3d finds the info hash of a torrent on a UNIT3D tracker from
// its name, through the tracker's API.
package unit3d

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

var (
	ErrNotFound  = errors.New("nessun torrent con questo nome sul tracker")
	ErrAmbiguous = errors.New("più torrent con questo nome sul tracker")
	ErrNoHash    = errors.New("il tracker non fornisce né l'hash né il file .torrent")
)

// RateLimitError means the tracker asked to slow down (HTTP 429).
type RateLimitError struct{ RetryAfter time.Duration }

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("limite di richieste del tracker raggiunto, riprovo tra %s", e.RetryAfter)
}

// Final reports whether err is a definitive answer about the torrent (not
// found, ambiguous, no hash available) rather than a temporary failure.
func Final(err error) bool {
	return errors.Is(err, ErrNotFound) || errors.Is(err, ErrAmbiguous) || errors.Is(err, ErrNoHash) || errors.Is(err, errBadTorrent)
}

type Client struct {
	base   string
	apiKey string
	http   *http.Client
	// MinInterval spaces out requests: UNIT3D allows 30 API calls a minute.
	MinInterval time.Duration

	mu   sync.Mutex
	last time.Time
}

func New(baseURL, apiKey string) *Client {
	return &Client{
		base:        strings.TrimRight(baseURL, "/"),
		apiKey:      apiKey,
		http:        &http.Client{Timeout: 60 * time.Second},
		MinInterval: 2500 * time.Millisecond,
	}
}

// Match is the tracker torrent chosen for a name.
type Match struct {
	ID   string
	Name string
	Size int64
	Hash string
}

type torrent struct {
	ID         flexString `json:"id"`
	Attributes struct {
		Name         string  `json:"name"`
		Folder       string  `json:"folder"`
		Size         float64 `json:"size"`
		InfoHash     string  `json:"info_hash"`
		MagnetLink   string  `json:"magnet_link"`
		DownloadLink string  `json:"download_link"`
	} `json:"attributes"`
}

// flexString accepts an id sent either as a string or as a number.
type flexString string

func (f *flexString) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*f = flexString(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*f = flexString(n.String())
	return nil
}

// FindHash looks the name up on the tracker and returns the info hash of
// the torrent with exactly that name. When several torrents share the name,
// size (if known) picks the one with the same size.
func (c *Client) FindHash(ctx context.Context, name string, size int64) (Match, error) {
	q := url.Values{"name": {name}, "perPage": {"100"}}
	var res struct {
		Data []torrent `json:"data"`
	}
	if err := c.getJSON(ctx, c.base+"/api/torrents/filter?"+q.Encode(), &res); err != nil {
		return Match{}, err
	}
	t, err := pick(res.Data, name, size)
	if err != nil {
		return Match{}, err
	}
	m := Match{ID: string(t.ID), Name: t.Attributes.Name, Size: int64(t.Attributes.Size)}
	if m.Hash, err = c.hashOf(ctx, t); err != nil {
		return Match{}, err
	}
	return m, nil
}

func pick(ts []torrent, name string, size int64) (torrent, error) {
	want := normName(name)
	var same []torrent
	for _, t := range ts {
		if normName(t.Attributes.Name) == want || (t.Attributes.Folder != "" && normName(t.Attributes.Folder) == want) {
			same = append(same, t)
		}
	}
	if len(same) > 1 && size > 0 {
		var sized []torrent
		for _, t := range same {
			// A size typed by hand ("12,5 GB") is only approximate.
			if math.Abs(t.Attributes.Size-float64(size)) <= float64(size)*0.01 {
				sized = append(sized, t)
			}
		}
		if len(sized) > 0 {
			same = sized
		}
	}
	switch len(same) {
	case 0:
		return torrent{}, ErrNotFound
	case 1:
		return same[0], nil
	default:
		ids := make([]string, len(same))
		for i, t := range same {
			ids[i] = string(t.ID)
		}
		return torrent{}, fmt.Errorf("%w (id %s)", ErrAmbiguous, strings.Join(ids, ", "))
	}
}

// normName compares release names ignoring case and the separators that
// vary between a file name and its title ("La.Mia.Release" = "La Mia Release").
func normName(s string) string {
	return strings.Join(strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return unicode.IsSpace(r) || r == '.' || r == '_'
	}), " ")
}

var (
	hexHash    = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)
	magnetHash = regexp.MustCompile(`(?i)xt=urn:btih:([0-9a-f]{40})\b`)
)

// hashOf reads the hash from the API when the tracker exposes it (some
// UNIT3D versions return info_hash, others only inside the magnet link);
// otherwise it downloads the .torrent file and computes it.
func (c *Client) hashOf(ctx context.Context, t torrent) (string, error) {
	a := t.Attributes
	if h := strings.TrimSpace(a.InfoHash); hexHash.MatchString(h) {
		return strings.ToLower(h), nil
	}
	if m := magnetHash.FindStringSubmatch(a.MagnetLink); m != nil {
		return strings.ToLower(m[1]), nil
	}
	if a.DownloadLink == "" {
		return "", ErrNoHash
	}
	body, err := c.get(ctx, a.DownloadLink, false, 20<<20)
	if err != nil {
		return "", fmt.Errorf("download del .torrent: %w", err)
	}
	return InfoHash(body)
}

func (c *Client) getJSON(ctx context.Context, u string, out any) error {
	body, err := c.get(ctx, u, true, 32<<20)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("risposta del tracker non valida (è un sito UNIT3D?): %w", err)
	}
	return nil
}

func (c *Client) get(ctx context.Context, u string, auth bool, limit int64) ([]byte, error) {
	if err := c.wait(ctx); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	if auth {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Accept", "application/json")
	}
	req.Header.Set("User-Agent", "bookrr")
	resp, err := c.http.Do(req)
	if err != nil {
		// The URL may carry the RSS key: don't let it end up in the logs.
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return nil, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		wait := time.Minute
		if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s > 0 {
			wait = time.Duration(s) * time.Second
		}
		return nil, &RateLimitError{RetryAfter: wait}
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, fmt.Errorf("accesso negato dal tracker (HTTP %d): controlla BOOKRR_UNIT3D_API_KEY", resp.StatusCode)
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("il tracker ha risposto HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, limit))
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
