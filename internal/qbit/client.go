// Package qbit is a minimal qBittorrent Web API (v2) client.
package qbit

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
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

var ErrAuth = errors.New("authentication failed (wrong username/password?)")

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
	// Older qBittorrent answers 200 "Ok." / 200 "Fails."; newer releases
	// answer 204 No Content on success and 401 on wrong credentials.
	switch {
	case resp.StatusCode == http.StatusForbidden:
		return errors.New("IP banned by qBittorrent after too many failed attempts")
	case resp.StatusCode == http.StatusUnauthorized:
		return ErrAuth
	case !success(resp.StatusCode):
		return fmt.Errorf("login: HTTP %d", resp.StatusCode)
	case strings.TrimSpace(string(body)) == "Fails.":
		return ErrAuth
	}
	c.loggedIn = true
	return nil
}

func success(status int) bool { return status >= 200 && status < 300 }

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
				return errors.New("qBittorrent requires authentication: set username and password")
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
	_, err := c.postBody(ctx, path, func() (io.Reader, string, error) {
		return strings.NewReader(form.Encode()), "application/x-www-form-urlencoded", nil
	})
	return err
}

// postBody performs an authenticated POST of the body that build makes
// (again for the retry after a login) and returns the start of the answer.
func (c *Client) postBody(ctx context.Context, path string, build func() (io.Reader, string, error)) (string, error) {
	if !c.loggedIn && c.username != "" {
		if err := c.login(ctx); err != nil {
			return "", err
		}
	}
	for attempt := 0; ; attempt++ {
		body, contentType, err := build()
		if err != nil {
			return "", err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, body)
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", contentType)
		req.Header.Set("Referer", c.base)
		req.Header.Set("Origin", c.base)
		resp, err := c.http.Do(req)
		if err != nil {
			return "", err
		}
		answer, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		resp.Body.Close()
		if resp.StatusCode == http.StatusForbidden && attempt == 0 {
			if c.username == "" {
				return "", errors.New("qBittorrent requires authentication: set username and password")
			}
			if err := c.login(ctx); err != nil {
				return "", err
			}
			continue
		}
		if !success(resp.StatusCode) {
			return "", &statusError{path: path, code: resp.StatusCode}
		}
		return strings.TrimSpace(string(answer)), nil
	}
}

// AddOptions are where and how a torrent is added: the empty ones leave
// the defaults of the client.
type AddOptions struct {
	Category string
	// Comma separated.
	Tags     string
	SavePath string
}

// AddTorrent adds a .torrent file to the client, started.
func (c *Client) AddTorrent(ctx context.Context, filename string, data []byte, o AddOptions) error {
	fields := [][2]string{{"category", o.Category}, {"tags", o.Tags}, {"savepath", o.SavePath}}
	if o.SavePath != "" {
		// With automatic management the client would choose the folder.
		fields = append(fields, [2]string{"autoTMM", "false"})
	}
	answer, err := c.postBody(ctx, "/api/v2/torrents/add", func() (io.Reader, string, error) {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, err := mw.CreateFormFile("torrents", filename)
		if err != nil {
			return nil, "", err
		}
		if _, err := fw.Write(data); err != nil {
			return nil, "", err
		}
		for _, f := range fields {
			if f[1] == "" {
				continue
			}
			if err := mw.WriteField(f[0], f[1]); err != nil {
				return nil, "", err
			}
		}
		if err := mw.Close(); err != nil {
			return nil, "", err
		}
		return &buf, mw.FormDataContentType(), nil
	})
	var se *statusError
	errors.As(err, &se)
	switch {
	case se != nil && se.code == http.StatusUnsupportedMediaType:
		return errors.New("qBittorrent says the .torrent file is not valid")
	case se != nil && se.code == http.StatusConflict:
		return errors.New("qBittorrent refused the torrent: it is already on the client")
	case err != nil:
		return err
	case answer == "Fails.":
		// Older versions say so when the torrent is already there, or invalid.
		return errors.New("qBittorrent refused the torrent: already on the client, or not valid")
	}
	return nil
}

// statusError is an answer of the client that is not a success.
type statusError struct {
	path string
	code int
}

func (e *statusError) Error() string { return fmt.Sprintf("%s: HTTP %d", e.path, e.code) }

// DeleteTorrent removes a torrent from the client, optionally deleting its
// downloaded files too. qBittorrent succeeds even if the hash is unknown.
func (c *Client) DeleteTorrent(ctx context.Context, hash string, deleteFiles bool) error {
	return c.post(ctx, "/api/v2/torrents/delete", url.Values{
		"hashes":      {hash},
		"deleteFiles": {fmt.Sprint(deleteFiles)},
	})
}

// RemoveTags removes the tags from the torrents. qBittorrent succeeds even
// for unknown hashes and for tags a torrent does not have.
func (c *Client) RemoveTags(ctx context.Context, hashes, tags []string) error {
	return c.post(ctx, "/api/v2/torrents/removeTags", url.Values{
		"hashes": {strings.Join(hashes, "|")},
		"tags":   {strings.Join(tags, ",")},
	})
}
