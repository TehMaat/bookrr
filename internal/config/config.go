// Package config reads bookrr settings from environment variables.
package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
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
}

func (c Config) DBPath() string { return filepath.Join(c.DataDir, "bookrr.db") }

func Load() (Config, error) {
	c := Config{
		Listen:       env("BOOKRR_LISTEN", ":8080"),
		DataDir:      env("BOOKRR_DATA_DIR", "/data"),
		ReleaseTag:   env("BOOKRR_RELEASE_TAG", "Personal Release"),
		WebhookToken: os.Getenv("BOOKRR_WEBHOOK_TOKEN"),
		AuthUser:     os.Getenv("BOOKRR_AUTH_USER"),
		AuthPassword: os.Getenv("BOOKRR_AUTH_PASSWORD"),
		Unit3DURL:    strings.TrimRight(strings.TrimSpace(os.Getenv("BOOKRR_UNIT3D_URL")), "/"),
		Unit3DAPIKey: strings.TrimSpace(os.Getenv("BOOKRR_UNIT3D_API_KEY")),
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
	return c, nil
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
