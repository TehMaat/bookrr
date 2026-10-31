package config

import (
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
