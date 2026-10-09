// Package config reads bookrr settings from environment variables.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Config struct {
	Listen       string
	DataDir      string
	SyncInterval time.Duration
	ReleaseTag   string
	WebhookToken string
	AuthUser     string
	AuthPassword string
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
	if (c.AuthUser == "") != (c.AuthPassword == "") {
		return c, fmt.Errorf("BOOKRR_AUTH_USER e BOOKRR_AUTH_PASSWORD vanno impostati insieme")
	}
	return c, nil
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
