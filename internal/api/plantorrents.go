package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/tehmaat/bookrr/internal/qbit"
	"github.com/tehmaat/bookrr/internal/store"
	"github.com/tehmaat/bookrr/internal/unit3d"
)

// maxTorrentFile is the largest .torrent file taken for the plan.
const maxTorrentFile = 10 << 20

// pathPosition reads the position of a .torrent of the plan (1 or 2) from the path.
func pathPosition(w http.ResponseWriter, r *http.Request) (int, bool) {
	p, err := strconv.Atoi(r.PathValue("position"))
	if err != nil || p < 1 || p > store.MaxPlanTorrents {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid position: 1 to %d", store.MaxPlanTorrents))
		return 0, false
	}
	return p, true
}

// readTorrentUpload reads the .torrent file sent as a multipart form
// (field "file", optional "label").
func readTorrentUpload(w http.ResponseWriter, r *http.Request) (store.PlanTorrentInput, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxTorrentFile+1<<20)
	if err := r.ParseMultipartForm(maxTorrentFile + 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "send the .torrent file as a form, at most 10 MB")
		return store.PlanTorrentInput{}, false
	}
	defer r.MultipartForm.RemoveAll()
	f, h, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "no .torrent file sent")
		return store.PlanTorrentInput{}, false
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxTorrentFile+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "reading the file: "+err.Error())
		return store.PlanTorrentInput{}, false
	}
	if len(data) > maxTorrentFile {
		writeError(w, http.StatusBadRequest, "the .torrent file is over 10 MB")
		return store.PlanTorrentInput{}, false
	}
	meta, err := unit3d.ReadTorrent(data)
	if err != nil {
		writeError(w, http.StatusBadRequest, "not a valid .torrent file: "+err.Error())
		return store.PlanTorrentInput{}, false
	}
	label := strings.TrimSpace(r.FormValue("label"))
	if len(label) > store.MaxPlanTorrentLabel {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("label too long: at most %d characters", store.MaxPlanTorrentLabel))
		return store.PlanTorrentInput{}, false
	}
	name := meta.Name
	if name == "" {
		name = strings.TrimSuffix(h.Filename, ".torrent")
	}
	return store.PlanTorrentInput{Hash: meta.Hash, Name: name, Size: meta.Size, Label: label, Data: data}, true
}

func (s *Server) failPlanTorrent(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrPlanTorrentsFull) || errors.Is(err, store.ErrPlanTorrentDup) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	s.fail(w, err)
}

// addPlanTorrent loads a .torrent file on a film of the plan, at the first
// free position (two at most, for a merge).
func (s *Server) addPlanTorrent(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	in, ok := readTorrentUpload(w, r)
	if !ok {
		return
	}
	t, err := s.store.SetPlanTorrent(r.Context(), id, 0, in)
	if err != nil {
		s.failPlanTorrent(w, err)
		return
	}
	slog.Info("torrent loaded on the plan", "plan", id, "position", t.Position, "hash", t.Hash, "name", t.Name)
	writeJSON(w, http.StatusCreated, t)
}

// replacePlanTorrent replaces a .torrent of a film of the plan.
func (s *Server) replacePlanTorrent(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	pos, ok := pathPosition(w, r)
	if !ok {
		return
	}
	in, ok := readTorrentUpload(w, r)
	if !ok {
		return
	}
	t, err := s.store.SetPlanTorrent(r.Context(), id, pos, in)
	if err != nil {
		s.failPlanTorrent(w, err)
		return
	}
	slog.Info("torrent replaced on the plan", "plan", id, "position", pos, "hash", t.Hash, "name", t.Name)
	writeJSON(w, http.StatusOK, t)
}

// updatePlanTorrent changes the label of a .torrent of the plan.
func (s *Server) updatePlanTorrent(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	pos, ok := pathPosition(w, r)
	if !ok {
		return
	}
	var in struct {
		Label string `json:"label"`
	}
	if !decode(w, r, &in) {
		return
	}
	if len(strings.TrimSpace(in.Label)) > store.MaxPlanTorrentLabel {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("label too long: at most %d characters", store.MaxPlanTorrentLabel))
		return
	}
	t, err := s.store.SetPlanTorrentLabel(r.Context(), id, pos, in.Label)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// deletePlanTorrent removes a .torrent from a film of the plan (not from the clients).
func (s *Server) deletePlanTorrent(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	pos, ok := pathPosition(w, r)
	if !ok {
		return
	}
	if err := s.store.DeletePlanTorrent(r.Context(), id, pos); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// planTorrentFile downloads a .torrent of the plan.
func (s *Server) planTorrentFile(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	pos, ok := pathPosition(w, r)
	if !ok {
		return
	}
	t, err := s.store.PlanTorrent(r.Context(), id, pos)
	if err != nil {
		s.fail(w, err)
		return
	}
	data, err := s.store.PlanTorrentFile(r.Context(), id, pos)
	if err != nil {
		s.fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/x-bittorrent")
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(torrentFilename(t)))
	w.Write(data)
}

// torrentFilename is the name of the .torrent file of t, without slashes.
func torrentFilename(t store.PlanTorrent) string {
	name := strings.NewReplacer("/", "_", "\\", "_").Replace(t.Name)
	if name == "" {
		name = t.Hash
	}
	return name + ".torrent"
}

type planSendInput struct {
	// 0 for the default client of the plan, from the settings.
	ClientID int64 `json:"clientId"`
	// The positions of the torrents to send; none for all of them.
	Positions []int `json:"positions"`
	// Empty ones leave the defaults of the client.
	Category string `json:"category"`
	Tags     string `json:"tags"`
	SavePath string `json:"savePath"`
}

type planSendResult struct {
	Position int    `json:"position"`
	Name     string `json:"name"`
	OK       bool   `json:"ok"`
	// It was on the client already: not sent again.
	Already bool   `json:"already"`
	Error   string `json:"error"`
}

// sendPlanTorrents sends the .torrent files of a film of the plan to a
// client, to download them; the film is then in progress.
func (s *Server) sendPlanTorrents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in planSendInput
	if !decode(w, r, &in) {
		return
	}
	svc := s.current()
	if svc.Syncer == nil {
		writeError(w, http.StatusServiceUnavailable, "the clients are not available")
		return
	}
	if _, err := s.store.PlanEntry(ctx, id); err != nil {
		s.fail(w, err)
		return
	}
	if in.ClientID == 0 {
		st, err := s.store.GetSettings(ctx)
		if err != nil {
			s.fail(w, err)
			return
		}
		in.ClientID = st.PlanClientID
	}
	if in.ClientID == 0 {
		writeError(w, http.StatusBadRequest, "choose the client to send it to")
		return
	}
	c, err := s.store.GetClient(ctx, in.ClientID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusBadRequest, "the client no longer exists: choose another one")
		return
	}
	if err != nil {
		s.fail(w, err)
		return
	}
	if !c.Enabled {
		writeError(w, http.StatusBadRequest, "the client "+c.Name+" is disabled")
		return
	}
	torrents, err := s.store.PlanTorrentsOf(ctx, id)
	if err != nil {
		s.fail(w, err)
		return
	}
	wanted := map[int]bool{}
	for _, p := range in.Positions {
		wanted[p] = true
	}
	var send []store.PlanTorrent
	for _, t := range torrents {
		if len(wanted) == 0 || wanted[t.Position] {
			send = append(send, t)
		}
	}
	if len(send) == 0 {
		writeError(w, http.StatusBadRequest, "load a .torrent file on the film first")
		return
	}
	opts := qbit.AddOptions{
		Category: strings.TrimSpace(in.Category),
		Tags:     store.NormalizeTags(in.Tags),
		SavePath: strings.TrimSpace(in.SavePath),
	}
	q := svc.Syncer.Qbit(c)
	results := make([]planSendResult, 0, len(send))
	sent := 0
	var errs []string
	for _, t := range send {
		res := planSendResult{Position: t.Position, Name: t.Name}
		for _, d := range t.Downloads {
			if d.ClientID == c.ID {
				res.Already = true
			}
		}
		if !res.Already {
			data, err := s.store.PlanTorrentFile(ctx, id, t.Position)
			if err != nil {
				s.fail(w, err)
				return
			}
			actx, cancel := context.WithTimeout(ctx, 30*time.Second)
			err = q.AddTorrent(actx, torrentFilename(t), data, opts)
			cancel()
			if err != nil {
				res.Error = err.Error()
				errs = append(errs, err.Error())
				slog.Warn("sending a plan torrent failed", "plan", id, "position", t.Position, "client", c.Name, "err", err)
				results = append(results, res)
				continue
			}
		}
		if err := s.store.MarkPlanTorrentSent(ctx, id, t.Position, c.ID); err != nil {
			s.fail(w, err)
			return
		}
		res.OK = true
		sent++
		slog.Info("plan torrent sent", "plan", id, "position", t.Position, "hash", t.Hash, "client", c.Name, "already", res.Already)
		results = append(results, res)
	}
	if sent == 0 {
		writeError(w, http.StatusBadGateway, "sending to "+c.Name+" failed: "+strings.Join(errs, "; "))
		return
	}
	// Read the clients soon, to show how the download goes: give the
	// client a moment to list the torrent.
	sy := svc.Syncer
	go func() {
		time.Sleep(2 * time.Second)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if _, err := sy.SyncAll(ctx); err != nil {
			slog.Error("sync failed", "err", err)
		}
	}()
	writeJSON(w, http.StatusOK, map[string]any{"client": c.Name, "results": results})
}
