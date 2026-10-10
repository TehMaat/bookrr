// Package qbit is a minimal qBittorrent Web API (v2) client.
package qbit

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

type Torrent struct {
	Hash        string  `json:"hash"`
	InfohashV1  string  `json:"infohash_v1"`
	Name        string  `json:"name"`
	Size        int64   `json:"size"`
	TotalSize   int64   `json:"total_size"`
	Tags        string  `json:"tags"`
	Category    string  `json:"category"`
	SavePath    string  `json:"save_path"`
	ContentPath string  `json:"content_path"`
	State       string  `json:"state"`
	Progress    float64 `json:"progress"`
	Ratio       float64 `json:"ratio"`
	AddedOn     int64   `json:"added_on"`
	Tracker     string  `json:"tracker"`
}

type Client struct {
	base     string
	username string
	password string
	http     *http.Client
	loggedIn bool
}

func New(baseURL, username, password string, skipTLSVerify bool) *Client {
	jar, _ := cookiejar.New(nil)
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if skipTLSVerify {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return &Client{
		base:     strings.TrimRight(baseURL, "/"),
		username: username,
		password: password,
		http:     &http.Client{Jar: jar, Transport: tr, Timeout: 60 * time.Second},
	}
}

var ErrAuth = errors.New("autenticazione fallita (utente/password errati?)")

func (c *Client) login(ctx context.Context) error {
	form := url.Values{"username": {c.username}, "password": {c.password}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/api/v2/auth/login", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// qBittorrent's CSRF protection wants a Referer/Origin matching the host.
	req.Header.Set("Referer", c.base)
	req.Header.Set("Origin", c.base)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	switch {
	case resp.StatusCode == http.StatusForbidden:
		return errors.New("IP bannato da qBittorrent per troppi tentativi falliti")
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("login: HTTP %d", resp.StatusCode)
	case strings.TrimSpace(string(body)) == "Fails.":
		return ErrAuth
	}
	c.loggedIn = true
	return nil
}

// get performs an authenticated GET, logging in when needed.
func (c *Client) get(ctx context.Context, path string, out any) error {
	if !c.loggedIn && c.username != "" {
		if err := c.login(ctx); err != nil {
			return err
		}
	}
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Referer", c.base)
		resp, err := c.http.Do(req)
		if err != nil {
			return err
		}
		if resp.StatusCode == http.StatusForbidden && attempt == 0 {
			resp.Body.Close()
			if c.username == "" {
				return errors.New("qBittorrent richiede l'autenticazione: imposta utente e password")
			}
			if err := c.login(ctx); err != nil {
				return err
			}
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("%s: HTTP %d", path, resp.StatusCode)
		}
		if s, ok := out.(*string); ok {
			b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			*s = strings.TrimSpace(string(b))
			return err
		}
		return json.NewDecoder(resp.Body).Decode(out)
	}
}

func (c *Client) Version(ctx context.Context) (string, error) {
	var v string
	err := c.get(ctx, "/api/v2/app/version", &v)
	return v, err
}

func (c *Client) Torrents(ctx context.Context) ([]Torrent, error) {
	var ts []Torrent
	if err := c.get(ctx, "/api/v2/torrents/info", &ts); err != nil {
		return nil, err
	}
	return ts, nil
}

// post performs an authenticated form POST, logging in when needed.
func (c *Client) post(ctx context.Context, path string, form url.Values) error {
	if !c.loggedIn && c.username != "" {
		if err := c.login(ctx); err != nil {
			return err
		}
	}
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, strings.NewReader(form.Encode()))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Referer", c.base)
		req.Header.Set("Origin", c.base)
		resp, err := c.http.Do(req)
		if err != nil {
			return err
		}
		io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		resp.Body.Close()
		if resp.StatusCode == http.StatusForbidden && attempt == 0 {
			if c.username == "" {
				return errors.New("qBittorrent richiede l'autenticazione: imposta utente e password")
			}
			if err := c.login(ctx); err != nil {
				return err
			}
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("%s: HTTP %d", path, resp.StatusCode)
		}
		return nil
	}
}

// DeleteTorrent removes a torrent from the client, optionally deleting its
// downloaded files too. qBittorrent succeeds even if the hash is unknown.
func (c *Client) DeleteTorrent(ctx context.Context, hash string, deleteFiles bool) error {
	return c.post(ctx, "/api/v2/torrents/delete", url.Values{
		"hashes":      {hash},
		"deleteFiles": {fmt.Sprint(deleteFiles)},
	})
}
