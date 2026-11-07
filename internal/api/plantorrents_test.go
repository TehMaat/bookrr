package api

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/tehmaat/bookrr/internal/config"
	"github.com/tehmaat/bookrr/internal/store"
	"github.com/tehmaat/bookrr/internal/syncer"
)

// torrentOf is a .torrent file of one file, with its info hash.
func torrentOf(name string, size int) ([]byte, string) {
	info := "d6:lengthi" + itoa(int64(size)) + "e4:name" + itoa(int64(len(name))) + ":" + name +
		"12:piece lengthi16384e6:pieces20:aaaaaaaaaaaaaaaaaaaae"
	sum := sha1.Sum([]byte(info))
	return []byte("d8:announce9:http://t/4:info" + info + "e"), hex.EncodeToString(sum[:])
}

// upload sends a .torrent file as the web UI does.
func upload(t *testing.T, h http.Handler, method, path string, data []byte, label string, out any) int {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "x.torrent")
	fw.Write(data)
	if label != "" {
		mw.WriteField("label", label)
	}
	mw.Close()
	req := httptest.NewRequest(method, path, &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if out != nil {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			t.Fatalf("%s %s: %v (%s)", method, path, err, rec.Body.String())
		}
	}
	return rec.Code
}

// fakeQbit is a qBittorrent that takes torrents and lists them complete.
type fakeQbit struct {
	mu     sync.Mutex
	added  []string // info hashes
	opts   []string // category|tags|savepath
	refuse bool
}

func (f *fakeQbit) serve(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.URL.Path {
		case "/api/v2/torrents/add":
			if f.refuse {
				w.Write([]byte("Fails."))
				return
			}
			file, _, err := r.FormFile("torrents")
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			data, _ := io.ReadAll(file)
			// The info hash, as qBittorrent would compute it.
			i := bytes.Index(data, []byte("4:info"))
			sum := sha1.Sum(data[i+6 : len(data)-1])
			f.added = append(f.added, hex.EncodeToString(sum[:]))
			f.opts = append(f.opts, r.FormValue("category")+"|"+r.FormValue("tags")+"|"+r.FormValue("savepath"))
			w.Write([]byte("Ok."))
		case "/api/v2/torrents/info":
			out := []map[string]any{}
			for _, h := range f.added {
				out = append(out, map[string]any{"hash": h, "name": h, "state": "uploading", "progress": 1})
			}
			json.NewEncoder(w).Encode(out)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestPlanTorrents(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	sy := syncer.New(st, "mine", time.Hour)
	srv := New(config.Config{}, st, sy, nil, nil, nil, nil, nil, fstest.MapFS{}, "test")
	h := srv.Handler()
	e, err := st.AddPlanEntry(ctx, store.PlanEntry{TmdbID: 603, Title: "Matrix"})
	if err != nil {
		t.Fatal(err)
	}
	base := "/api/plan/" + itoa(e.ID) + "/torrents"

	var errOut map[string]string
	if code := upload(t, h, "POST", base, []byte("not a torrent"), "", &errOut); code != http.StatusBadRequest ||
		!strings.Contains(errOut["error"], "not a valid .torrent") {
		t.Fatalf("invalid file: %d %v", code, errOut)
	}
	if code := upload(t, h, "POST", "/api/plan/999/torrents", []byte("x"), "", nil); code != http.StatusBadRequest {
		// The file is checked first.
		t.Fatalf("unknown film, invalid file: %d", code)
	}
	video, videoHash := torrentOf("Matrix.Video", 1000)
	audio, audioHash := torrentOf("Matrix.Audio", 200)
	var pt store.PlanTorrent
	if code := upload(t, h, "POST", base, video, "video", &pt); code != http.StatusCreated ||
		pt.Position != 1 || pt.Hash != videoHash || pt.Name != "Matrix.Video" || pt.Size != 1000 || pt.Label != "video" {
		t.Fatalf("upload: %d %+v", code, pt)
	}
	if code := upload(t, h, "POST", base, video, "", nil); code != http.StatusConflict {
		t.Fatalf("same torrent twice: %d", code)
	}
	if code := upload(t, h, "POST", base, audio, "audio ITA", &pt); code != http.StatusCreated || pt.Position != 2 {
		t.Fatalf("second: %d %+v", code, pt)
	}
	third, _ := torrentOf("Other", 1)
	if code := upload(t, h, "POST", base, third, "", nil); code != http.StatusConflict {
		t.Fatalf("third: %d", code)
	}
	if code := call(t, h, "PATCH", base+"/2", map[string]string{"label": "audio"}, &pt); code != http.StatusOK || pt.Label != "audio" {
		t.Fatalf("label: %d %+v", code, pt)
	}
	if code := call(t, h, "PATCH", base+"/3", map[string]string{"label": "x"}, nil); code != http.StatusBadRequest {
		t.Fatalf("position 3: %d", code)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", base+"/1/file", nil))
	if rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), video) ||
		!strings.Contains(rec.Header().Get("Content-Disposition"), "Matrix.Video.torrent") {
		t.Fatalf("file: %d %q", rec.Code, rec.Header())
	}

	// Sending: no client chosen nor by default.
	if code := call(t, h, "POST", base+"/send", planSendInput{}, &errOut); code != http.StatusBadRequest {
		t.Fatalf("no client: %d %v", code, errOut)
	}
	q := &fakeQbit{}
	qs := q.serve(t)
	c, err := st.CreateClient(ctx, store.Client{Name: "seedbox", URL: qs.URL, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if code := call(t, h, "PUT", "/api/settings", map[string]any{"planClientId": 999}, nil); code != http.StatusBadRequest {
		t.Fatalf("unknown default client: %d", code)
	}
	var set store.Settings
	if code := call(t, h, "PUT", "/api/settings", map[string]any{"planClientId": c.ID}, &set); code != http.StatusOK || set.PlanClientID != c.ID {
		t.Fatalf("default client: %d %+v", code, set)
	}

	// The default client, only the video, with the advanced options.
	var sent struct {
		Client  string           `json:"client"`
		Results []planSendResult `json:"results"`
	}
	if code := call(t, h, "POST", base+"/send", planSendInput{Positions: []int{1}, Category: " film ", Tags: "a, b"}, &sent); code != http.StatusOK ||
		sent.Client != "seedbox" || len(sent.Results) != 1 || !sent.Results[0].OK || sent.Results[0].Already {
		t.Fatalf("send: %d %+v", code, sent)
	}
	if len(q.added) != 1 || q.added[0] != videoHash || q.opts[0] != "film|a,b|" {
		t.Fatalf("on the client: %v %v", q.added, q.opts)
	}
	if got, _ := st.PlanEntry(ctx, e.ID); got.Status != store.PlanDoing {
		t.Fatalf("sent, not in progress: %+v", got)
	}
	if _, err := sy.SyncAll(ctx); err != nil {
		t.Fatal(err)
	}
	// Then all: the video is there already.
	if code := call(t, h, "POST", base+"/send", planSendInput{ClientID: c.ID}, &sent); code != http.StatusOK ||
		len(sent.Results) != 2 || !sent.Results[0].Already || !sent.Results[1].OK || sent.Results[1].Already {
		t.Fatalf("send all: %d %+v", code, sent)
	}
	if len(q.added) != 2 || q.added[1] != audioHash {
		t.Fatalf("on the client: %v", q.added)
	}
	if _, err := sy.SyncAll(ctx); err != nil {
		t.Fatal(err)
	}
	var list struct {
		Entries []struct {
			store.PlanEntry
			Torrents []store.PlanTorrent `json:"torrents"`
		} `json:"entries"`
	}
	if code := call(t, h, "GET", "/api/plan", nil, &list); code != http.StatusOK || len(list.Entries) != 1 {
		t.Fatalf("list: %d %+v", code, list)
	}
	it := list.Entries[0]
	if it.Status != store.PlanDownloaded || len(it.Torrents) != 2 || len(it.Torrents[1].Downloads) != 1 ||
		it.Torrents[1].Downloads[0].Progress != 1 || it.Torrents[1].SentClientName != "seedbox" {
		t.Fatalf("downloaded: %+v", it)
	}

	// A client that refuses.
	q.refuse = true
	if code := call(t, h, "PUT", base+"/1", nil, nil); code != http.StatusBadRequest {
		t.Fatalf("replace without a file: %d", code)
	}
	other, _ := torrentOf("Matrix.Video.v2", 1000)
	if code := upload(t, h, "PUT", base+"/1", other, "", &pt); code != http.StatusOK || pt.Position != 1 || pt.SentAt != nil {
		t.Fatalf("replace: %d %+v", code, pt)
	}
	if got, _ := st.PlanEntry(ctx, e.ID); got.Status != store.PlanDoing {
		t.Fatalf("replaced, not back in progress: %+v", got)
	}
	if code := call(t, h, "POST", base+"/send", planSendInput{Positions: []int{1}}, &errOut); code != http.StatusBadGateway ||
		!strings.Contains(errOut["error"], "seedbox") {
		t.Fatalf("refused: %d %v", code, errOut)
	}

	if code := call(t, h, "DELETE", base+"/2", nil, nil); code != http.StatusNoContent {
		t.Fatalf("delete: %d", code)
	}
	if code := call(t, h, "DELETE", base+"/2", nil, nil); code != http.StatusNotFound {
		t.Fatalf("delete twice: %d", code)
	}
}
