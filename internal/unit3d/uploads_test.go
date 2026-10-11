package unit3d

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/tehmaat/bookrr/internal/store"
)

// fileFor is a .torrent file whose info dictionary names the release, so
// that every release has its own hash.
func fileFor(name string) ([]byte, string) {
	info := fmt.Sprintf("d6:lengthi1234e4:name%d:%s12:piece lengthi16384e6:pieces20:aaaaaaaaaaaaaaaaaaaa7:privatei1ee", len(name), name)
	sum := sha1.Sum([]byte(info))
	return []byte("d8:announce22:https://t.example/a/xx4:info" + info + "e"), hex.EncodeToString(sum[:])
}

// uploadsTracker serves /api/user, the user's uploads split over pages
// (the first linked by cursor, the next by a "next" link that drops the
// filters and points to another host) and their .torrent files.
type uploadsTracker struct {
	t         *testing.T
	srv       *httptest.Server
	mu        sync.Mutex
	pages     [][]map[string]any
	downloads map[string]int
	limited   map[string]bool // answer 429 once
	queries   []string
}

func newUploadsTracker(t *testing.T) *uploadsTracker {
	ut := &uploadsTracker{t: t, downloads: map[string]int{}, limited: map[string]bool{}}
	ut.srv = httptest.NewServer(http.HandlerFunc(ut.serve))
	t.Cleanup(ut.srv.Close)
	return ut
}

func (ut *uploadsTracker) client() *Client {
	c := New(ut.srv.URL, "KEY")
	c.MinInterval = 0
	return c
}

func (ut *uploadsTracker) serve(w http.ResponseWriter, r *http.Request) {
	ut.mu.Lock()
	defer ut.mu.Unlock()
	if name, ok := strings.CutPrefix(r.URL.Path, "/download/"); ok {
		if ut.limited[name] {
			delete(ut.limited, name)
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		ut.downloads[name]++
		f, _ := fileFor(name)
		w.Write(f)
		return
	}
	if r.Header.Get("Authorization") != "Bearer KEY" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	switch r.URL.Path {
	case "/api/user":
		// Wrapped in "data", as a Laravel resource.
		json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"username": "Mario Rossi", "group": "Uploader"}})
	case "/api/torrents/filter":
		q := r.URL.Query()
		ut.queries = append(ut.queries, r.URL.RawQuery)
		if q.Get("uploader") != "Mario Rossi" || q.Get("perPage") != "100" || q.Get("bookrr_user") != "Mario Rossi" {
			ut.t.Errorf("unexpected query %q", r.URL.RawQuery)
		}
		page := 0
		switch {
		case q.Get("cursor") == "eyJpZCI6MTB9":
			page = 1
		case q.Get("page") != "":
			page, _ = strconv.Atoi(q.Get("page"))
		}
		res := map[string]any{"data": ut.pages[page], "links": map[string]any{"next": nil}, "meta": map[string]any{"next_cursor": nil}}
		if page+1 < len(ut.pages) {
			if page == 0 {
				res["meta"] = map[string]any{"next_cursor": "eyJpZCI6MTB9", "per_page": 100}
			} else {
				res["links"] = map[string]any{"next": fmt.Sprintf("https://elsewhere.example/api/torrents/filter?page=%d", page+1)}
			}
		}
		json.NewEncoder(w).Encode(res)
	default:
		http.NotFound(w, r)
	}
}

func (ut *uploadsTracker) upload(id any, name string, attrs map[string]any) map[string]any {
	a := map[string]any{
		"name": name, "size": 1234, "download_link": ut.srv.URL + "/download/" + name,
		"details_link": fmt.Sprintf("%s/torrents/%v", ut.srv.URL, id),
	}
	for k, v := range attrs {
		a[k] = v
	}
	return map[string]any{"type": "torrent", "id": id, "attributes": a}
}

func TestUploads(t *testing.T) {
	ctx := context.Background()
	ut := newUploadsTracker(t)
	ut.pages = [][]map[string]any{
		{ut.upload(12, "Uno", map[string]any{
			"category": "Movies", "type": "WEB-DL", "type_id": 4, "resolution": "2160p", "resolution_id": "2",
			"seeders": 7, "leechers": "1", "times_completed": 30, "tmdb_id": 550, "imdb_id": "tt0137523", "tvdb_id": nil,
			"personal_release": true, "created_at": "2024-05-01T10:00:00.000000Z", "size": 1.5e10,
		})},
		{ut.upload("13", "Due", map[string]any{"personal_release": 0, "created_at": "2024-06-02 08:30:00", "tvdb_id": "", "seeders": 2.0})},
		{ut.upload("14", "Tre", map[string]any{"personal_release": "1", "imdb_id": 123})},
	}
	c := ut.client()
	name, err := c.Username(ctx)
	if err != nil || name != "Mario Rossi" {
		t.Fatalf("username %q %v", name, err)
	}
	ups, err := c.Uploads(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	if len(ups) != 3 || len(ut.queries) != 3 {
		t.Fatalf("got %d uploads in %d requests: %+v", len(ups), len(ut.queries), ups)
	}
	u := ups[0]
	if u.ID != 12 || u.Name != "Uno" || u.Category != "Movies" || u.Type != "WEB-DL" || u.TypeID != 4 || u.Resolution != "2160p" ||
		u.ResolutionID != 2 || u.Size != 15000000000 || u.Seeders != 7 || u.Leechers != 1 || u.TimesCompleted != 30 ||
		u.TmdbID != 550 || u.ImdbID != 137523 || u.TvdbID != 0 || !u.PersonalRelease || u.CreatedAt != "2024-05-01T10:00:00Z" ||
		!strings.HasSuffix(u.DetailsLink, "/torrents/12") {
		t.Fatalf("unexpected first upload: %+v", u)
	}
	if u := ups[1]; u.ID != 13 || u.PersonalRelease || u.CreatedAt != "2024-06-02T08:30:00Z" || u.Seeders != 2 {
		t.Fatalf("unexpected second upload: %+v", u)
	}
	if u := ups[2]; u.ID != 14 || !u.PersonalRelease || u.ImdbID != 123 {
		t.Fatalf("unexpected third upload: %+v", u)
	}
	hash, file, err := c.UploadHash(ctx, ups[0])
	if _, want := fileFor("Uno"); err != nil || hash != want || len(file) == 0 {
		t.Fatalf("hash %s %v", hash, err)
	}

	// An upload without a valid id is an error, not a silent skip.
	ut.pages = [][]map[string]any{{ut.upload("abc", "X", nil)}}
	if _, err := c.Uploads(ctx, name); err == nil {
		t.Fatal("expected an error for a non numeric id")
	}
}

func TestMatchRelease(t *testing.T) {
	cs := []store.ReleaseCandidate{
		{ID: 1, Name: "La Mia Release", Size: 100, Hash: "a"},
		{ID: 2, Name: "Criminal Minds S19 2160p UHD DSNP WEB-DL DD+ 5.1 DV HDR10+ H.265-MaTiTa", Size: 1000, Hash: "b"},
		{ID: 3, Name: "Doppia 2020 1080p WEB-DL H.264-GRP", Size: 500, Hash: "c", Removed: true},
		{ID: 4, Name: "Doppia 2020 1080p WEB-DL H.264-GRP", Size: 500, Hash: "d"},
	}
	cases := []struct {
		name string
		size int64
		want int64 // 0: no match
	}{
		{"La.Mia.Release", 0, 1},
		{"Criminal.Minds.S19.2160p.DSNP.WEB-DL.DDP5.1.DV.HDR10.H.265-MaTiTa", 1010, 2},
		{"Criminal.Minds.S19.2160p.DSNP.WEB-DL.DDP5.1.DV.HDR10.H.265-MaTiTa", 2000, 0}, // size too different
		{"Criminal.Minds.S19E01.2160p.DSNP.WEB-DL.DDP5.1.DV.HDR10.H.265-MaTiTa", 0, 0}, // an episode is not the season
		{"Doppia.2020.1080p.WEB-DL.H.264-GRP", 0, 4},                                   // the one still on the tracker
		{"Sconosciuta", 0, 0},
	}
	for _, tc := range cases {
		c, ok := matchRelease(tc.name, tc.size, cs)
		if (tc.want == 0) == ok || (ok && c.ID != tc.want) {
			t.Errorf("%s: got %d %v, want %d", tc.name, c.ID, ok, tc.want)
		}
	}
}

// A full round: uploads are saved, their hash computed once, linked to
// the torrents of bookrr by hash and, for those added without, by name.
func TestHistorySync(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	_, onClient := fileFor("Sul.Client")
	client, err := st.CreateClient(ctx, store.Client{Name: "qb", URL: "http://qb", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.ApplySnapshot(ctx, client, []store.SnapshotTorrent{{Hash: onClient, Name: "Sul.Client", Size: 1234, Tags: "Personal Release"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.FinalizeSync(ctx, nil, "Personal Release"); err != nil {
		t.Fatal(err)
	}
	placeholder := store.ManualHashPrefix + "1"
	if _, err := st.CreateManualTorrent(ctx, store.ManualInput{
		TorrentInput: store.TorrentInput{Hash: placeholder, Name: "Archiviata.2023.1080p.WEB-DL.H.264-GRP", Size: 1234, PersonalRelease: true},
		Archive:      &store.ArchiveInput{Path: "/archivio"},
	}); err != nil {
		t.Fatal(err)
	}
	gone := "0123456789abcdef0123456789abcdef01234567"
	if _, err := st.CreateManualTorrent(ctx, store.ManualInput{
		TorrentInput: store.TorrentInput{Hash: gone, Name: "Cancellata", PersonalRelease: true},
	}); err != nil {
		t.Fatal(err)
	}

	ut := newUploadsTracker(t)
	ut.pages = [][]map[string]any{{
		ut.upload(1, "Sul.Client", map[string]any{"seeders": 3}),
		ut.upload("2", "Archiviata 2023 1080p WEB-DL H.264-GRP", nil),
		ut.upload(3, "Nuova", map[string]any{"personal_release": 1}),
	}}
	ut.limited["Nuova"] = true
	h := NewHistory(st, ut.client(), 0)
	h.Sync(ctx)
	if s := h.State(); s.LastError != "" || s.Username != "Mario Rossi" || s.Running {
		t.Fatalf("unexpected state: %+v", s)
	}

	rs, err := st.ListTrackerReleases(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[int64]store.TrackerRelease{}
	for _, r := range rs {
		byID[r.ID] = r
	}
	_, archived := fileFor("Archiviata 2023 1080p WEB-DL H.264-GRP")
	if r := byID[1]; r.Hash != onClient || r.TorrentHash != onClient || r.Status != store.StatusClient || r.Seeders != 3 {
		t.Fatalf("release on a client: %+v", r)
	}
	if r := byID[2]; r.Hash != archived || r.TorrentHash != archived || r.Status != store.StatusArchived {
		t.Fatalf("release linked by name: %+v", r)
	}
	if r := byID[3]; r.TorrentHash != "" || r.Status != store.ReleaseUntracked || !r.HasTorrentFile || !r.PersonalRelease {
		t.Fatalf("unknown release: %+v", r)
	}
	if found, _ := st.TorrentExists(ctx, placeholder); found {
		t.Fatal("the placeholder must have its real hash")
	}
	tr, err := st.GetTorrent(ctx, archived)
	if err != nil || len(tr.Archives) != 1 || !tr.HasTorrentFile || !strings.HasSuffix(tr.TrackerURL, "/torrents/2") {
		t.Fatalf("linked torrent: %+v %v", tr, err)
	}
	if tr, _ := st.GetTorrent(ctx, onClient); !tr.HasTorrentFile || tr.TrackerURL == "" {
		t.Fatalf("the torrent on the client must get the .torrent and the link: %+v", tr)
	}
	missing, err := st.PersonalReleasesNotOnTracker(ctx)
	if err != nil || len(missing) != 1 || missing[0].Hash != gone {
		t.Fatalf("not on tracker: %+v %v", missing, err)
	}

	// Next round: counters are refreshed, nothing is downloaded again, and
	// an upload no longer listed stays in the history as removed.
	ut.pages = [][]map[string]any{{
		ut.upload(1, "Sul.Client", map[string]any{"seeders": 9}),
		ut.upload(3, "Nuova", nil),
	}}
	h.Sync(ctx)
	for name, n := range ut.downloads {
		if n != 1 {
			t.Errorf("%s downloaded %d times", name, n)
		}
	}
	if r, _ := st.GetTrackerRelease(ctx, 1); r.Seeders != 9 {
		t.Fatalf("seeders not refreshed: %+v", r)
	}
	if r, _ := st.GetTrackerRelease(ctx, 2); r.RemovedAt == nil || r.Status != store.StatusArchived {
		t.Fatalf("release removed from the tracker: %+v", r)
	}
	if missing, _ := st.PersonalReleasesNotOnTracker(ctx); len(missing) != 2 {
		t.Fatalf("a removed upload is no longer on the tracker: %+v", missing)
	}

	// "Aggiungi a bookrr".
	if n, err := st.AddTrackerReleases(ctx, []int64{3}); err != nil || n != 1 {
		t.Fatalf("add: %d %v", n, err)
	}
	_, nuova := fileFor("Nuova")
	tr, err = st.GetTorrent(ctx, nuova)
	if err != nil || !tr.PersonalRelease || tr.Name != "Nuova" || tr.Size != 1234 || !tr.HasTorrentFile || tr.Status != store.StatusUnknown {
		t.Fatalf("added torrent: %+v %v", tr, err)
	}
	if _, err := st.AddTrackerReleases(ctx, []int64{3}); err != store.ErrExists {
		t.Fatalf("adding twice: %v", err)
	}
}
