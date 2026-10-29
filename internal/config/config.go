// Package config reads bookrr settings from environment variables.
package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Listen       string
	DataDir      string
	SyncInterval time.Duration
	// How often the S3 buckets of the cloud archives are listed.
	S3ScanInterval time.Duration
	ReleaseTag     string
	WebhookToken   string
	AuthUser       string
	AuthPassword   string
	// UNIT3D tracker used to find the info hash of torrents added without one.
	Unit3DURL    string
	Unit3DAPIKey string
	// How often the user's uploads on the UNIT3D tracker are read.
	Unit3DReleasesInterval time.Duration
	// The Movie Database key, for the seasons of the series and the films
	// of the sagas uploaded.
	TMDBAPIKey string
	// MDBList key, for the lists of films the user follows beyond the sagas.
	MDBListAPIKey string
	// Time of the nightly check of the missing seasons and films, in minutes after
	// midnight (local time).
	MissingCheckAt int
	// Prowlarr, for the best resolution that exists of the films uploaded,
	// and how often each film is searched again.
	ProwlarrURL     string
	ProwlarrAPIKey  string
	ProwlarrRecheck time.Duration
}

func (c Config) DBPath() string { return filepath.Join(c.DataDir, "bookrr.db") }

func Load() (Config, error) {
	c := Config{
		Listen:         env("BOOKRR_LISTEN", ":8080"),
		DataDir:        env("BOOKRR_DATA_DIR", "/data"),
		ReleaseTag:     env("BOOKRR_RELEASE_TAG", "Personal Release"),
		WebhookToken:   os.Getenv("BOOKRR_WEBHOOK_TOKEN"),
		AuthUser:       os.Getenv("BOOKRR_AUTH_USER"),
		AuthPassword:   os.Getenv("BOOKRR_AUTH_PASSWORD"),
		Unit3DURL:      strings.TrimRight(strings.TrimSpace(os.Getenv("BOOKRR_UNIT3D_URL")), "/"),
		Unit3DAPIKey:   strings.TrimSpace(os.Getenv("BOOKRR_UNIT3D_API_KEY")),
		TMDBAPIKey:     strings.TrimSpace(os.Getenv("BOOKRR_TMDB_API_KEY")),
		MDBListAPIKey:  strings.TrimSpace(os.Getenv("BOOKRR_MDBLIST_API_KEY")),
		ProwlarrURL:    strings.TrimRight(strings.TrimSpace(os.Getenv("BOOKRR_PROWLARR_URL")), "/"),
		ProwlarrAPIKey: strings.TrimSpace(os.Getenv("BOOKRR_PROWLARR_API_KEY")),
	}
	interval := env("BOOKRR_SYNC_INTERVAL", "5m")
	d, err := time.ParseDuration(interval)
	if err != nil {
		return c, fmt.Errorf("BOOKRR_SYNC_INTERVAL %q non valido: %w", interval, err)
	}
	if d < 30*time.Second {
		d = 30 * time.Second
	}
	c.SyncInterval = d
	interval = env("BOOKRR_S3_SCAN_INTERVAL", "1h")
	if d, err = time.ParseDuration(interval); err != nil {
		return c, fmt.Errorf("BOOKRR_S3_SCAN_INTERVAL %q non valido: %w", interval, err)
	}
	c.S3ScanInterval = max(d, time.Minute)
	interval = env("BOOKRR_UNIT3D_RELEASES_INTERVAL", "6h")
	if d, err = time.ParseDuration(interval); err != nil {
		return c, fmt.Errorf("BOOKRR_UNIT3D_RELEASES_INTERVAL %q non valido: %w", interval, err)
	}
	c.Unit3DReleasesInterval = max(d, 15*time.Minute)
	interval = env("BOOKRR_PROWLARR_RECHECK", "7d")
	if d, err = days(interval); err != nil {
		return c, fmt.Errorf("BOOKRR_PROWLARR_RECHECK %q non valido (es. 7d o 36h): %w", interval, err)
	}
	c.ProwlarrRecheck = max(d, 24*time.Hour)
	at := env("BOOKRR_MISSING_CHECK_AT", "04:00")
	t, err := time.Parse("15:04", at)
	if err != nil {
		return c, fmt.Errorf("BOOKRR_MISSING_CHECK_AT %q non valido (es. 04:00)", at)
	}
	c.MissingCheckAt = t.Hour()*60 + t.Minute()
	if (c.AuthUser == "") != (c.AuthPassword == "") {
		return c, fmt.Errorf("BOOKRR_AUTH_USER e BOOKRR_AUTH_PASSWORD vanno impostati insieme")
	}
	if (c.Unit3DURL == "") != (c.Unit3DAPIKey == "") {
		return c, fmt.Errorf("BOOKRR_UNIT3D_URL e BOOKRR_UNIT3D_API_KEY vanno impostati insieme")
	}
	if c.Unit3DURL != "" {
		if u, err := url.Parse(c.Unit3DURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return c, fmt.Errorf("BOOKRR_UNIT3D_URL %q non valido (es. https://tracker.example)", c.Unit3DURL)
		}
	}
	if (c.ProwlarrURL == "") != (c.ProwlarrAPIKey == "") {
		return c, fmt.Errorf("BOOKRR_PROWLARR_URL e BOOKRR_PROWLARR_API_KEY vanno impostati insieme")
	}
	if c.ProwlarrURL != "" {
		if u, err := url.Parse(c.ProwlarrURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return c, fmt.Errorf("BOOKRR_PROWLARR_URL %q non valido (es. http://prowlarr:9696)", c.ProwlarrURL)
		}
	}
	return c, nil
}

// days reads a duration that may also be in days ("7d").
func days(s string) (time.Duration, error) {
	if n, ok := strings.CutSuffix(strings.TrimSpace(s), "d"); ok {
		v, err := strconv.Atoi(n)
		if err != nil {
			return 0, err
		}
		return time.Duration(v) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
