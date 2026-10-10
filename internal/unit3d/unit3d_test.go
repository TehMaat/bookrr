package unit3d

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
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

// fakeTracker answers /api/torrents/filter and /api/torrents/{id} with the
// given torrents and serves the .torrent file at /download.
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
			id, ok := strings.CutPrefix(r.URL.Path, "/api/torrents/")
			if !ok {
				http.NotFound(w, r)
				return
			}
			for _, it := range data(srv.URL) {
				if fmt.Sprint(it["id"]) == id {
					json.NewEncoder(w).Encode(it)
					return
				}
			}
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
			name: "a bare title is not matched to a longer one",
			data: func(string) []map[string]any {
				return []map[string]any{item("6", "La Mia Release 2160p REPACK", 1234, map[string]any{"info_hash": other})}
			},
			err: ErrNotFound,
		},
		{
			name: "containing every word but another size",
			data: func(string) []map[string]any {
				return []map[string]any{item("6", "La Mia Release 2160p REPACK", 5000, map[string]any{"info_hash": other})}
			},
			size: 1234,
			err:  ErrNotFound,
		},
		{
			name: "several torrents containing every word",
			data: func(string) []map[string]any {
				return []map[string]any{item("6", "La.Mia.Release.REPACK", 1, nil), item("7", "La.Mia.Release.PROPER", 1, nil)}
			},
			err: ErrNotFound,
		},
		{
			name: "the .torrent file wins over info_hash",
			data: func(base string) []map[string]any {
				return []map[string]any{item("9", "La.Mia.Release", 1, map[string]any{"info_hash": other, "download_link": base + "/download"})}
			},
			want:      infoHash(),
			downloads: 1,
		},
		{
			name: "info_hash when the download fails",
			data: func(base string) []map[string]any {
				return []map[string]any{item("9", "La.Mia.Release", 1, map[string]any{"info_hash": other, "download_link": base + "/missing"})}
			},
			want: other,
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
			if m.Hash != tc.want || *downloads != tc.downloads || (len(m.File) > 0) != (tc.downloads > 0) {
				t.Fatalf("got %s (%d downloads, file %d bytes), want %s (%d)", m.Hash, *downloads, len(m.File), tc.want, tc.downloads)
			}
		})
	}
}

// The example from the issue: a file name that differs from the tracker title.
func TestCandidatesAndFetch(t *testing.T) {
	ctx := context.Background()
	local := "AD.ASTRA.2019.REMUX.2160P.VU.HDR10.DTS.ITA.TRUEHD.ENG.SUBS.ITA.ENG"
	remux := "Ad astra 2019 2160p UHD VU REMUX TrueHD 7.1 Atmos DD 5.1 DTS 5.1 DD 2.0 ENG ITA SUBS HDR10 H.265-MaTiTa"
	c, downloads := fakeTracker(t, func(base string) []map[string]any {
		return []map[string]any{
			item("1", "Ad Astra 2019 1080p BluRay x264", 8, nil),
			item(2, remux, 60, map[string]any{"download_link": base + "/download", "details_link": base + "/torrents/2"}),
		}
	})
	if !sameRelease(local, remux) {
		t.Fatal("same release")
	}
	if got := searchForms(local); len(got) != 4 || got[2] != "ad astra 2019" || got[3] != "ad astra" {
		t.Fatalf("search forms: %q", got)
	}
	cs, err := c.Candidates(ctx, local, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 2 || cs[0].ID != "2" || !cs[0].Match || cs[1].Match || cs[0].Score <= cs[1].Score || cs[0].DetailsLink == "" {
		t.Fatalf("unexpected candidates: %+v", cs)
	}
	m, err := c.Fetch(ctx, "2")
	if err != nil {
		t.Fatal(err)
	}
	if m.Hash != infoHash() || len(m.File) == 0 || m.Name != remux || *downloads != 1 {
		t.Fatalf("unexpected match: %+v", m)
	}
	if _, err := c.Fetch(ctx, "../user"); err == nil {
		t.Fatal("a non numeric id must be rejected")
	}
	// Two results, but only the REMUX is the same release.
	if m, err := c.FindHash(ctx, local, 0); err != nil || m.ID != "2" {
		t.Fatalf("expected the REMUX, got %+v %v", m, err)
	}
	// The same release twice: the user chooses.
	c, _ = fakeTracker(t, func(base string) []map[string]any {
		return []map[string]any{item("2", remux, 60, nil), item("3", remux+" ", 60, nil)}
	})
	if _, err := c.FindHash(ctx, local, 0); !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("expected ambiguous, got %v", err)
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
	if got.Name != "La.Mia.Release" || len(got.Archives) != 1 || got.Archives[0].Path != "/archivio/La.Mia.Release" || !got.PersonalRelease || !got.HasTorrentFile {
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
