package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/tehmaat/bookrr/internal/arr"
	"github.com/tehmaat/bookrr/internal/config"
	"github.com/tehmaat/bookrr/internal/listing"
	"github.com/tehmaat/bookrr/internal/store"
)

// The listing of a disk: the files on it, as printed by ls, find, dir or
// Get-ChildItem on the computer it is plugged into. bookrr finds the
// torrents stored there and, once confirmed, records them as archived on
// the disk, removes them from the clients and deletes them from Radarr and
// Sonarr.

type listingRequest struct {
	Listing string `json:"listing"`
	// Put before the paths found, e.g. the folder of the disk listed.
	Prefix string `json:"prefix"`
}

type listingLocation struct {
	ClientID   int64   `json:"clientId"`
	ClientName string  `json:"clientName"`
	SavePath   string  `json:"savePath"`
	Progress   float64 `json:"progress"`
}

type listingItem struct {
	Hash            string `json:"hash"`
	Name            string `json:"name"`
	PersonalRelease bool   `json:"personalRelease"`
	// The torrent's size and the one found on the disk.
	Size     int64 `json:"size"`
	DiskSize int64 `json:"diskSize"`
	Files    int   `json:"files"`
	// complete, incomplete, larger or unknown (see listing.Compare).
	Check string `json:"check"`
	// Where it is on the disk, as recorded in the archive.
	Path string `json:"path"`
	// Already recorded as archived on this disk.
	Archived  bool              `json:"archived"`
	Locations []listingLocation `json:"locations"`
	Radarr    *arr.Movie        `json:"radarr"`
	Sonarr    *arr.Episodes     `json:"sonarr"`
}

type listingOther struct {
	Path  string `json:"path"`
	Name  string `json:"name"`
	Files int    `json:"files"`
	Size  int64  `json:"size"`
	Sized bool   `json:"sized"`
}

type listingPreview struct {
	Format    string         `json:"format"`
	Root      string         `json:"root"`
	Files     int            `json:"files"`
	Size      int64          `json:"size"`
	Sized     bool           `json:"sized"`
	Items     []listingItem  `json:"items"`
	Unmatched []listingOther `json:"unmatched"`
	// Configured; the error when they could not be read.
	Radarr      bool   `json:"radarr"`
	Sonarr      bool   `json:"sonarr"`
	RadarrError string `json:"radarrError"`
	SonarrError string `json:"sonarrError"`
}

// arrs returns the Radarr and Sonarr configured (nil when not).
func arrs(cfg config.Config) (*arr.Radarr, *arr.Sonarr) {
	var r *arr.Radarr
	var s *arr.Sonarr
	if cfg.RadarrURL != "" {
		r = arr.NewRadarr(cfg.RadarrURL, cfg.RadarrAPIKey)
	}
	if cfg.SonarrURL != "" {
		s = arr.NewSonarr(cfg.SonarrURL, cfg.SonarrAPIKey)
	}
	return r, s
}

// listingDisk returns the disk of the request, refusing the cloud archives:
// those are read from their bucket.
func (s *Server) listingDisk(w http.ResponseWriter, r *http.Request) (store.Disk, bool) {
	id, ok := pathID(w, r)
	if !ok {
		return store.Disk{}, false
	}
	d, err := s.store.GetDisk(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return d, false
	}
	if d.S3 != nil {
		writeError(w, http.StatusBadRequest, "this archive is read from its S3 bucket")
		return d, false
	}
	return d, true
}

// analyzeListing finds the torrents of the listing, with where they are now
// and what Radarr and Sonarr hold of them.
func (s *Server) analyzeListing(ctx context.Context, d store.Disk, in listingRequest) (listingPreview, error) {
	var p listingPreview
	l, err := listing.Parse(in.Listing)
	if err != nil {
		return p, err
	}
	p.Format, p.Root, p.Sized = l.Format, l.Root, true
	for _, e := range l.Entries {
		if !e.Dir {
			p.Files++
			p.Size += e.Size
			p.Sized = p.Sized && e.Sized
		}
	}
	names, err := s.store.TorrentNames(ctx)
	if err != nil {
		return p, err
	}
	found, others := listing.Match(l, names)
	ts, err := s.store.ListTorrents(ctx)
	if err != nil {
		return p, err
	}
	byHash := make(map[string]store.Torrent, len(ts))
	for _, t := range ts {
		byHash[t.Hash] = t
	}
	prefix := strings.Trim(strings.ReplaceAll(strings.TrimSpace(in.Prefix), "\\", "/"), "/")
	seen := map[string]bool{}
	for _, f := range found {
		t, ok := byHash[f.Hash]
		if !ok || seen[f.Hash] {
			continue
		}
		seen[f.Hash] = true
		it := listingItem{Hash: t.Hash, Name: t.Name, PersonalRelease: t.PersonalRelease, Size: t.Size,
			DiskSize: f.Size, Files: f.Files, Check: f.Compare(t.Size), Path: f.Path, Locations: []listingLocation{}}
		if prefix != "" {
			it.Path = prefix + "/" + f.Path
		}
		for _, a := range t.Archives {
			if a.DiskID != nil && *a.DiskID == d.ID {
				it.Archived = true
			}
		}
		for _, loc := range t.Locations {
			it.Locations = append(it.Locations, listingLocation{ClientID: loc.ClientID, ClientName: loc.ClientName, SavePath: loc.SavePath, Progress: loc.Progress})
		}
		p.Items = append(p.Items, it)
	}
	sort.Slice(p.Items, func(i, j int) bool { return strings.ToLower(p.Items[i].Path) < strings.ToLower(p.Items[j].Path) })
	p.Unmatched = make([]listingOther, 0, len(others))
	for _, o := range others {
		p.Unmatched = append(p.Unmatched, listingOther{Path: o.Path, Name: o.Name, Files: o.Files, Size: o.Size, Sized: o.Sized})
	}

	radarr, sonarr := arrs(s.current().Config)
	p.Radarr, p.Sonarr = radarr != nil, sonarr != nil
	if radarr != nil {
		look := radarr.Lookup()
		for i := range p.Items {
			m, err := look.Find(ctx, p.Items[i].Hash, p.Items[i].Name)
			if err != nil {
				p.RadarrError = err.Error()
				break
			}
			p.Items[i].Radarr = m
		}
	}
	if sonarr != nil {
		for i := range p.Items {
			e, err := sonarr.Find(ctx, p.Items[i].Hash)
			if err != nil {
				p.SonarrError = err.Error()
				break
			}
			p.Items[i].Sonarr = e
		}
	}
	if p.Items == nil {
		p.Items = []listingItem{}
	}
	return p, nil
}

func (s *Server) previewListing(w http.ResponseWriter, r *http.Request) {
	d, ok := s.listingDisk(w, r)
	if !ok {
		return
	}
	var in listingRequest
	if !decodeLimit(w, r, &in, 64<<20) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	p, err := s.analyzeListing(ctx, d, in)
	if errors.Is(err, listing.ErrEmpty) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, p)
}

// listingStep is one thing done (or not) for a torrent.
type listingStep struct {
	What    string `json:"what"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

type listingResult struct {
	Hash  string        `json:"hash"`
	Name  string        `json:"name"`
	Steps []listingStep `json:"steps"`
}

func (r *listingResult) add(what string, err error, msg string) bool {
	if err != nil {
		r.Steps = append(r.Steps, listingStep{What: what, Message: err.Error()})
		return false
	}
	r.Steps = append(r.Steps, listingStep{What: what, OK: true, Message: msg})
	return true
}

// applyListing does, for the torrents chosen, what the preview showed:
// records them as archived on the disk, then removes them from every client
// they are on and deletes them from Radarr and Sonarr. The listing is read
// again, so nothing is done to a torrent whose copy on the disk is smaller.
func (s *Server) applyListing(w http.ResponseWriter, r *http.Request) {
	d, ok := s.listingDisk(w, r)
	if !ok {
		return
	}
	var in struct {
		listingRequest
		Items []struct {
			Hash    string `json:"hash"`
			Clients bool   `json:"clients"`
			Radarr  bool   `json:"radarr"`
			Sonarr  bool   `json:"sonarr"`
		} `json:"items"`
		// Delete the files on the clients too: always chosen by the user.
		DeleteFiles *bool `json:"deleteFiles"`
	}
	if !decodeLimit(w, r, &in, 64<<20) {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 15*time.Minute)
	defer cancel()
	p, err := s.analyzeListing(ctx, d, in.listingRequest)
	if errors.Is(err, listing.ErrEmpty) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	items := map[string]listingItem{}
	for _, it := range p.Items {
		items[it.Hash] = it
	}
	for _, want := range in.Items {
		if want.Clients && len(items[want.Hash].Locations) > 0 && in.DeleteFiles == nil {
			writeError(w, http.StatusBadRequest, "choose whether to delete the files on the clients too")
			return
		}
	}
	svc := s.current()
	radarr, sonarr := arrs(svc.Config)
	deleted := map[int64]error{} // films of Radarr already deleted
	type seriesWork struct {
		removal arr.SeriesRemoval
		files   map[int64]bool
		unmon   map[int64]bool
		results []*listingResult
	}
	series := map[int64]*seriesWork{}
	results := make([]*listingResult, 0, len(in.Items))

	for _, want := range in.Items {
		res := &listingResult{Hash: want.Hash}
		results = append(results, res)
		it, ok := items[want.Hash]
		if !ok {
			res.add("Disk", errors.New("not found in the listing"), "")
			continue
		}
		res.Name = it.Name
		if it.Check == listing.CheckIncomplete {
			res.add("Disk", errors.New("the copy on the disk is smaller than the torrent: nothing done"), "")
			continue
		}
		if it.Archived {
			res.add("Archive", nil, "already on "+d.Label)
		} else if !res.add("Archive", s.store.AddArchive(ctx, it.Hash, store.ArchiveInput{DiskID: &d.ID, Path: it.Path}), "recorded on "+d.Label+" · "+it.Path) {
			continue
		}
		onClients := false
		if want.Clients {
			for _, loc := range it.Locations {
				if !res.add(loc.ClientName, s.removeTorrent(ctx, svc, it.Hash, loc.ClientID, *in.DeleteFiles), removedMsg(*in.DeleteFiles)) {
					onClients = true
				}
			}
		}
		if onClients {
			if want.Radarr && it.Radarr != nil || want.Sonarr && it.Sonarr != nil {
				res.add("Radarr/Sonarr", errors.New("skipped: the torrent is still on a client"), "")
			}
			continue
		}
		if want.Radarr {
			switch m := it.Radarr; {
			case radarr == nil:
				// Not configured: nothing to do.
			case p.RadarrError != "":
				res.add("Radarr", errors.New(p.RadarrError), "")
			case m == nil:
				// Not in Radarr: nothing to do.
			case !m.Deletable():
				res.add("Radarr", errors.New("Radarr has another file of this film now (upgraded?): left as it is"), "")
			default:
				err, done := deleted[m.ID]
				if !done {
					err = radarr.DeleteMovie(ctx, m.ID)
					deleted[m.ID] = err
					if err == nil {
						slog.Info("film deleted from Radarr", "title", m.Title, "year", m.Year, "torrent", it.Name)
					}
				}
				res.add("Radarr", err, fmt.Sprintf("%s (%d) deleted with its files", m.Title, m.Year))
			}
		}
		if want.Sonarr {
			switch e := it.Sonarr; {
			case sonarr == nil:
				// Not configured: nothing to do.
			case p.SonarrError != "":
				res.add("Sonarr", errors.New(p.SonarrError), "")
			case e == nil:
				// Not in Sonarr: nothing to do.
			default:
				sw := series[e.SeriesID]
				if sw == nil {
					sw = &seriesWork{removal: arr.SeriesRemoval{SeriesID: e.SeriesID, SeriesFiles: e.SeriesFiles}, files: map[int64]bool{}, unmon: map[int64]bool{}}
					series[e.SeriesID] = sw
				}
				for _, f := range e.Files {
					if !sw.files[f] {
						sw.files[f] = true
						sw.removal.Files = append(sw.removal.Files, f)
					}
				}
				for _, id := range e.Unmonitor {
					if !sw.unmon[id] {
						sw.unmon[id] = true
						sw.removal.Unmonitor = append(sw.removal.Unmonitor, id)
					}
				}
				sw.results = append(sw.results, res)
			}
		}
	}
	ids := make([]int64, 0, len(series))
	for id := range series {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		sw := series[id]
		whole, err := sonarr.Remove(ctx, sw.removal)
		msg := fmt.Sprintf("%d episode files deleted, %d episodes no longer monitored", len(sw.removal.Files), len(sw.removal.Unmonitor))
		if whole {
			msg = "series deleted with its files: none of them was left"
		}
		if err == nil {
			slog.Info("episodes deleted from Sonarr", "series", id, "files", len(sw.removal.Files), "unmonitored", len(sw.removal.Unmonitor), "seriesDeleted", whole)
		}
		for _, res := range sw.results {
			res.add("Sonarr", err, msg)
		}
	}
	slog.Info("disk listing applied", "disk", d.Label, "torrents", len(results))
	writeJSON(w, 200, map[string]any{"results": results})
}

func removedMsg(deleteFiles bool) string {
	if deleteFiles {
		return "removed with its files"
	}
	return "removed, files kept"
}

// removeTorrent deletes the torrent from a client and forgets it is there.
func (s *Server) removeTorrent(ctx context.Context, svc Services, hash string, clientID int64, deleteFiles bool) error {
	if svc.Syncer == nil {
		return errors.New("the clients are not being read")
	}
	c, err := s.store.GetClient(ctx, clientID)
	if err != nil {
		return err
	}
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := svc.Syncer.Qbit(c).DeleteTorrent(cctx, hash, deleteFiles); err != nil {
		return fmt.Errorf("removal failed: %w", err)
	}
	if err := s.store.RemoveLocation(ctx, hash, clientID); err != nil {
		return err
	}
	slog.Info("torrent removed from the client after archiving", "hash", hash, "client", c.Name, "deleteFiles", deleteFiles)
	return nil
}
