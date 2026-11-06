// Command demo runs bookrr with made-up data, for the screenshots of the
// documentation: two qBittorrent clients, a UNIT3D tracker, TMDB, MDBList,
// Prowlarr and an S3 bucket are faked in this process, and the data is
// entered through bookrr's own API. Every title, group and name is invented.
//
//	cd web && npm run build && cd .. && go run ./scripts/demo
//	node scripts/demo/screenshots.mjs   # in another shell, once "demo ready" is logged
//
// The data folder is a new temporary one each time (-data to keep it).
package main

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/tehmaat/bookrr/internal/api"
	"github.com/tehmaat/bookrr/internal/app"
	"github.com/tehmaat/bookrr/internal/config"
	"github.com/tehmaat/bookrr/internal/logs"
	"github.com/tehmaat/bookrr/internal/mdblist"
	"github.com/tehmaat/bookrr/internal/store"
	"github.com/tehmaat/bookrr/internal/tmdb"
	"github.com/tehmaat/bookrr/web"
)

const site = "https://tracker.example"

// release is a torrent on the fake tracker; mine are the user's uploads.
type release struct {
	id         int64
	name       string
	cat, typ   string
	res        string
	size       int64
	seeders    int64
	leechers   int64
	completed  int64
	tmdb       int64
	collection int64
	mine       bool
	created    string
}

func (r release) hash() string { return hashOf(r.name) }

func hashOf(name string) string {
	h := sha1.Sum([]byte("bookrr-demo:" + name))
	return hex.EncodeToString(h[:])
}

const gb = int64(1) << 30

var releases = []release{
	{1, "Starfall.Courier.2019.2160p.UHD.BluRay.REMUX.DV.HDR.HEVC.TrueHD.7.1.ITA.ENG-NOVA", "Movie", "Remux", "2160p", 61 * gb, 9, 0, 41, 9001, 7001, true, "2025-02-14T20:10:00Z"},
	{2, "The.Quiet.Orchard.2016.1080p.BluRay.REMUX.AVC.DTS-HD.MA.5.1.ITA.ENG-NOVA", "Movie", "Remux", "1080p", 29 * gb, 6, 1, 14, 9011, 0, true, "2025-04-02T18:30:00Z"},
	{3, "Paper.Lanterns.2021.1080p.AMZN.WEB-DL.DDP5.1.H.264.ITA.ENG-NOVA", "Movie", "WEB-DL", "1080p", 7 * gb, 1, 0, 3, 9021, 0, true, "2025-06-21T09:45:00Z"},
	{4, "Iron.Tide.2018.1080p.BluRay.x264.DTS.ITA.ENG-NOVA", "Movie", "Encode", "1080p", 14 * gb, 3, 0, 22, 9031, 7011, true, "2024-11-08T21:00:00Z"},
	{5, "Glass.Meridian.2022.2160p.NF.WEB-DL.DDP5.1.Atmos.DV.H.265.ITA.ENG-NOVA", "Movie", "WEB-DL", "2160p", 18 * gb, 0, 0, 7, 9041, 0, true, "2024-09-30T17:20:00Z"},
	{6, "Harbor.Lights.S01.2160p.ATVP.WEB-DL.DDP5.1.Atmos.DV.H.265.ITA.ENG-NOVA", "TV", "WEB-DL", "2160p", 48 * gb, 5, 0, 19, 8001, 0, true, "2025-01-12T19:00:00Z"},
	{7, "Harbor.Lights.S02.2160p.ATVP.WEB-DL.DDP5.1.Atmos.DV.H.265.ITA.ENG-NOVA", "TV", "WEB-DL", "2160p", 51 * gb, 4, 0, 12, 8001, 0, true, "2025-03-03T19:00:00Z"},
	{8, "Northbound.S01-S02.1080p.BluRay.REMUX.AVC.DTS-HD.MA.5.1.ITA.ENG-NOVA", "TV", "Remux", "1080p", 212 * gb, 2, 1, 6, 8011, 0, true, "2025-05-17T16:00:00Z"},
	{9, "Velvet.Static.S01.1080p.WEB-DL.DDP5.1.H.264.ITA.ENG-NOVA", "TV", "WEB-DL", "1080p", 21 * gb, 4, 0, 9, 8021, 0, true, "2025-07-08T20:40:00Z"},

	{21, "Starfall.Courier.II.2021.2160p.UHD.BluRay.REMUX.DV.HDR.HEVC.TrueHD.7.1.ITA.ENG-LUMEN", "Movie", "Remux", "2160p", 64 * gb, 11, 0, 30, 9002, 7001, false, "2025-03-01T10:00:00Z"},
	{22, "Starfall.Courier.III.2024.1080p.WEB-DL.DDP5.1.H.264.ITA.ENG-LUMEN", "Movie", "WEB-DL", "1080p", 8 * gb, 15, 2, 51, 9003, 7001, false, "2025-08-10T10:00:00Z"},
	{23, "Iron.Tide.Undertow.2020.1080p.BluRay.x264.DTS.ITA.ENG-YIFX", "Movie", "Encode", "1080p", 12 * gb, 7, 0, 18, 9032, 7011, false, "2025-01-20T10:00:00Z"},
	{24, "Harbor.Lights.S03.2160p.ATVP.WEB-DL.DDP5.1.Atmos.DV.H.265.ITA.ENG-LUMEN", "TV", "WEB-DL", "2160p", 50 * gb, 8, 1, 21, 8001, 0, false, "2025-05-05T10:00:00Z"},
	{25, "Harbor.Lights.S04.1080p.ATVP.WEB-DL.DDP5.1.H.264.ITA.ENG-LUMEN", "TV", "WEB-DL", "1080p", 17 * gb, 6, 0, 10, 8001, 0, false, "2025-10-01T10:00:00Z"},
	{26, "Velvet.Static.S02.1080p.WEB-DL.DDP5.1.H.264.ITA.ENG-KITE", "TV", "WEB-DL", "1080p", 22 * gb, 5, 0, 8, 8021, 0, false, "2025-08-15T10:00:00Z"},
	{27, "Neon.Run.2020.1080p.BluRay.REMUX.AVC.DTS-HD.MA.5.1.ITA.ENG-KITE", "Movie", "Remux", "1080p", 27 * gb, 4, 0, 16, 9051, 0, false, "2025-02-02T10:00:00Z"},
}

// TMDB: series with their seasons, films with their saga.
type season struct {
	n    int
	date string
}

type series struct {
	name    string
	seasons []season
}

type film struct {
	title, date string
	collection  int64
}

var tvShows = map[int64]series{
	8001: {"Harbor Lights", []season{{1, "2019-09-20"}, {2, "2021-07-16"}, {3, "2023-06-02"}, {4, "2025-08-29"}}},
	8011: {"Northbound", []season{{1, "2018-03-11"}, {2, "2019-04-07"}, {3, "2022-02-20"}}},
	8021: {"Velvet Static", []season{{1, "2020-10-02"}, {2, "2022-10-07"}}},
}

var films = map[int64]film{
	9001: {"Starfall Courier", "2019-05-10", 7001},
	9002: {"Starfall Courier II", "2021-06-18", 7001},
	9003: {"Starfall Courier III", "2024-03-01", 7001},
	9011: {"The Quiet Orchard", "2016-09-23", 0},
	9021: {"Paper Lanterns", "2021-11-12", 0},
	9031: {"Iron Tide", "2018-07-27", 7011},
	9032: {"Iron Tide: Undertow", "2020-08-14", 7011},
	9033: {"Iron Tide: Riptide", "2023-10-20", 7011},
	9041: {"Glass Meridian", "2022-04-08", 0},
	9051: {"Neon Run", "2020-02-21", 0},
	9052: {"Neon Run: Afterglow", "2023-12-01", 0},
	9061: {"Lantern Bay", "2019-03-15", 0},
	9071: {"The Long Thaw", "2027-02-12", 0},
}

var collections = map[int64]string{7001: "Starfall Collection", 7011: "Iron Tide Collection"}

// Prowlarr: the best releases its indexers have of each film.
var prowlarrResults = map[int64][]map[string]any{
	9011: {{"title": "The.Quiet.Orchard.2016.2160p.UHD.BluRay.REMUX.HDR.HEVC.DTS-HD.MA.5.1-ARCADE", "indexer": "Indexer A", "seeders": 12, "size": 58 * gb}},
	9021: {{"title": "Paper.Lanterns.2021.2160p.AMZN.WEB-DL.DDP5.1.HDR.H.265-ARCADE", "indexer": "Indexer B", "seeders": 31, "size": 15 * gb}},
	9031: {{"title": "Iron.Tide.2018.1080p.BluRay.REMUX.AVC.DTS-HD.MA.5.1-ARCADE", "indexer": "Indexer A", "seeders": 4, "size": 27 * gb}},
	9041: {{"title": "Glass.Meridian.2022.2160p.NF.WEB-DL.DDP5.1.Atmos.DV.H.265-ARCADE", "indexer": "Indexer B", "seeders": 22, "size": 17 * gb}},
	9002: {{"title": "Starfall.Courier.II.2021.2160p.UHD.BluRay.REMUX.DV.HDR.HEVC.TrueHD.7.1-ARCADE", "indexer": "Indexer A", "seeders": 9, "size": 66 * gb}},
	9033: {{"title": "Iron.Tide.Riptide.2023.2160p.WEB-DL.DDP5.1.HDR.H.265-ARCADE", "indexer": "Indexer B", "seeders": 40, "size": 16 * gb}},
	9061: {{"title": "Lantern.Bay.2019.1080p.BluRay.REMUX.AVC.DTS-HD.MA.5.1-ARCADE", "indexer": "Indexer A", "seeders": 6, "size": 25 * gb}},
	9052: {{"title": "Neon.Run.Afterglow.2023.2160p.WEB-DL.DDP5.1.DV.H.265-ARCADE", "indexer": "Indexer B", "seeders": 18, "size": 19 * gb}},
}

func main() {
	listen := flag.String("listen", "127.0.0.1:8080", "address of bookrr")
	data := flag.String("data", "", "data folder (default: a new temporary one)")
	flag.Parse()
	if *data == "" {
		dir, err := os.MkdirTemp("", "bookrr-demo-")
		if err != nil {
			panic(err)
		}
		*data = dir
	}

	qbit := newQbit()
	fakes := map[string]*httptest.Server{
		"seedbox":  httptest.NewServer(qbit.handler("seedbox")),
		"nas":      httptest.NewServer(qbit.handler("nas")),
		"tracker":  httptest.NewServer(http.HandlerFunc(serveTracker)),
		"tmdb":     httptest.NewServer(http.HandlerFunc(serveTMDB)),
		"mdblist":  httptest.NewServer(http.HandlerFunc(serveMDBList)),
		"prowlarr": httptest.NewServer(http.HandlerFunc(serveProwlarr)),
		"s3":       httptest.NewServer(http.HandlerFunc(serveS3)),
	}
	tmdb.DefaultBaseURL = fakes["tmdb"].URL
	mdblist.DefaultBaseURL = fakes["mdblist"].URL

	for k, v := range map[string]string{
		"BOOKRR_LISTEN": *listen, "BOOKRR_DATA_DIR": *data, "TZ": "Europe/Rome",
		"BOOKRR_UNIT3D_URL": fakes["tracker"].URL, "BOOKRR_UNIT3D_API_KEY": "demo",
		"BOOKRR_TMDB_API_KEY": "demo", "BOOKRR_MDBLIST_API_KEY": "demo",
		"BOOKRR_PROWLARR_URL": fakes["prowlarr"].URL, "BOOKRR_PROWLARR_API_KEY": "demo",
	} {
		os.Setenv(k, v)
	}
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	logFiles, err := logs.New(cfg.LogDir())
	if err != nil {
		panic(err)
	}
	logFiles.Configure(cfg.LogRetentionDays, cfg.LogMaxSize, cfg.LogLevel)
	slog.SetDefault(slog.New(logs.Handler(logFiles, os.Stderr)))
	st, err := store.Open(cfg.DBPath())
	if err != nil {
		panic(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runner := app.New(ctx, st)
	runner.Logs = logFiles
	svc := runner.Apply(cfg)
	server := api.New(cfg, st, svc.Syncer, svc.Buckets, svc.Hashes, svc.History, svc.Missing, svc.Best, web.FS(), "demo")
	server.SetServices(svc)
	server.Reconfigure = runner.Apply
	server.Logs = logFiles
	srv := &http.Server{Addr: cfg.Listen, Handler: server.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		srv.Shutdown(context.Background())
	}()
	go func() {
		seed(ctx, "http://"+*listen, fakes, qbit)
		// The daily searches would start minutes after the start: now.
		svc.Missing.Check(ctx)
		svc.Best.Check(ctx)
		slog.Info("demo ready", "url", "http://"+*listen, "data", *data)
	}()
	slog.Info("bookrr demo", "url", "http://"+*listen, "data", *data)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		panic(err)
	}
}

// seed enters the data through the API, as a user would.
func seed(ctx context.Context, base string, fakes map[string]*httptest.Server, qbit *qbitFake) {
	call := func(method, path string, body any) map[string]any {
		var r *bytes.Reader
		if s, ok := body.(string); ok {
			r = bytes.NewReader([]byte(s))
		} else {
			b, _ := json.Marshal(body)
			r = bytes.NewReader(b)
		}
		for {
			req, _ := http.NewRequestWithContext(ctx, method, base+path, r)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				time.Sleep(200 * time.Millisecond) // the server is starting
				r.Seek(0, 0)
				continue
			}
			defer resp.Body.Close()
			var out map[string]any
			json.NewDecoder(resp.Body).Decode(&out)
			if resp.StatusCode >= 300 {
				slog.Warn("demo seed", "path", path, "status", resp.StatusCode, "body", out)
			}
			return out
		}
	}
	id := func(m map[string]any) int64 { f, _ := m["id"].(float64); return int64(f) }

	call("POST", "/api/clients", map[string]any{"name": "seedbox", "url": fakes["seedbox"].URL, "username": "admin", "password": "demo", "enabled": true})
	call("POST", "/api/clients", map[string]any{"name": "nas", "url": fakes["nas"].URL, "username": "admin", "password": "demo", "enabled": true})

	orchid := id(call("POST", "/api/adopters", map[string]any{"name": "orchid", "contact": "tracker profile: orchid", "notes": "Keeps the series packs seeded."}))
	call("POST", "/api/adopters", map[string]any{"name": "kestrel", "contact": "@kestrel on Telegram"})

	smart := `=== START OF INFORMATION SECTION ===
Device Model:     WDC WD80EFZZ-68BTXN0
Serial Number:    WD-DEMO0001
Firmware Version: 81.00A81
User Capacity:    8,001,563,222,016 bytes [8.00 TB]
Rotation Rate:    5640 rpm
=== START OF READ SMART DATA SECTION ===
SMART overall-health self-assessment test result: PASSED
ID# ATTRIBUTE_NAME          FLAG     VALUE WORST THRESH TYPE      UPDATED  WHEN_FAILED RAW_VALUE
  9 Power_On_Hours          0x0032   078   078   000    Old_age   Always       -       19284
`
	archive01 := call("POST", "/api/disks/smart?label=Archive%2001", smart)
	a1 := int64(0)
	if d, ok := archive01["disk"].(map[string]any); ok {
		a1 = id(d)
		d["place"], d["notes"] = "Office shelf, top", "Movies 2018–2023"
		call("PUT", "/api/disks/"+strconv.FormatInt(a1, 10), d)
	}
	a2 := id(call("POST", "/api/disks", map[string]any{"label": "Archive 02", "kind": "USB", "serial": "S5DEMO0002", "model": "Samsung T7 2TB",
		"capacity": 2000 * 1000 * 1000 * 1000, "place": "Drawer 2"}))
	cloud := id(call("POST", "/api/disks", map[string]any{"label": "Cold Storage", "kind": "Cloud", "model": "Scaleway Object Storage",
		"s3": map[string]any{"endpoint": fakes["s3"].URL, "region": "fr-par", "bucket": "demo-releases", "prefix": "releases/", "accessKey": "SCWDEMO", "secretKey": "demo"}}))

	byName := map[string]release{}
	for _, r := range releases {
		byName[r.name] = r
	}
	add := func(name string, diskID int64, path string, adopter int64) {
		r := byName[name]
		in := map[string]any{"name": name, "hash": r.hash(), "size": r.size, "personalRelease": true, "tags": []string{"Personal Release"}}
		if diskID != 0 || path != "" {
			in["archive"] = map[string]any{"diskId": diskID, "path": path}
		}
		if adopter != 0 {
			in["adoption"] = map[string]any{"adopterId": adopter}
		}
		call("POST", "/api/torrents", in)
	}
	add("Iron.Tide.2018.1080p.BluRay.x264.DTS.ITA.ENG-NOVA", a1, "/Movies/2018/Iron.Tide.2018.1080p.BluRay.x264.DTS.ITA.ENG-NOVA", 0)
	add("Harbor.Lights.S02.2160p.ATVP.WEB-DL.DDP5.1.Atmos.DV.H.265.ITA.ENG-NOVA", a2, "/Series/Harbor.Lights", orchid)
	for _, t := range []struct {
		name string
		size int64
		disk int64
		path string
	}{
		{"Moonlit.Ferry.2017.1080p.BluRay.x264.DTS.ITA.ENG-NOVA", 11 * gb, a1, "/Movies/2017"},
		{"Saltwater.Choir.S01.1080p.WEB-DL.DDP5.1.H.264.ITA.ENG-NOVA", 19 * gb, a2, "/Series"},
	} {
		call("POST", "/api/torrents", map[string]any{"name": t.name, "hash": hashOf(t.name), "size": t.size, "personalRelease": true,
			"tags": []string{"Personal Release"}, "archive": map[string]any{"diskId": t.disk, "path": t.path}})
	}

	call("POST", "/api/sync", nil)
	// Copper Skies is still on the nas: recorded as moved before removing it.
	call("POST", "/api/torrents/"+hashOf("Copper.Skies.2020.2160p.WEB-DL.DDP5.1.HDR.H.265.ITA.ENG-NOVA")+"/archives", map[string]any{"diskId": a1, "path": "/Movies/2020"})
	// Harbor Lights S01 leaves the seedbox: bookrr asks where it went.
	qbit.remove("seedbox", byName["Harbor.Lights.S01.2160p.ATVP.WEB-DL.DDP5.1.Atmos.DV.H.265.ITA.ENG-NOVA"].hash())
	call("POST", "/api/sync", nil)
	// Another one left earlier and was recorded as moved.
	qbit.remove("nas", hashOf("Copper.Skies.2020.2160p.WEB-DL.DDP5.1.HDR.H.265.ITA.ENG-NOVA"))

	call("PUT", "/api/missing/banned-groups", map[string]any{"groups": []string{"YIFX"}})
	call("POST", "/api/missing/lists/sources", map[string]any{"url": "https://mdblist.com/lists/demo/neon-cinema"})
	call("POST", "/api/releases/sync", nil)
	call("POST", "/api/plan", map[string]any{"tmdb": "9061", "note": "Italian audio from the 2019 DVD"})
	call("POST", "/api/plan", map[string]any{"tmdb": "9071"})
	call("POST", "/api/plan", map[string]any{"tmdb": "9033", "source": "saga:7011", "sourceTitle": "Iron Tide Collection"})
	call("POST", "/api/plan", map[string]any{"tmdb": "9002", "source": "saga:7001", "sourceTitle": "Starfall Collection"})
	// Wait for the tracker: its requests are spaced out.
	for i := 0; i < 120; i++ {
		var rs struct {
			State    struct{ Running bool } `json:"state"`
			Releases []any                  `json:"releases"`
		}
		b, _ := json.Marshal(call("GET", "/api/releases", nil))
		json.Unmarshal(b, &rs)
		if !rs.State.Running && len(rs.Releases) > 0 {
			break
		}
		time.Sleep(time.Second)
	}
	call("POST", "/api/sync", nil)
	call("POST", "/api/disks/"+strconv.FormatInt(cloud, 10)+"/scan", nil)
}

// qbitFake serves the torrents of two qBittorrent clients.
type qbitFake struct {
	mu       sync.Mutex
	torrents map[string][]map[string]any
}

func newQbit() *qbitFake {
	q := &qbitFake{torrents: map[string][]map[string]any{}}
	added := time.Date(2025, 3, 1, 20, 0, 0, 0, time.UTC).Unix()
	put := func(client, name, state string, progress, ratio float64, size int64, tags, category string) {
		q.torrents[client] = append(q.torrents[client], map[string]any{
			"hash": hashOf(name), "name": name, "size": size, "total_size": size, "tags": tags, "category": category,
			"save_path": "/downloads/" + category, "state": state, "progress": progress, "ratio": ratio,
			"added_on": added, "tracker": site + "/announce/demo",
		})
		added += 86400 * 3
	}
	pr := "Personal Release"
	for _, r := range releases {
		if !r.mine {
			continue
		}
		cat := "movies"
		if r.cat == "TV" {
			cat = "tv"
		}
		switch r.id {
		case 1:
			put("seedbox", r.name, "uploading", 1, 3.2, r.size, pr, cat)
			put("nas", r.name, "stalledUP", 1, 0.4, r.size, pr, cat)
		case 2, 6, 8:
			put("seedbox", r.name, "uploading", 1, 1.8, r.size, pr, cat)
		case 3, 9:
			put("nas", r.name, "stalledUP", 1, 0.9, r.size, pr, cat)
		}
	}
	put("nas", "Copper.Skies.2020.2160p.WEB-DL.DDP5.1.HDR.H.265.ITA.ENG-NOVA", "uploading", 1, 2.1, 16*gb, pr, "movies")
	put("seedbox", "Open.Mic.Night.Live.2024.FLAC", "stalledUP", 1, 0.6, gb/2, "", "music")
	put("seedbox", "Community.Radio.Archive.Vol.3.FLAC", "downloading", 0.42, 0, 3*gb, "", "music")
	put("nas", "Community.Radio.Archive.Vol.3.FLAC", "uploading", 1, 1.1, 3*gb, "", "music")
	return q
}

func (q *qbitFake) remove(client, hash string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	list := q.torrents[client]
	for i, t := range list {
		if t["hash"] == hash {
			q.torrents[client] = append(list[:i:i], list[i+1:]...)
			return
		}
	}
}

func (q *qbitFake) handler(client string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "demo"})
			w.Write([]byte("Ok."))
		case "/api/v2/app/version":
			w.Write([]byte(`"v5.1.0"`))
		case "/api/v2/torrents/info":
			q.mu.Lock()
			defer q.mu.Unlock()
			json.NewEncoder(w).Encode(q.torrents[client])
		default:
			w.WriteHeader(http.StatusOK)
		}
	})
}

func serveTracker(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/user" {
		w.Write([]byte(`{"username":"demo"}`))
		return
	}
	if !strings.HasPrefix(r.URL.Path, "/api/torrents") {
		http.NotFound(w, r)
		return
	}
	q := r.URL.Query()
	var data []any
	for _, t := range releases {
		switch {
		case q.Get("uploader") != "" && !t.mine,
			q.Get("tmdbId") != "" && q.Get("tmdbId") != strconv.FormatInt(t.tmdb, 10),
			q.Get("collectionId") != "" && q.Get("collectionId") != strconv.FormatInt(t.collection, 10),
			q.Get("name") != "" && !strings.Contains(strings.ToLower(t.name), strings.ToLower(strings.ReplaceAll(q.Get("name"), " ", "."))):
			continue
		}
		a := map[string]any{"name": t.name, "category": t.cat, "type": t.typ, "resolution": t.res, "size": t.size,
			"seeders": t.seeders, "leechers": t.leechers, "times_completed": t.completed, "tmdb_id": t.tmdb,
			"personal_release": t.mine, "created_at": t.created, "info_hash": t.hash(),
			"details_link": site + "/torrents/" + strconv.FormatInt(t.id, 10)}
		if t.cat == "Movie" {
			a["imdb_id"] = 9100000 + t.tmdb
		}
		data = append(data, map[string]any{"id": t.id, "attributes": a})
	}
	if data == nil {
		data = []any{}
	}
	json.NewEncoder(w).Encode(map[string]any{"data": data})
}

func serveTMDB(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	n, _ := strconv.ParseInt(strings.TrimPrefix(parts[1], "tt"), 10, 64)
	var out any
	switch parts[0] {
	case "tv":
		s, ok := tvShows[n]
		if !ok {
			break
		}
		var seasons []map[string]any
		for _, x := range s.seasons {
			seasons = append(seasons, map[string]any{"season_number": x.n, "name": fmt.Sprintf("Season %d", x.n), "air_date": x.date, "episode_count": 8})
		}
		out = map[string]any{"id": n, "name": s.name, "original_name": s.name, "first_air_date": s.seasons[0].date, "status": "Returning Series", "seasons": seasons}
	case "movie":
		f, ok := films[n]
		if !ok {
			break
		}
		m := map[string]any{"id": n, "title": f.title, "original_title": f.title, "release_date": f.date, "imdb_id": fmt.Sprintf("tt%07d", 9100000+n)}
		if f.collection != 0 {
			m["belongs_to_collection"] = map[string]any{"id": f.collection, "name": collections[f.collection]}
		}
		out = m
	case "collection":
		name, ok := collections[n]
		if !ok {
			break
		}
		var ps []map[string]any
		for id := int64(9001); id <= 9099; id++ {
			if f, ok := films[id]; ok && f.collection == n {
				ps = append(ps, map[string]any{"id": id, "title": f.title, "release_date": f.date})
			}
		}
		out = map[string]any{"id": n, "name": name, "parts": ps}
	case "find":
		id := n - 9100000
		var movies []any
		if _, ok := films[id]; ok {
			movies = append(movies, map[string]any{"id": id})
		}
		out = map[string]any{"movie_results": movies, "tv_results": []any{}}
	}
	if out == nil {
		http.NotFound(w, r)
		return
	}
	json.NewEncoder(w).Encode(out)
}

func serveMDBList(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/lists/demo/neon-cinema":
		w.Write([]byte(`[{"id":4242,"name":"Neon Cinema","slug":"neon-cinema","items":4,"user_name":"demo"}]`))
	case "/lists/demo/neon-cinema/items/movie":
		var items []map[string]any
		for i, id := range []int64{9051, 9052, 9011, 9021} {
			items = append(items, map[string]any{"id": id, "imdb_id": fmt.Sprintf("tt%07d", 9100000+id), "title": films[id].title, "mediatype": "movie", "rank": i + 1})
		}
		json.NewEncoder(w).Encode(items)
	default:
		http.NotFound(w, r)
	}
}

func serveProwlarr(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/v1/system/status" {
		w.Write([]byte(`{"version":"1.30.2"}`))
		return
	}
	q := r.URL.Query().Get("query")
	var out []map[string]any
	for id, rs := range prowlarrResults {
		if strings.Contains(q, fmt.Sprintf("{TmdbId:%d}", id)) {
			for _, x := range rs {
				y := map[string]any{"tmdbId": id, "imdbId": 9100000 + id, "infoUrl": "https://indexer.example/details/" + strconv.FormatInt(id, 10)}
				for k, v := range x {
					y[k] = v
				}
				out = append(out, y)
			}
		}
	}
	if out == nil {
		out = []map[string]any{}
	}
	json.NewEncoder(w).Encode(out)
}

// serveS3 lists the bucket of the cloud archive.
func serveS3(w http.ResponseWriter, r *http.Request) {
	keys := []struct {
		key  string
		size int64
	}{
		{"releases/Velvet.Static.S01.1080p.WEB-DL.DDP5.1.H.264.ITA.ENG-NOVA/Velvet.Static.S01E01.mkv", 2 * gb},
		{"releases/Velvet.Static.S01.1080p.WEB-DL.DDP5.1.H.264.ITA.ENG-NOVA/Velvet.Static.S01E02.mkv", 2 * gb},
		{"releases/Paper.Lanterns.2021.1080p.AMZN.WEB-DL.DDP5.1.H.264.ITA.ENG-NOVA.mkv", 7 * gb},
		{"releases/Old.Demo.Reel.2015.1080p.WEB-DL.AAC.ITA-NOVA/Old.Demo.Reel.mkv", 3 * gb},
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><ListBucketResult><IsTruncated>false</IsTruncated>`)
	for _, k := range keys {
		fmt.Fprintf(&b, "<Contents><Key>%s</Key><Size>%d</Size><LastModified>2025-09-01T08:00:00.000Z</LastModified></Contents>", k.key, k.size)
	}
	b.WriteString(`</ListBucketResult>`)
	w.Header().Set("Content-Type", "application/xml")
	w.Write([]byte(b.String()))
}
