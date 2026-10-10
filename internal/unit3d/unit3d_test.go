package unit3d

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/tehmaat/bookrr/internal/store"
)

const info = "d6:lengthi1234e4:name14:La.Mia.Release12:piece lengthi16384e6:pieces20:aaaaaaaaaaaaaaaaaaaa7:privatei1e6:source4:TESTe"

func torrentFile() []byte {
	return []byte("d8:announce22:https://t.example/a/xx7:comment3:abc4:info" + info + "e")
}

func infoHash() string {
	sum := sha1.Sum([]byte(info))
	return hex.EncodeToString(sum[:])
}

func TestInfoHash(t *testing.T) {
	got, err := InfoHash(torrentFile())
	if err != nil {
		t.Fatal(err)
	}
	if got != infoHash() {
		t.Fatalf("got %s want %s", got, infoHash())
	}
	for _, bad := range []string{"", "le", "d4:infoi1ee", "d4:info", "d99:infoe", "d8:announce3:abce"} {
		if _, err := InfoHash([]byte(bad)); err == nil {
			t.Errorf("%q: expected an error", bad)
		}
	}
}

// fakeTracker answers /api/torrents/filter with the given torrents and
// serves the .torrent file at /download.
func fakeTracker(t *testing.T, data func(base string) []map[string]any) (*Client, *int) {
	t.Helper()
	downloads := 0
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/torrents/filter":
			if r.Header.Get("Authorization") != "Bearer KEY" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{"data": data(srv.URL)})
		case "/download":
			downloads++
			w.Write(torrentFile())
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	c := New(srv.URL, "KEY")
	c.MinInterval = 0
	return c, &downloads
}

func item(id any, name string, size int64, extra map[string]any) map[string]any {
	a := map[string]any{"name": name, "size": size}
	for k, v := range extra {
		a[k] = v
	}
	return map[string]any{"type": "torrent", "id": id, "attributes": a}
}

func TestFindHash(t *testing.T) {
	ctx := context.Background()
	other := "0123456789abcdef0123456789abcdef01234567"
	cases := []struct {
		name      string
		data      func(base string) []map[string]any
		size      int64
		want      string
		err       error
		downloads int
	}{
		{
			name: "info_hash in the API",
			data: func(string) []map[string]any {
				return []map[string]any{
					item("1", "La.Mia.Release.Extended", 1, nil),
					item("2", "La Mia Release", 1, map[string]any{"info_hash": "0123456789ABCDEF0123456789ABCDEF01234567"}),
				}
			},
			want: other,
		},
		{
			name: "hash from the magnet link",
			data: func(string) []map[string]any {
				return []map[string]any{item(7, "la.mia.release", 1, map[string]any{"magnet_link": "magnet:?dn=x&xt=urn:btih:" + other + "&tr=y"})}
			},
			want: other,
		},
		{
			name: "hash from the .torrent file",
			data: func(base string) []map[string]any {
				return []map[string]any{item("3", "La.Mia.Release", 1234, map[string]any{"download_link": base + "/download", "magnet_link": nil})}
			},
			want:      infoHash(),
			downloads: 1,
		},
		{
			name: "size picks between equal names",
			data: func(base string) []map[string]any {
				return []map[string]any{
					item("4", "La.Mia.Release", 500000, map[string]any{"info_hash": other}),
					item("5", "La.Mia.Release", 1000000, map[string]any{"download_link": base + "/download"}),
				}
			},
			size:      1000500,
			want:      infoHash(),
			downloads: 1,
		},
		{
			name: "same name without size",
			data: func(string) []map[string]any {
				return []map[string]any{item("4", "La.Mia.Release", 1, nil), item("5", "La.Mia.Release", 2, nil)}
			},
			err: ErrAmbiguous,
		},
		{
			name: "only similar names",
			data: func(string) []map[string]any {
				return []map[string]any{item("6", "La.Mia.Release.REPACK", 1, map[string]any{"info_hash": other})}
			},
			err: ErrNotFound,
		},
		{
			name: "no hash and no download link",
			data: func(string) []map[string]any { return []map[string]any{item("8", "La.Mia.Release", 1, nil)} },
			err:  ErrNoHash,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, downloads := fakeTracker(t, tc.data)
			m, err := c.FindHash(ctx, "La.Mia.Release", tc.size)
			if tc.err != nil {
				if !errors.Is(err, tc.err) || !Final(err) {
					t.Fatalf("got %v, want %v", err, tc.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if m.Hash != tc.want || *downloads != tc.downloads {
				t.Fatalf("got %s (%d downloads), want %s (%d)", m.Hash, *downloads, tc.want, tc.downloads)
			}
		})
	}
}

func TestFindHashErrors(t *testing.T) {
	ctx := context.Background()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c := New(srv.URL, "KEY")
	var rl *RateLimitError
	if _, err := c.FindHash(ctx, "x", 0); !errors.As(err, &rl) || rl.RetryAfter.Seconds() != 7 || Final(err) {
		t.Fatalf("expected a rate limit error, got %v", err)
	}

	c, _ = fakeTracker(t, func(string) []map[string]any { return nil })
	c.apiKey = "WRONG"
	if _, err := c.FindHash(ctx, "x", 0); err == nil || Final(err) {
		t.Fatalf("a wrong key must be a temporary error, got %v", err)
	}
}

func TestResolver(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	add := func(hash, name string) {
		t.Helper()
		in := store.ManualInput{
			TorrentInput: store.TorrentInput{Hash: hash, Name: name, PersonalRelease: true},
			Archive:      &store.ArchiveInput{Path: "/archivio/" + name},
		}
		if _, err := st.CreateManualTorrent(ctx, in); err != nil {
			t.Fatal(err)
		}
	}
	add(store.ManualHashPrefix+"1", "La.Mia.Release")
	add(store.ManualHashPrefix+"2", "Sconosciuto")

	c, _ := fakeTracker(t, func(base string) []map[string]any {
		return []map[string]any{item("3", "La.Mia.Release", 1234, map[string]any{"download_link": base + "/download"})}
	})
	r := NewResolver(st, c)
	r.runPending(ctx)

	got, err := st.GetTorrent(ctx, infoHash())
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "La.Mia.Release" || len(got.Archives) != 1 || got.Archives[0].Path != "/archivio/La.Mia.Release" || !got.PersonalRelease {
		t.Fatalf("unexpected torrent: %+v", got)
	}
	if found, _ := st.TorrentExists(ctx, store.ManualHashPrefix+"1"); found {
		t.Fatal("the placeholder must be gone")
	}
	missing, err := st.GetTorrent(ctx, store.ManualHashPrefix+"2")
	if err != nil {
		t.Fatal(err)
	}
	if missing.HashLookupAt == nil || missing.HashLookupError == "" {
		t.Fatalf("a failed lookup must be recorded: %+v", missing)
	}
	// Not retried before RetryAfter.
	if p, _ := st.PendingHashLookups(ctx, time.Now().Add(-r.RetryAfter), 10); len(p) != 0 {
		t.Fatalf("nothing should be pending, got %+v", p)
	}
}
