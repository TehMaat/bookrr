// Package wayfinderr talks to a Wayfinderr (github.com/TehMaat/wayfinderr),
// the app that rips and uploads what a client downloads: bookrr sends it the
// .torrent files of the plan once they are downloaded, as the source
// .torrent of their film, plans the pair of the two downloads of a merge,
// and drops its own copy once the film is released.
package wayfinderr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Origin is how bookrr names itself to Wayfinderr.
const Origin = "bookrr"

// maxTorrentFile is the largest .torrent file read back.
const maxTorrentFile = 10 << 20

type Client struct {
	base string
	key  string
	http *http.Client
}

// New is a Wayfinderr at base (the address its page is opened at, or its
// backend), with its API key.
func New(base, key string) *Client {
	return &Client{
		base: strings.TrimRight(strings.TrimSpace(base), "/"),
		key:  strings.TrimSpace(key),
		http: &http.Client{Timeout: 30 * time.Second},
	}
}

// ErrUnauthorized is a refused API key.
var ErrUnauthorized = errors.New("Wayfinderr refused the API key: copy it again from its account menu (API key); it takes one from version 0.25.0")

// apiError is the error Wayfinderr answered with.
type apiError struct {
	Status  int
	Message string
}

func (e *apiError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("Wayfinderr answered HTTP %d", e.Status)
	}
	return fmt.Sprintf("Wayfinderr: %s (HTTP %d)", e.Message, e.Status)
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body []byte, contentType string) (*http.Response, error) {
	u := c.base + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Api-Key", c.key)
	req.Header.Set("Accept", "application/json")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Wayfinderr not reachable: %w", err)
	}
	return resp, nil
}

// failure turns an unexpected answer into an error, and closes it.
func failure(resp *http.Response) error {
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return ErrUnauthorized
	}
	var body struct {
		Error string `json:"error"`
	}
	json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&body)
	return &apiError{Status: resp.StatusCode, Message: body.Error}
}

// Ping checks the address and the key: it returns the user the key logs in as.
func (c *Client) Ping(ctx context.Context) (string, error) {
	resp, err := c.do(ctx, http.MethodGet, "/api/auth/status", nil, nil, "")
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", failure(resp)
	}
	defer resp.Body.Close()
	var st struct {
		Configured *bool   `json:"configured"`
		Username   *string `json:"username"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&st); err != nil || st.Configured == nil {
		return "", errors.New("this address does not answer like Wayfinderr")
	}
	if !*st.Configured {
		return "", errors.New("Wayfinderr has no account yet: create it first")
	}
	if st.Username == nil {
		return "", ErrUnauthorized
	}
	// Status answers to anyone: the key is checked on a route that needs it.
	resp, err = c.do(ctx, http.MethodGet, "/api/source-torrents", nil, nil, "")
	if err != nil {
		return "", err
	}
	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return "", errors.New("this Wayfinderr is too old: it takes .torrent files from version 0.25.0")
	}
	if resp.StatusCode != http.StatusOK {
		return "", failure(resp)
	}
	resp.Body.Close()
	return *st.Username, nil
}

// SourceTorrent is a .torrent to send as the source .torrent of a film.
type SourceTorrent struct {
	// Its file name.
	FileName string
	Data     []byte
	TmdbID   int64
	// What it is for ("video", "audio ITA"), "" if nothing.
	Label string
}

// AddSourceTorrent sends a source .torrent: Wayfinderr keeps it and copies it
// into its upload as soon as the rip of its download is done. Sending it
// again is harmless.
func (c *Client) AddSourceTorrent(ctx context.Context, t SourceTorrent) error {
	q := url.Values{"origin": {Origin}}
	if t.FileName != "" {
		q.Set("name", t.FileName)
	}
	if t.TmdbID > 0 {
		q.Set("tmdbId", strconv.FormatInt(t.TmdbID, 10))
	}
	if t.Label != "" {
		q.Set("label", t.Label)
	}
	resp, err := c.do(ctx, http.MethodPost, "/api/source-torrents", q, t.Data, "application/x-bittorrent")
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return failure(resp)
	}
	resp.Body.Close()
	return nil
}

// HasSourceTorrent reports whether Wayfinderr keeps the .torrent with this
// v1 info hash.
func (c *Client) HasSourceTorrent(ctx context.Context, hash string) (bool, error) {
	resp, err := c.do(ctx, http.MethodGet, "/api/source-torrents/"+url.PathEscape(strings.ToLower(hash)), nil, nil, "")
	if err != nil {
		return false, err
	}
	switch resp.StatusCode {
	case http.StatusOK:
		resp.Body.Close()
		return true, nil
	case http.StatusNotFound:
		resp.Body.Close()
		return false, nil
	}
	return false, failure(resp)
}

// SourceTorrentFile returns the .torrent with this v1 info hash.
func (c *Client) SourceTorrentFile(ctx context.Context, hash string) ([]byte, error) {
	resp, err := c.do(ctx, http.MethodGet, "/api/source-torrents/"+url.PathEscape(strings.ToLower(hash))+"/file", nil, nil, "")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, errors.New("Wayfinderr no longer has this .torrent")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, failure(resp)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxTorrentFile+1))
	if err != nil {
		return nil, fmt.Errorf("reading the .torrent from Wayfinderr: %w", err)
	}
	if len(data) == 0 || len(data) > maxTorrentFile {
		return nil, errors.New("Wayfinderr sent an empty or too large .torrent")
	}
	return data, nil
}

// MergeSide is a download of a pair to merge: its torrent's v1 info hash
// and its name (the torrent's folder, or its file for a single-file torrent).
type MergeSide struct {
	InfoHash string `json:"infoHash,omitempty"`
	Name     string `json:"name,omitempty"`
	// What it is for ("video", "audio ITA"), "" if nothing.
	Label string `json:"label,omitempty"`
}

// MergePlan is the pair of two downloads to merge, planned before their
// files arrive: Wayfinderr holds each file as it arrives, and pairs and
// analyzes them once both are there (the merge still waits for a confirmation).
type MergePlan struct {
	Files  [2]MergeSide
	TmdbID int64
}

// Statuses of a planned pair on Wayfinderr.
const (
	// A file is still to come.
	MergeWaiting = "WAITING"
	// Both arrived: the pair is PairID.
	MergePaired = "PAIRED"
	// Both arrived, but the pair could not be created: Message says why.
	MergeError = "ERROR"
)

// PlannedPair is a planned pair as Wayfinderr has it.
type PlannedPair struct {
	ID      string  `json:"id"`
	Status  string  `json:"status"`
	PairID  *string `json:"pairId"`
	Message *string `json:"message"`
}

// ErrMergeUnsupported is a Wayfinderr without planned pairs.
var ErrMergeUnsupported = errors.New("this Wayfinderr is too old: it plans the pairs to merge from version 0.29.0")

// ErrPlanNotFound is a planned pair Wayfinderr no longer has.
var ErrPlanNotFound = errors.New("Wayfinderr no longer has this planned pair")

func readPlan(resp *http.Response) (PlannedPair, error) {
	defer resp.Body.Close()
	var p PlannedPair
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&p); err != nil || p.ID == "" {
		return PlannedPair{}, errors.New("Wayfinderr answered with no planned pair")
	}
	return p, nil
}

// PlanMerge plans the pair of two downloads; created is false when
// Wayfinderr had the same pair planned already (planning again is harmless).
func (c *Client) PlanMerge(ctx context.Context, p MergePlan) (plan PlannedPair, created bool, err error) {
	body := map[string]any{"files": p.Files, "origin": Origin}
	if p.TmdbID > 0 {
		body["tmdbId"] = p.TmdbID
	}
	data, err := json.Marshal(body)
	if err != nil {
		return PlannedPair{}, false, err
	}
	resp, err := c.do(ctx, http.MethodPost, "/api/merge/plans", nil, data, "application/json")
	if err != nil {
		return PlannedPair{}, false, err
	}
	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated:
		plan, err := readPlan(resp)
		return plan, resp.StatusCode == http.StatusCreated, err
	case http.StatusNotFound:
		resp.Body.Close()
		return PlannedPair{}, false, ErrMergeUnsupported
	}
	return PlannedPair{}, false, failure(resp)
}

// MergePlanOf returns a planned pair: ErrPlanNotFound once Wayfinderr no
// longer has it.
func (c *Client) MergePlanOf(ctx context.Context, id string) (PlannedPair, error) {
	resp, err := c.do(ctx, http.MethodGet, "/api/merge/plans/"+url.PathEscape(id), nil, nil, "")
	if err != nil {
		return PlannedPair{}, err
	}
	switch resp.StatusCode {
	case http.StatusOK:
		return readPlan(resp)
	case http.StatusNotFound:
		resp.Body.Close()
		return PlannedPair{}, ErrPlanNotFound
	}
	return PlannedPair{}, failure(resp)
}

// PairMerge creates again the pair of a planned pair that ended in ERROR,
// once both files are there.
func (c *Client) PairMerge(ctx context.Context, id string) (PlannedPair, error) {
	resp, err := c.do(ctx, http.MethodPost, "/api/merge/plans/"+url.PathEscape(id)+"/pair", nil, nil, "")
	if err != nil {
		return PlannedPair{}, err
	}
	switch resp.StatusCode {
	case http.StatusOK:
		return readPlan(resp)
	case http.StatusNotFound:
		resp.Body.Close()
		return PlannedPair{}, ErrPlanNotFound
	}
	return PlannedPair{}, failure(resp)
}

// DeleteMergePlan forgets a planned pair: the files Wayfinderr held for it
// stay held, to pair, upload or delete there. One it no longer has is fine.
func (c *Client) DeleteMergePlan(ctx context.Context, id string) error {
	resp, err := c.do(ctx, http.MethodDelete, "/api/merge/plans/"+url.PathEscape(id), nil, nil, "")
	if err != nil {
		return err
	}
	switch resp.StatusCode {
	case http.StatusOK, http.StatusNoContent, http.StatusNotFound:
		resp.Body.Close()
		return nil
	}
	return failure(resp)
}

// SameWayfinderr reports whether two addresses are the same Wayfinderr.
func SameWayfinderr(a, b string) bool {
	norm := func(s string) string { return strings.ToLower(strings.TrimRight(strings.TrimSpace(s), "/")) }
	return a != "" && norm(a) == norm(b)
}
