package unit3d

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Upload is one of the user's own torrents on the tracker, as listed by
// /api/torrents/filter?uploader=…
type Upload struct {
	ID              int64
	Name            string
	Category        string
	Type            string // WEB-DL, Remux, Encode…
	TypeID          int64
	Resolution      string
	ResolutionID    int64
	Size            int64
	Seeders         int64
	Leechers        int64
	TimesCompleted  int64
	TmdbID          int64
	ImdbID          int64
	TvdbID          int64
	PersonalRelease bool
	CreatedAt       string // RFC 3339 when the tracker's format is understood
	DetailsLink     string

	t torrent // what is needed to compute the hash
}

type uploadAttrs struct {
	Category        string   `json:"category"`
	Type            string   `json:"type"`
	TypeID          flexInt  `json:"type_id"`
	Resolution      string   `json:"resolution"`
	ResolutionID    flexInt  `json:"resolution_id"`
	Seeders         flexInt  `json:"seeders"`
	Leechers        flexInt  `json:"leechers"`
	TimesCompleted  flexInt  `json:"times_completed"`
	TmdbID          flexInt  `json:"tmdb_id"`
	ImdbID          flexInt  `json:"imdb_id"`
	TvdbID          flexInt  `json:"tvdb_id"`
	PersonalRelease flexBool `json:"personal_release"`
}

// flexInt accepts a number, a numeric string ("12", "tt0111161"), null or "".
type flexInt int64

func (f *flexInt) UnmarshalJSON(b []byte) error {
	var n json.Number
	if err := json.Unmarshal(b, &n); err == nil {
		if v, err := n.Int64(); err == nil {
			*f = flexInt(v)
		} else if fl, err := n.Float64(); err == nil {
			*f = flexInt(fl)
		}
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		s = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(s)), "tt")
		v, _ := strconv.ParseInt(s, 10, 64)
		*f = flexInt(v)
		return nil
	}
	*f = 0 // null, objects: unknown
	return nil
}

// flexBool accepts true/false, 0/1 and "0"/"1"/"true".
type flexBool bool

func (f *flexBool) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.ToLower(string(bytes.TrimSpace(b))), `"`)
	*f = flexBool(s == "true" || (s != "" && s != "0" && s != "false" && s != "null"))
	return nil
}

// Username returns the name of the user the API key belongs to.
func (c *Client) Username(ctx context.Context) (string, error) {
	var res struct {
		Username string `json:"username"`
		Data     *struct {
			Username   string `json:"username"`
			Attributes *struct {
				Username string `json:"username"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := c.getJSON(ctx, c.base+"/api/user", &res); err != nil {
		return "", err
	}
	name := res.Username
	if name == "" && res.Data != nil {
		name = res.Data.Username
		if name == "" && res.Data.Attributes != nil {
			name = res.Data.Attributes.Username
		}
	}
	if name = strings.TrimSpace(name); name == "" {
		return "", errors.New("il tracker non ha indicato il nome utente (/api/user)")
	}
	return name, nil
}

// maxUploadPages stops a tracker that keeps answering with a next page.
const maxUploadPages = 500

// Uploads lists every torrent uploaded by username, anonymous ones included
// (the tracker shows them because the request comes from their uploader),
// following the pages of the answer.
func (c *Client) Uploads(ctx context.Context, username string) ([]Upload, error) {
	// The tracker caches /filter answers for a few minutes by URL, without
	// the token, for every user: a parameter it ignores, unique to this
	// user, keeps us from getting someone else's answer, which lacks the
	// anonymous uploads.
	base := url.Values{"uploader": {username}, "perPage": {"100"}, "bookrr_user": {username}}
	q := base
	var out []Upload
	seen := map[string]bool{}
	for page := 0; ; page++ {
		if page >= maxUploadPages {
			return nil, fmt.Errorf("troppe pagine di upload (oltre %d)", maxUploadPages)
		}
		key := q.Encode()
		if seen[key] {
			return nil, errors.New("il tracker ripete la stessa pagina di upload")
		}
		seen[key] = true
		var res struct {
			Data []json.RawMessage `json:"data"`
			// Laravel puts the pagination either in links/meta or at the top.
			Links struct {
				Next *string `json:"next"`
			} `json:"links"`
			Meta struct {
				NextCursor *string `json:"next_cursor"`
			} `json:"meta"`
			NextCursor  *string `json:"next_cursor"`
			NextPageURL *string `json:"next_page_url"`
		}
		if err := c.getJSON(ctx, c.base+"/api/torrents/filter?"+key, &res); err != nil {
			return nil, err
		}
		for _, raw := range res.Data {
			u, err := parseUpload(raw)
			if err != nil {
				return nil, err
			}
			out = append(out, u)
		}
		next := nextPage(base, deref(res.Meta.NextCursor, res.NextCursor), deref(res.Links.Next, res.NextPageURL))
		if next == nil || len(res.Data) == 0 {
			return out, nil
		}
		q = next
	}
}

func deref(ps ...*string) string {
	for _, p := range ps {
		if p != nil && *p != "" {
			return *p
		}
	}
	return ""
}

// nextPage is the query of the next page: the cursor if the tracker gives
// one, otherwise the parameters of its "next" link. The link itself is not
// followed, so the API key only goes to the configured tracker, and our
// filters are kept even if the link drops them.
func nextPage(base url.Values, cursor, link string) url.Values {
	q := url.Values{}
	for k, v := range base {
		q[k] = v
	}
	if cursor != "" {
		q.Set("cursor", cursor)
		return q
	}
	if link == "" {
		return nil
	}
	u, err := url.Parse(link)
	if err != nil {
		return nil
	}
	for k, v := range u.Query() {
		if _, ours := base[k]; !ours {
			q[k] = v
		}
	}
	return q
}

func parseUpload(raw json.RawMessage) (Upload, error) {
	var t torrent
	if err := json.Unmarshal(raw, &t); err != nil {
		return Upload{}, fmt.Errorf("upload non valido: %w", err)
	}
	var w struct {
		Attributes uploadAttrs `json:"attributes"`
	}
	if err := json.Unmarshal(raw, &w); err != nil {
		return Upload{}, fmt.Errorf("upload non valido: %w", err)
	}
	id, err := strconv.ParseInt(strings.TrimSpace(string(t.ID)), 10, 64)
	if err != nil || id <= 0 {
		return Upload{}, fmt.Errorf("id dell'upload non valido: %q", t.ID)
	}
	a, x := t.Attributes, w.Attributes
	return Upload{
		ID: id, Name: a.Name, Category: x.Category, Type: x.Type, TypeID: int64(x.TypeID),
		Resolution: x.Resolution, ResolutionID: int64(x.ResolutionID), Size: int64(a.Size),
		Seeders: int64(x.Seeders), Leechers: int64(x.Leechers), TimesCompleted: int64(x.TimesCompleted),
		TmdbID: int64(x.TmdbID), ImdbID: int64(x.ImdbID), TvdbID: int64(x.TvdbID),
		PersonalRelease: bool(x.PersonalRelease), CreatedAt: normTime(a.CreatedAt), DetailsLink: a.DetailsLink,
		t: t,
	}, nil
}

// normTime turns the tracker's dates ("2024-05-01T10:00:00.000000Z",
// "2024-05-01 10:00:00") into RFC 3339; anything else is kept as is.
func normTime(s string) string {
	s = strings.TrimSpace(s)
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC().Format(time.RFC3339)
		}
	}
	return s
}

// UploadHash downloads the .torrent file of an upload and computes its
// info hash (falling back to the hash the API may give).
func (c *Client) UploadHash(ctx context.Context, u Upload) (string, []byte, error) {
	return c.hashOf(ctx, u.t)
}
