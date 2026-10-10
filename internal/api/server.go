// Package api exposes the bookrr REST API, the qBittorrent webhook and the web UI.
package api

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tehmaat/bookrr/internal/config"
	"github.com/tehmaat/bookrr/internal/qbit"
	"github.com/tehmaat/bookrr/internal/store"
	"github.com/tehmaat/bookrr/internal/syncer"
)

type Server struct {
	cfg     config.Config
	store   *store.Store
	syncer  *syncer.Syncer
	web     fs.FS
	version string
}

func New(cfg config.Config, s *store.Store, sy *syncer.Syncer, web fs.FS, version string) *Server {
	return &Server{cfg: cfg, store: s, syncer: sy, web: web, version: version}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /api/info", s.info)

	mux.HandleFunc("GET /api/torrents", s.listTorrents)
	mux.HandleFunc("POST /api/torrents", s.createTorrent)
	mux.HandleFunc("POST /api/torrents/import", s.importTorrents)
	mux.HandleFunc("GET /api/torrents/{hash}", s.getTorrent)
	mux.HandleFunc("PATCH /api/torrents/{hash}", s.updateTorrent)
	mux.HandleFunc("DELETE /api/torrents/{hash}", s.deleteTorrent)
	mux.HandleFunc("DELETE /api/torrents/{hash}/locations/{clientId}", s.removeFromClient)
	mux.HandleFunc("POST /api/torrents/{hash}/archives", s.addArchive)
	mux.HandleFunc("PUT /api/archives/{id}", s.updateArchive)
	mux.HandleFunc("DELETE /api/archives/{id}", s.deleteArchive)
	mux.HandleFunc("POST /api/torrents/{hash}/adoptions", s.addAdoption)
	mux.HandleFunc("DELETE /api/adoptions/{id}", s.deleteAdoption)

	mux.HandleFunc("GET /api/alerts", s.listAlerts)
	mux.HandleFunc("POST /api/alerts/{id}/resolve", s.resolveAlert)

	mux.HandleFunc("GET /api/disks", s.listDisks)
	mux.HandleFunc("POST /api/disks", s.createDisk)
	mux.HandleFunc("PUT /api/disks/{id}", s.updateDisk)
	mux.HandleFunc("DELETE /api/disks/{id}", s.deleteDisk)

	mux.HandleFunc("GET /api/adopters", s.listAdopters)
	mux.HandleFunc("POST /api/adopters", s.createAdopter)
	mux.HandleFunc("PUT /api/adopters/{id}", s.updateAdopter)
	mux.HandleFunc("DELETE /api/adopters/{id}", s.deleteAdopter)

	mux.HandleFunc("GET /api/clients", s.listClients)
	mux.HandleFunc("POST /api/clients", s.createClient)
	mux.HandleFunc("PUT /api/clients/{id}", s.updateClient)
	mux.HandleFunc("DELETE /api/clients/{id}", s.deleteClient)
	mux.HandleFunc("POST /api/clients/test", s.testClient)

	mux.HandleFunc("GET /api/sync", s.syncState)
	mux.HandleFunc("POST /api/sync", s.syncNow)

	mux.HandleFunc("POST /api/webhook/qbit", s.webhook)
	mux.HandleFunc("GET /api/webhook/qbit", s.webhook)

	mux.Handle("/", s.static())
	return s.logging(s.auth(mux))
}

// auth applies optional HTTP basic auth to everything except the health
// check and the webhook, which has its own token.
func (s *Server) auth(next http.Handler) http.Handler {
	if s.cfg.AuthUser == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/health" || strings.HasPrefix(r.URL.Path, "/api/webhook/") {
			next.ServeHTTP(w, r)
			return
		}
		u, p, ok := r.BasicAuth()
		if !ok || subtle.ConstantTimeCompare([]byte(u), []byte(s.cfg.AuthUser)) != 1 ||
			subtle.ConstantTimeCompare([]byte(p), []byte(s.cfg.AuthPassword)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="bookrr"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if strings.HasPrefix(r.URL.Path, "/api/") && r.Method != http.MethodGet {
			slog.Info("api", "method", r.Method, "path", r.URL.Path, "dur", time.Since(start).Round(time.Millisecond))
		}
	})
}

func (s *Server) static() http.Handler {
	files := http.FileServerFS(s.web)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeError(w, http.StatusNotFound, "endpoint inesistente")
			return
		}
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(s.web, p); err != nil {
			// SPA fallback.
			index, err := fs.ReadFile(s.web, "index.html")
			if err != nil {
				http.Error(w, "frontend non compilato (esegui npm run build in web/)", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			w.Write(index)
			return
		}
		if strings.HasPrefix(p, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, store.ErrExists), errors.Is(err, store.ErrOnClient), errors.Is(err, store.ErrAlreadyAdopted), errors.Is(err, store.ErrLastCopy):
		writeError(w, http.StatusConflict, err.Error())
	case strings.Contains(err.Error(), "UNIQUE"):
		writeError(w, http.StatusConflict, "esiste già un elemento con questo nome")
	default:
		slog.Error("errore api", "err", err)
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	return decodeLimit(w, r, v, 1<<20)
}

func decodeLimit(w http.ResponseWriter, r *http.Request, v any, limit int64) bool {
	dec := json.NewDecoder(io.LimitReader(r.Body, limit))
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "JSON non valido: "+err.Error())
		return false
	}
	return true
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "id non valido")
		return 0, false
	}
	return id, true
}

func (s *Server) info(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{
		"version":              s.version,
		"releaseTag":           s.cfg.ReleaseTag,
		"syncInterval":         shortDuration(s.cfg.SyncInterval),
		"webhookTokenRequired": s.cfg.WebhookToken != "",
	})
}

// shortDuration renders 5m0s as "5m" and 1h0m0s as "1h".
func shortDuration(d time.Duration) string {
	s := d.String()
	if strings.HasSuffix(s, "m0s") {
		s = strings.TrimSuffix(s, "0s")
	}
	if strings.HasSuffix(s, "h0m") {
		s = strings.TrimSuffix(s, "0m")
	}
	return s
}

// --- torrents ---

func (s *Server) listTorrents(w http.ResponseWriter, r *http.Request) {
	ts, err := s.store.ListTorrents(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, ts)
}

func (s *Server) getTorrent(w http.ResponseWriter, r *http.Request) {
	t, err := s.store.GetTorrent(r.Context(), r.PathValue("hash"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, t)
}

var hashRe = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

// normalizeManual cleans up a hand-entered torrent the same way for the
// "Aggiungi" form and for the bulk import.
func normalizeManual(in *store.ManualInput) error {
	in.Name = strings.TrimSpace(in.Name)
	in.Hash = strings.ToLower(strings.TrimSpace(in.Hash))
	tags := []string{}
	for _, t := range in.Tags {
		if t = strings.TrimSpace(t); t != "" {
			tags = append(tags, t)
		}
	}
	in.Tags = tags
	if in.Name == "" {
		return errors.New("il nome è obbligatorio")
	}
	if in.Hash != "" && !hashRe.MatchString(in.Hash) {
		return errors.New("info hash non valido: servono 40 o 64 caratteri esadecimali")
	}
	if in.Hash == "" {
		b := make([]byte, 8)
		rand.Read(b)
		in.Hash = "manual-" + hex.EncodeToString(b)
	}
	return nil
}

func (s *Server) createTorrent(w http.ResponseWriter, r *http.Request) {
	var in store.ManualInput
	if !decode(w, r, &in) {
		return
	}
	if err := normalizeManual(&in); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	t, err := s.store.CreateManualTorrent(r.Context(), in)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

type importRow struct {
	Row   int    `json:"row"`
	Hash  string `json:"hash"`
	Name  string `json:"name"`
	Error string `json:"error,omitempty"`
}

// importTorrents adds many archived torrents at once. Each item has the same
// shape as the body of POST /api/torrents and is stored exactly like a
// torrent added from the form. Nothing is written unless every item is
// valid; with dryRun nothing is written at all and the per-row check is
// returned.
func (s *Server) importTorrents(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Items  []store.ManualInput `json:"items"`
		DryRun bool                `json:"dryRun"`
	}
	if !decodeLimit(w, r, &in, 32<<20) {
		return
	}
	if len(in.Items) == 0 {
		writeError(w, http.StatusBadRequest, "nessun torrent da importare")
		return
	}
	ctx := r.Context()
	rows := make([]importRow, len(in.Items))
	seen := map[string]int{}
	disks := map[int64]bool{}
	adopters := map[int64]bool{}
	failed := false
	for i := range in.Items {
		item := &in.Items[i]
		err := s.checkImportItem(ctx, item, seen, disks, adopters)
		rows[i] = importRow{Row: i, Hash: item.Hash, Name: item.Name}
		if err != nil {
			rows[i].Error = err.Error()
			failed = true
		} else {
			seen[item.Hash] = i
		}
	}
	if failed {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"rows": rows, "imported": 0})
		return
	}
	if in.DryRun {
		writeJSON(w, 200, map[string]any{"rows": rows, "imported": 0})
		return
	}
	if err := s.store.ImportManualTorrents(ctx, in.Items); err != nil {
		var re *store.RowError
		if !errors.As(err, &re) {
			s.fail(w, err)
			return
		}
		rows[re.Row].Error = re.Err.Error()
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"rows": rows, "imported": 0})
		return
	}
	slog.Info("import torrent archiviati", "count", len(in.Items))
	writeJSON(w, http.StatusCreated, map[string]any{"rows": rows, "imported": len(in.Items)})
}

func (s *Server) checkImportItem(ctx context.Context, item *store.ManualInput, seen map[string]int, disks, adopters map[int64]bool) error {
	if err := normalizeManual(item); err != nil {
		return err
	}
	if !item.HasArchive() {
		return errors.New("indica il disco o il percorso dove è archiviato")
	}
	if i, dup := seen[item.Hash]; dup {
		return fmt.Errorf("hash ripetuto (riga %d)", i+1)
	}
	exists, err := s.store.TorrentExists(ctx, item.Hash)
	if err != nil {
		return err
	}
	if exists {
		return errors.New("torrent già presente in bookrr")
	}
	if id := item.Archive.DiskID; id != nil {
		ok, known := disks[*id]
		if !known {
			_, err := s.store.GetDisk(ctx, *id)
			if err != nil && !errors.Is(err, store.ErrNotFound) {
				return err
			}
			ok = err == nil
			disks[*id] = ok
		}
		if !ok {
			return errors.New("disco inesistente")
		}
	}
	if item.HasAdoption() {
		id := item.Adoption.AdopterID
		ok, known := adopters[id]
		if !known {
			_, err := s.store.GetAdopter(ctx, id)
			if err != nil && !errors.Is(err, store.ErrNotFound) {
				return err
			}
			ok = err == nil
			adopters[id] = ok
		}
		if !ok {
			return errors.New("adottatore inesistente")
		}
	}
	return nil
}

func (s *Server) updateTorrent(w http.ResponseWriter, r *http.Request) {
	var p store.TorrentPatch
	if !decode(w, r, &p) {
		return
	}
	t, err := s.store.UpdateTorrent(r.Context(), r.PathValue("hash"), p)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, t)
}

func (s *Server) deleteTorrent(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteTorrent(r.Context(), r.PathValue("hash")); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// removeFromClient deletes a duplicate torrent from one client, as long as
// another client keeps a complete copy. ?deleteFiles=1 also deletes the
// downloaded data on that client.
func (s *Server) removeFromClient(w http.ResponseWriter, r *http.Request) {
	hash := r.PathValue("hash")
	clientID, err := strconv.ParseInt(r.PathValue("clientId"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "id client non valido")
		return
	}
	deleteFiles := r.URL.Query().Get("deleteFiles") == "1"
	if err := s.store.CheckRemovableLocation(r.Context(), hash, clientID); err != nil {
		s.fail(w, err)
		return
	}
	c, err := s.store.GetClient(r.Context(), clientID)
	if err != nil {
		s.fail(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := s.syncer.Qbit(c).DeleteTorrent(ctx, hash, deleteFiles); err != nil {
		writeError(w, http.StatusBadGateway, "rimozione da "+c.Name+" non riuscita: "+err.Error())
		return
	}
	if err := s.store.RemoveLocation(r.Context(), hash, clientID); err != nil {
		s.fail(w, err)
		return
	}
	slog.Info("torrent rimosso dal client", "hash", hash, "client", c.Name, "deleteFiles", deleteFiles)
	t, err := s.store.GetTorrent(r.Context(), hash)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, t)
}

func (s *Server) addArchive(w http.ResponseWriter, r *http.Request) {
	var in store.ArchiveInput
	if !decode(w, r, &in) {
		return
	}
	hash := r.PathValue("hash")
	if err := s.store.AddArchive(r.Context(), hash, in); err != nil {
		s.fail(w, err)
		return
	}
	t, err := s.store.GetTorrent(r.Context(), hash)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) updateArchive(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in store.ArchiveInput
	if !decode(w, r, &in) {
		return
	}
	if err := s.store.UpdateArchive(r.Context(), id, in); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteArchive(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.store.DeleteArchive(r.Context(), id); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) addAdoption(w http.ResponseWriter, r *http.Request) {
	var in store.AdoptionInput
	if !decode(w, r, &in) {
		return
	}
	hash := r.PathValue("hash")
	if err := s.store.AddAdoption(r.Context(), hash, in); err != nil {
		s.fail(w, err)
		return
	}
	t, err := s.store.GetTorrent(r.Context(), hash)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) deleteAdoption(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.store.DeleteAdoption(r.Context(), id); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- alerts ---

func (s *Server) listAlerts(w http.ResponseWriter, r *http.Request) {
	open := r.URL.Query().Get("open") != "0"
	as, err := s.store.ListAlerts(r.Context(), open, "")
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, as)
}

func (s *Server) resolveAlert(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in store.ResolveInput
	if !decode(w, r, &in) {
		return
	}
	if err := s.store.ResolveAlert(r.Context(), id, in); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- disks ---

func (s *Server) listDisks(w http.ResponseWriter, r *http.Request) {
	ds, err := s.store.ListDisks(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, ds)
}

func validDisk(w http.ResponseWriter, d *store.Disk) bool {
	d.Label = strings.TrimSpace(d.Label)
	d.Serial = strings.TrimSpace(d.Serial)
	if d.Label == "" {
		writeError(w, http.StatusBadRequest, "il nome del disco è obbligatorio")
		return false
	}
	if d.Kind == "" {
		d.Kind = "HDD"
	}
	return true
}

func (s *Server) createDisk(w http.ResponseWriter, r *http.Request) {
	var d store.Disk
	if !decode(w, r, &d) || !validDisk(w, &d) {
		return
	}
	d, err := s.store.CreateDisk(r.Context(), d)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, d)
}

func (s *Server) updateDisk(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var d store.Disk
	if !decode(w, r, &d) || !validDisk(w, &d) {
		return
	}
	d.ID = id
	d, err := s.store.UpdateDisk(r.Context(), d)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, d)
}

func (s *Server) deleteDisk(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.store.DeleteDisk(r.Context(), id); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- adopters ---

func (s *Server) listAdopters(w http.ResponseWriter, r *http.Request) {
	as, err := s.store.ListAdopters(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, as)
}

func validAdopter(w http.ResponseWriter, a *store.Adopter) bool {
	a.Name = strings.TrimSpace(a.Name)
	a.Contact = strings.TrimSpace(a.Contact)
	if a.Name == "" {
		writeError(w, http.StatusBadRequest, "il nome dell'adottatore è obbligatorio")
		return false
	}
	return true
}

func (s *Server) createAdopter(w http.ResponseWriter, r *http.Request) {
	var a store.Adopter
	if !decode(w, r, &a) || !validAdopter(w, &a) {
		return
	}
	a, err := s.store.CreateAdopter(r.Context(), a)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

func (s *Server) updateAdopter(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var a store.Adopter
	if !decode(w, r, &a) || !validAdopter(w, &a) {
		return
	}
	a.ID = id
	a, err := s.store.UpdateAdopter(r.Context(), a)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, a)
}

func (s *Server) deleteAdopter(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.store.DeleteAdopter(r.Context(), id); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- clients ---

type clientInput struct {
	Name          string  `json:"name"`
	URL           string  `json:"url"`
	Username      string  `json:"username"`
	Password      *string `json:"password"` // nil = keep current
	SkipTLSVerify bool    `json:"skipTlsVerify"`
	Enabled       *bool   `json:"enabled"`
}

func (in *clientInput) validate(w http.ResponseWriter) bool {
	in.Name = strings.TrimSpace(in.Name)
	in.URL = strings.TrimRight(strings.TrimSpace(in.URL), "/")
	if in.Name == "" || in.URL == "" {
		writeError(w, http.StatusBadRequest, "nome e URL sono obbligatori")
		return false
	}
	u, err := url.Parse(in.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		writeError(w, http.StatusBadRequest, "URL non valido (es. http://192.168.1.10:8080)")
		return false
	}
	return true
}

func (s *Server) listClients(w http.ResponseWriter, r *http.Request) {
	cs, err := s.store.ListClients(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, cs)
}

func (s *Server) createClient(w http.ResponseWriter, r *http.Request) {
	var in clientInput
	if !decode(w, r, &in) || !in.validate(w) {
		return
	}
	c := store.Client{Name: in.Name, URL: in.URL, Username: in.Username, SkipTLSVerify: in.SkipTLSVerify, Enabled: true}
	if in.Password != nil {
		c.Password = *in.Password
	}
	if in.Enabled != nil {
		c.Enabled = *in.Enabled
	}
	c, err := s.store.CreateClient(r.Context(), c)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.syncInBackground()
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) updateClient(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in clientInput
	if !decode(w, r, &in) || !in.validate(w) {
		return
	}
	c, err := s.store.GetClient(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	c.Name, c.URL, c.Username, c.SkipTLSVerify = in.Name, in.URL, in.Username, in.SkipTLSVerify
	if in.Password != nil {
		c.Password = *in.Password
	}
	if in.Enabled != nil {
		c.Enabled = *in.Enabled
	}
	c, err = s.store.UpdateClient(r.Context(), c)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.syncInBackground()
	writeJSON(w, 200, c)
}

func (s *Server) deleteClient(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.store.DeleteClient(r.Context(), id); err != nil {
		s.fail(w, err)
		return
	}
	s.syncInBackground()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) testClient(w http.ResponseWriter, r *http.Request) {
	var in struct {
		clientInput
		ID *int64 `json:"id"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Name == "" {
		in.Name = "test"
	}
	if !in.validate(w) {
		return
	}
	password := ""
	if in.Password != nil {
		password = *in.Password
	} else if in.ID != nil {
		// Editing an existing client without retyping the password.
		if c, err := s.store.GetClient(r.Context(), *in.ID); err == nil {
			password = c.Password
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	q := qbit.New(in.URL, in.Username, password, in.SkipTLSVerify)
	v, err := q.Version(ctx)
	if err != nil {
		writeJSON(w, 200, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	ts, err := q.Torrents(ctx)
	if err != nil {
		writeJSON(w, 200, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "version": v, "torrents": len(ts)})
}

// --- sync ---

func (s *Server) syncState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.syncer.State())
}

func (s *Server) syncNow(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Minute)
	defer cancel()
	res, err := s.syncer.SyncAll(ctx)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]string{"result": res})
}

func (s *Server) syncInBackground() {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if _, err := s.syncer.SyncAll(ctx); err != nil {
			slog.Error("sync fallito", "err", err)
		}
	}()
}

// --- webhook ---

// webhook accepts a "torrent removed" notification as JSON, form data or
// query parameters, so it can be called from qBittorrent scripts, curl,
// qbit_manage, n8n and similar tools.
func (s *Server) webhook(w http.ResponseWriter, r *http.Request) {
	if s.cfg.WebhookToken != "" {
		tok := r.Header.Get("X-Bookrr-Token")
		if tok == "" {
			tok = r.URL.Query().Get("token")
		}
		if auth := r.Header.Get("Authorization"); tok == "" && strings.HasPrefix(auth, "Bearer ") {
			tok = strings.TrimPrefix(auth, "Bearer ")
		}
		if subtle.ConstantTimeCompare([]byte(tok), []byte(s.cfg.WebhookToken)) != 1 {
			writeError(w, http.StatusUnauthorized, "token non valido")
			return
		}
	}
	fields, err := webhookFields(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	pick := func(keys ...string) string {
		for _, k := range keys {
			if v := strings.TrimSpace(fields[k]); v != "" {
				return v
			}
		}
		return ""
	}
	hash := strings.ToLower(pick("hash", "infohash", "info_hash", "infohash_v1", "hash_v1", "torrent_hash", "id"))
	if hash == "" {
		writeError(w, http.StatusBadRequest, "manca l'hash del torrent (campo 'hash')")
		return
	}
	size, _ := strconv.ParseInt(pick("size", "total_size"), 10, 64)
	ev := store.WebhookRemoval{
		Hash:       hash,
		Name:       pick("name", "torrent_name", "title"),
		Tags:       store.NormalizeTags(pick("tags", "tag")),
		Category:   pick("category"),
		Size:       size,
		ClientName: pick("client", "client_name", "instance"),
	}
	created, err := s.store.HandleWebhookRemoval(r.Context(), ev, s.cfg.ReleaseTag)
	if err != nil {
		s.fail(w, err)
		return
	}
	slog.Info("webhook rimozione", "hash", hash, "name", ev.Name, "client", ev.ClientName, "alert", created)
	writeJSON(w, 200, map[string]any{"ok": true, "alert": created})
}

func webhookFields(r *http.Request) (map[string]string, error) {
	out := map[string]string{}
	for k, v := range r.URL.Query() {
		if len(v) > 0 {
			out[strings.ToLower(k)] = v[0]
		}
	}
	if r.Method != http.MethodPost || r.ContentLength == 0 {
		return out, nil
	}
	ct, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	switch ct {
	case "application/x-www-form-urlencoded", "multipart/form-data":
		if err := r.ParseMultipartForm(1 << 20); err != nil && !errors.Is(err, http.ErrNotMultipart) {
			return nil, err
		}
		for k, v := range r.PostForm {
			if len(v) > 0 {
				out[strings.ToLower(k)] = v[0]
			}
		}
	default:
		var body map[string]any
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body); err != nil {
			if errors.Is(err, io.EOF) {
				return out, nil
			}
			return nil, errors.New("body JSON non valido")
		}
		flatten(out, body)
	}
	return out, nil
}

// flatten copies scalar values (also from one level of nesting, e.g.
// {"torrent": {"hash": ...}}) into out.
func flatten(out map[string]string, m map[string]any) {
	for k, v := range m {
		k = strings.ToLower(k)
		switch x := v.(type) {
		case string:
			out[k] = x
		case float64:
			out[k] = strconv.FormatFloat(x, 'f', -1, 64)
		case []any:
			parts := []string{}
			for _, p := range x {
				if s, ok := p.(string); ok {
					parts = append(parts, s)
				}
			}
			out[k] = strings.Join(parts, ",")
		case map[string]any:
			nested := map[string]string{}
			flatten(nested, x)
			for nk, nv := range nested {
				if _, exists := out[nk]; !exists {
					out[nk] = nv
				}
			}
		}
	}
}
