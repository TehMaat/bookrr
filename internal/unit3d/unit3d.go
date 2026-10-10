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
	"sort"
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
	ID          string
	Name        string
	Size        int64
	Hash        string
	DetailsLink string
	File        []byte // the .torrent file, when the tracker gives a download link
}

// Candidate is a tracker torrent that may be the one with a given name.
type Candidate struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	CreatedAt   string `json:"createdAt"`
	DetailsLink string `json:"detailsLink"`
	// Score is the share of words the two names have in common (0-100).
	Score int `json:"score"`
	// Match: same release written differently (title, formats, group).
	Match bool `json:"match"`
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
		DetailsLink  string  `json:"details_link"`
		CreatedAt    string  `json:"created_at"`
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

func (c *Client) search(ctx context.Context, query string) ([]torrent, error) {
	q := url.Values{"name": {query}, "perPage": {"100"}}
	var res struct {
		Data []torrent `json:"data"`
	}
	if err := c.getJSON(ctx, c.base+"/api/torrents/filter?"+q.Encode(), &res); err != nil {
		return nil, err
	}
	return res.Data, nil
}

// FindHash looks the name up on the tracker and returns the torrent with
// exactly that name (size, if known, picks between equal names). A name
// written differently ("Criminal.Minds.S19…DDP5.1…HDR10.H.265-MaTiTa" for
// "Criminal Minds S19 … DD+ 5.1 … HDR10+ H.265-MaTiTa") is accepted when
// title, season or year, resolution, source, HDR, codecs, languages and
// group all agree (see sameRelease).
func (c *Client) FindHash(ctx context.Context, name string, size int64) (Match, error) {
	queries := []string{name}
	if words := strings.Join(uniq(tokens(name)), " "); words != "" && words != name {
		queries = append(queries, words)
	}
	var t torrent
	var err error
	for _, q := range queries {
		var ts []torrent
		if ts, err = c.search(ctx, q); err != nil {
			return Match{}, err
		}
		if t, err = pick(ts, name, size); !errors.Is(err, ErrNotFound) {
			break
		}
		if len(ts) > 0 {
			// Something similar exists: let the user choose.
			err = fmt.Errorf("%w; %d simili: scegli quello giusto con \"Scegli dal tracker\"", err, len(ts))
			break
		}
	}
	if err != nil {
		return Match{}, err
	}
	return c.fetchHash(ctx, t)
}

// Candidates lists the tracker torrents that may be the one with this name,
// best first. With query empty it searches the name, then shorter forms
// of it (title and year, title) until something turns up.
func (c *Client) Candidates(ctx context.Context, name, query string, size int64) ([]Candidate, error) {
	queries := []string{query}
	if query == "" {
		queries = searchForms(name)
	}
	seen := map[string]bool{}
	var out []Candidate
	for _, q := range queries {
		ts, err := c.search(ctx, q)
		if err != nil {
			return nil, err
		}
		for _, t := range ts {
			if seen[string(t.ID)] {
				continue
			}
			seen[string(t.ID)] = true
			a := t.Attributes
			out = append(out, Candidate{
				ID: string(t.ID), Name: a.Name, Size: int64(a.Size), CreatedAt: a.CreatedAt,
				DetailsLink: a.DetailsLink, Score: score(name, a.Name),
				Match: normName(name) == normName(a.Name) || sameRelease(name, a.Name),
			})
		}
		if len(out) > 0 {
			break
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Match != out[j].Match {
			return out[i].Match
		}
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return sizeGap(out[i].Size, size) < sizeGap(out[j].Size, size)
	})
	if len(out) > 25 {
		out = out[:25]
	}
	return out, nil
}

// Fetch returns the hash (and .torrent file) of the tracker torrent with
// this id, as chosen by the user.
func (c *Client) Fetch(ctx context.Context, id string) (Match, error) {
	if _, err := strconv.ParseUint(id, 10, 64); err != nil {
		return Match{}, errors.New("id del torrent sul tracker non valido")
	}
	// UNIT3D answers with the torrent itself; be lenient with forks that
	// wrap it in "data".
	var res struct {
		torrent
		Data *torrent `json:"data"`
	}
	if err := c.getJSON(ctx, c.base+"/api/torrents/"+id, &res); err != nil {
		return Match{}, err
	}
	t := res.torrent
	if res.Data != nil {
		t = *res.Data
	}
	if t.ID == "" {
		t.ID = flexString(id)
	}
	return c.fetchHash(ctx, t)
}

func (c *Client) fetchHash(ctx context.Context, t torrent) (Match, error) {
	a := t.Attributes
	m := Match{ID: string(t.ID), Name: a.Name, Size: int64(a.Size), DetailsLink: a.DetailsLink}
	var err error
	m.Hash, m.File, err = c.hashOf(ctx, t)
	return m, err
}

func pick(ts []torrent, name string, size int64) (torrent, error) {
	want := normName(name)
	var same []torrent
	for _, t := range ts {
		if normName(t.Attributes.Name) == want || (t.Attributes.Folder != "" && normName(t.Attributes.Folder) == want) {
			same = append(same, t)
		}
	}
	if len(same) == 0 {
		// Same release written differently: same title and formats, same group.
		for _, t := range ts {
			if sameRelease(name, t.Attributes.Name) && sizeGap(int64(t.Attributes.Size), size) <= 0.05 {
				same = append(same, t)
			}
		}
	}
	if len(same) > 1 && size > 0 {
		var sized []torrent
		for _, t := range same {
			// A size typed by hand ("12,5 GB") is only approximate.
			if sizeGap(int64(t.Attributes.Size), size) <= 0.01 {
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

// tokens splits a release name into lowercase words.
func tokens(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

func uniq(ws []string) []string {
	seen := map[string]bool{}
	out := ws[:0:0]
	for _, w := range ws {
		if !seen[w] {
			seen[w] = true
			out = append(out, w)
		}
	}
	return out
}

func wordSet(s string) map[string]bool {
	set := map[string]bool{}
	for _, w := range tokens(s) {
		set[w] = true
	}
	return set
}

// score is the share of words the two names have in common (Jaccard, 0-100).
func score(a, b string) int {
	wa, wb := wordSet(a), wordSet(b)
	common := 0
	for w := range wa {
		if wb[w] {
			common++
		}
	}
	all := len(wa) + len(wb) - common
	if all == 0 {
		return 0
	}
	return common * 100 / all
}

// sizeGap is the relative difference between two sizes; an unknown size
// is no gap at all.
func sizeGap(a, b int64) float64 {
	if a <= 0 || b <= 0 {
		return 0
	}
	return math.Abs(float64(a-b)) / float64(b)
}

var yearRe = regexp.MustCompile(`^(19|20)\d\d$`)

// searchForms is the name, then its words, then title and year, then the
// title alone: "AD.ASTRA.2019.REMUX…" → "ad astra 2019", "ad astra".
func searchForms(name string) []string {
	ws := uniq(tokens(name))
	forms := []string{name}
	add := func(f string) {
		if f != "" && f != forms[len(forms)-1] {
			forms = append(forms, f)
		}
	}
	add(strings.Join(ws, " "))
	year := -1
	for i, w := range ws {
		if i > 0 && yearRe.MatchString(w) {
			year = i
			break
		}
	}
	if year > 0 {
		add(strings.Join(ws[:year+1], " "))
		add(strings.Join(ws[:year], " "))
	} else if len(ws) > 3 {
		add(strings.Join(ws[:3], " "))
	}
	return forms
}

var (
	hexHash    = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)
	magnetHash = regexp.MustCompile(`(?i)xt=urn:btih:([0-9a-f]{40})\b`)
)

// hashOf downloads the .torrent file, to keep it and compute the hash from
// it. Without a download link (or if the download fails) it falls back to
// the hash some UNIT3D versions return as info_hash or in the magnet link.
func (c *Client) hashOf(ctx context.Context, t torrent) (string, []byte, error) {
	a := t.Attributes
	known := ""
	if h := strings.TrimSpace(a.InfoHash); hexHash.MatchString(h) {
		known = strings.ToLower(h)
	} else if m := magnetHash.FindStringSubmatch(a.MagnetLink); m != nil {
		known = strings.ToLower(m[1])
	}
	if a.DownloadLink == "" {
		if known == "" {
			return "", nil, ErrNoHash
		}
		return known, nil, nil
	}
	body, err := c.get(ctx, a.DownloadLink, false, 20<<20)
	var hash string
	if err == nil {
		hash, err = InfoHash(body)
	}
	if err != nil {
		var rl *RateLimitError
		if known != "" && !errors.As(err, &rl) {
			return known, nil, nil
		}
		return "", nil, fmt.Errorf("download del .torrent: %w", err)
	}
	return hash, body, nil
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
