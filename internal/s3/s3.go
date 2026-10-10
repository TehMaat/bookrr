// Package s3 is a minimal, read-only client for S3-compatible object storage
// (Scaleway, AWS, MinIO, …). It can only list a bucket: bookrr never writes
// or deletes anything there.
package s3

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Config tells where the bucket is and how to sign the requests.
type Config struct {
	Endpoint  string // e.g. https://s3.fr-par.scw.cloud
	Region    string // e.g. fr-par
	Bucket    string
	AccessKey string
	SecretKey string
}

// ScalewayEndpoint is the S3 endpoint of a Scaleway region (fr-par, nl-ams, pl-waw).
func ScalewayEndpoint(region string) string { return "https://s3." + region + ".scw.cloud" }

type Object struct {
	Key          string
	Size         int64
	LastModified time.Time
}

type Client struct {
	cfg  Config
	base *url.URL
	http *http.Client
	now  func() time.Time
}

func New(cfg Config) (*Client, error) {
	cfg.Endpoint = strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/")
	u, err := url.Parse(cfg.Endpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("endpoint %q non valido (es. https://s3.fr-par.scw.cloud)", cfg.Endpoint)
	}
	if cfg.Bucket == "" {
		return nil, errors.New("bucket non indicato")
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	return &Client{cfg: cfg, base: u, http: &http.Client{Timeout: 60 * time.Second}, now: time.Now}, nil
}

// List returns every object whose key starts with prefix, following the
// pagination of ListObjectsV2.
func (c *Client) List(ctx context.Context, prefix string) ([]Object, error) {
	var out []Object
	token := ""
	for {
		page, err := c.ListPage(ctx, prefix, token, 1000)
		if err != nil {
			return nil, err
		}
		out = append(out, page.Objects...)
		if !page.Truncated || page.NextToken == "" {
			return out, nil
		}
		token = page.NextToken
	}
}

type Page struct {
	Objects   []Object
	Truncated bool
	NextToken string
}

// ListPage returns one page (at most max objects) of the bucket listing.
func (c *Client) ListPage(ctx context.Context, prefix, token string, max int) (Page, error) {
	q := url.Values{"list-type": {"2"}, "max-keys": {fmt.Sprint(max)}}
	if prefix != "" {
		q.Set("prefix", prefix)
	}
	if token != "" {
		q.Set("continuation-token", token)
	}
	u := *c.base
	// Path-style addressing (endpoint/bucket) works with Scaleway, MinIO and
	// most S3-compatible services, whatever the bucket name.
	u.Path = strings.TrimRight(u.Path, "/") + "/" + c.cfg.Bucket
	u.RawPath = ""
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return Page{}, err
	}
	sign(req, c.cfg.AccessKey, c.cfg.SecretKey, c.cfg.Region, c.now())
	resp, err := c.http.Do(req)
	if err != nil {
		return Page{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return Page{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return Page{}, responseError(resp.StatusCode, body)
	}
	var res struct {
		IsTruncated           bool
		NextContinuationToken string
		Contents              []struct {
			Key          string
			Size         int64
			LastModified time.Time
		}
	}
	if err := xml.Unmarshal(body, &res); err != nil {
		return Page{}, fmt.Errorf("risposta non valida: %w", err)
	}
	p := Page{Truncated: res.IsTruncated, NextToken: res.NextContinuationToken, Objects: make([]Object, 0, len(res.Contents))}
	for _, o := range res.Contents {
		p.Objects = append(p.Objects, Object{Key: o.Key, Size: o.Size, LastModified: o.LastModified})
	}
	return p, nil
}

func responseError(status int, body []byte) error {
	var e struct {
		Code    string
		Message string
	}
	xml.Unmarshal(body, &e)
	switch e.Code {
	case "NoSuchBucket":
		return errors.New("il bucket non esiste (controlla nome e regione)")
	case "InvalidAccessKeyId":
		return errors.New("access key non valida")
	case "SignatureDoesNotMatch":
		return errors.New("secret key errata")
	case "AccessDenied":
		return errors.New("accesso negato: la chiave non può leggere questo bucket")
	}
	msg := fmt.Sprintf("HTTP %d", status)
	if e.Code != "" {
		msg += " " + e.Code
	}
	if e.Message != "" {
		msg += ": " + e.Message
	}
	return errors.New(msg)
}

const emptySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// sign adds an AWS Signature Version 4 to a request without a body. It
// rewrites the query string in canonical form so the URL that is sent is
// exactly the one that was signed.
func sign(req *http.Request, accessKey, secretKey, region string, t time.Time) {
	t = t.UTC()
	amzDate := t.Format("20060102T150405Z")
	day := t.Format("20060102")

	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("x-amz-content-sha256", emptySHA256)

	query := canonicalQuery(req.URL.Query())
	req.URL.RawQuery = query
	path := req.URL.EscapedPath()
	if path == "" {
		path = "/"
	}
	canonical := strings.Join([]string{
		req.Method,
		path,
		query,
		"host:" + req.URL.Host + "\n" +
			"x-amz-content-sha256:" + emptySHA256 + "\n" +
			"x-amz-date:" + amzDate + "\n",
		"host;x-amz-content-sha256;x-amz-date",
		emptySHA256,
	}, "\n")
	scope := day + "/" + region + "/s3/aws4_request"
	sum := sha256.Sum256([]byte(canonical))
	toSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hex.EncodeToString(sum[:])

	key := hmacSHA256([]byte("AWS4"+secretKey), day)
	key = hmacSHA256(key, region)
	key = hmacSHA256(key, "s3")
	key = hmacSHA256(key, "aws4_request")
	signature := hex.EncodeToString(hmacSHA256(key, toSign))

	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+accessKey+"/"+scope+
		", SignedHeaders=host;x-amz-content-sha256;x-amz-date, Signature="+signature)
}

func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

// canonicalQuery sorts the parameters and encodes them as SigV4 wants
// (RFC 3986: spaces as %20, only A-Z a-z 0-9 - _ . ~ left as they are).
func canonicalQuery(q url.Values) string {
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := []string{}
	for _, k := range keys {
		vs := append([]string(nil), q[k]...)
		sort.Strings(vs)
		for _, v := range vs {
			parts = append(parts, uriEncode(k)+"="+uriEncode(v))
		}
	}
	return strings.Join(parts, "&")
}

func uriEncode(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if 'A' <= c && c <= 'Z' || 'a' <= c && c <= 'z' || '0' <= c && c <= '9' || c == '-' || c == '_' || c == '.' || c == '~' {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}
