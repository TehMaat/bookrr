package api

import (
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/tehmaat/bookrr/internal/logs"
)

// logLimit is how many lines a read returns at most, the last ones.
const logLimit = 5000

// logFiles lists the log files, the newest day first.
func (s *Server) logFiles(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"enabled": s.Logs != nil, "files": []logs.File{}, "today": "", "dir": ""}
	if s.Logs != nil {
		out["files"], out["today"], out["dir"] = s.Logs.Files(), s.Logs.Today(), s.Logs.Dir()
	}
	writeJSON(w, 200, out)
}

// readLogs reads the lines of a day: ?day=2026-11-06 (today if not
// given), level= (the minimum: debug, info, warn, error), q= (text),
// after= (the "next" of an earlier read: only the lines added since).
func (s *Server) readLogs(w http.ResponseWriter, r *http.Request) {
	if s.Logs == nil {
		writeError(w, http.StatusNotFound, "file logs are not active")
		return
	}
	v := r.URL.Query()
	q := logs.Query{Day: strings.TrimSpace(v.Get("day")), Text: v.Get("q"), Limit: logLimit}
	if q.Day == "" {
		q.Day = s.Logs.Today()
	}
	if !logs.ValidDay(q.Day) {
		writeError(w, http.StatusBadRequest, "invalid day (YYYY-MM-DD)")
		return
	}
	if l := strings.TrimSpace(v.Get("level")); l != "" {
		level, err := logs.ParseLevel(l)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid level (debug, info, warn, error)")
			return
		}
		q.Level = &level
	}
	if a := v.Get("after"); a != "" {
		n, err := strconv.ParseInt(a, 10, 64)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "invalid after")
			return
		}
		q.After = n
	}
	res, err := s.Logs.Read(q)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, 200, res)
}

// downloadLog sends a log file as it is.
func (s *Server) downloadLog(w http.ResponseWriter, r *http.Request) {
	if s.Logs == nil {
		writeError(w, http.StatusNotFound, "file logs are not active")
		return
	}
	name := r.PathValue("name")
	f, err := s.Logs.Open(name)
	if errors.Is(err, os.ErrNotExist) {
		writeError(w, http.StatusNotFound, "no such log file")
		return
	}
	if err != nil {
		slog.Error("reading log", "file", name, "err", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	w.Header().Set("Cache-Control", "no-store")
	io.Copy(w, f)
}
