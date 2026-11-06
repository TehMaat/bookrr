package config

import (
	"log/slog"
	"path/filepath"
	"testing"
	"time"
)

func TestResolve(t *testing.T) {
	t.Setenv("BOOKRR_SYNC_INTERVAL", "2m")
	stored := map[string]string{
		"BOOKRR_SYNC_INTERVAL":    "10m", // the environment wins
		"BOOKRR_UNIT3D_URL":       "https://tracker.example/",
		"BOOKRR_UNIT3D_API_KEY":   "key",
		"BOOKRR_PROWLARR_RECHECK": "3d",
		"BOOKRR_AUTH_USER":        "admin", // not an option: ignored
	}
	c, err := Resolve(stored)
	if err != nil {
		t.Fatal(err)
	}
	if c.SyncInterval != 2*time.Minute || c.Unit3DURL != "https://tracker.example" || c.Unit3DAPIKey != "key" ||
		c.ProwlarrRecheck != 72*time.Hour || c.AuthUser != "" || c.ReleaseTag != "Personal Release" {
		t.Fatalf("got %+v", c)
	}
	if !Locked("BOOKRR_SYNC_INTERVAL") || Locked("BOOKRR_UNIT3D_URL") {
		t.Fatal("locked")
	}
	// The same checks as for the environment.
	if _, err := Resolve(map[string]string{"BOOKRR_UNIT3D_URL": "https://tracker.example"}); err == nil {
		t.Fatal("URL without key accepted")
	}
	if _, err := Resolve(map[string]string{"BOOKRR_MISSING_CHECK_AT": "25:00"}); err == nil {
		t.Fatal("bad time accepted")
	}
}

func TestResolveLogs(t *testing.T) {
	c, err := Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	if c.LogRetentionDays != 30 || c.LogMaxSize != 50<<20 || c.LogLevel != slog.LevelInfo || c.LogDir() != filepath.Join("/data", "logs") {
		t.Fatalf("defaults %+v", c)
	}
	c, err = Resolve(map[string]string{"BOOKRR_LOG_RETENTION": "0", "BOOKRR_LOG_MAX_SIZE": "1.5 GB", "BOOKRR_LOG_LEVEL": "DEBUG"})
	if err != nil {
		t.Fatal(err)
	}
	if c.LogRetentionDays != 0 || c.LogMaxSize != 3<<29 || c.LogLevel != slog.LevelDebug {
		t.Fatalf("got %+v", c)
	}
	// At least a day, at least 1MB.
	c, err = Resolve(map[string]string{"BOOKRR_LOG_RETENTION": "36h", "BOOKRR_LOG_MAX_SIZE": "10k"})
	if err != nil || c.LogRetentionDays != 2 || c.LogMaxSize != 1<<20 {
		t.Fatalf("got %+v %v", c, err)
	}
	if c, err = Resolve(map[string]string{"BOOKRR_LOG_MAX_SIZE": "0"}); err != nil || c.LogMaxSize != 0 {
		t.Fatalf("no limit: %+v %v", c, err)
	}
	for k, v := range map[string]string{"BOOKRR_LOG_RETENTION": "-1d", "BOOKRR_LOG_MAX_SIZE": "tanti", "BOOKRR_LOG_LEVEL": "loud"} {
		if _, err := Resolve(map[string]string{k: v}); err == nil {
			t.Errorf("%s=%s accepted", k, v)
		}
	}
}
