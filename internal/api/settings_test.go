package api

import (
	"testing"

	"github.com/tehmaat/bookrr/internal/store"
)

func TestNormalizePublicURL(t *testing.T) {
	ok := map[string]string{
		"":                           "",
		"  ":                         "",
		"192.168.1.10:8080":          "http://192.168.1.10:8080",
		"http://192.168.1.10:8080/":  "http://192.168.1.10:8080",
		"https://bookrr.lan":         "https://bookrr.lan",
		"http://nas.lan:8080/bookrr": "http://nas.lan:8080/bookrr",
	}
	for in, want := range ok {
		got, err := normalizePublicURL(in)
		if err != nil || got != want {
			t.Errorf("normalizePublicURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"ftp://nas.lan", "http://", "http://nas.lan/?a=1", "http://nas.lan/#disks"} {
		if got, err := normalizePublicURL(in); err == nil {
			t.Errorf("normalizePublicURL(%q) = %q, want error", in, got)
		}
	}
}

// Saving only some options (the QR dialog sends just the address) keeps
// the others; invalid thresholds are refused.
func TestSaveSettingsPartial(t *testing.T) {
	_, h := newServer(t)
	var st store.Settings
	if code := call(t, h, "PUT", "/api/settings", map[string]any{"archiveMinSeeders": 3, "archiveMinCompleted": 2, "archiveDays": 7}, &st); code != 200 {
		t.Fatalf("status %d", code)
	}
	if code := call(t, h, "PUT", "/api/settings", map[string]any{"publicUrl": "nas.lan:8080"}, &st); code != 200 {
		t.Fatalf("status %d", code)
	}
	want := store.Settings{PublicURL: "http://nas.lan:8080", ArchiveMinSeeders: 3, ArchiveMinCompleted: 2, ArchiveDays: 7}
	if st != want {
		t.Fatalf("got %+v, want %+v", st, want)
	}
	if code := call(t, h, "PUT", "/api/settings", map[string]any{"archiveDays": -1}, nil); code != 400 {
		t.Fatalf("negative days: status %d", code)
	}
	if code := call(t, h, "GET", "/api/settings", nil, &st); code != 200 || st != want {
		t.Fatalf("after invalid save: %d %+v", code, st)
	}
}
