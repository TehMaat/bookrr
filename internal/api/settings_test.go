package api

import "testing"

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
