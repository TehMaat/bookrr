package api

import (
	"net/http"
	"testing"
)

func TestReleasesWithoutTracker(t *testing.T) {
	_, h := newServer(t)
	var out struct {
		Enabled  bool  `json:"enabled"`
		Releases []any `json:"releases"`
	}
	if code := call(t, h, "GET", "/api/releases", nil, &out); code != http.StatusOK || out.Enabled || out.Releases == nil {
		t.Fatalf("code %d: %+v", code, out)
	}
	if code := call(t, h, "POST", "/api/releases/sync", nil, nil); code != http.StatusBadRequest {
		t.Fatalf("sync without tracker: %d", code)
	}
	if code := call(t, h, "POST", "/api/releases/add", map[string]any{"ids": []int{7}}, nil); code != http.StatusNotFound {
		t.Fatalf("add unknown release: %d", code)
	}
}

func TestSameSite(t *testing.T) {
	site := "https://tracker.example"
	for announce, want := range map[string]bool{
		"":                                    true,
		"https://tracker.example/announce/x":  true,
		"https://announce.tracker.example/xx": true,
		"udp://other.org:6969/announce":       false,
	} {
		if got := sameSite(announce, site); got != want {
			t.Errorf("%q: got %v", announce, got)
		}
	}
}
