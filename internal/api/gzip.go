package api

import (
	"compress/gzip"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// The fastest level: the lists of torrents shrink almost as much and a
// small NAS or a Raspberry Pi spends little time on them.
var gzipWriters = sync.Pool{New: func() any {
	w, _ := gzip.NewWriterLevel(nil, gzip.BestSpeed)
	return w
}}

// compress gzips the text responses (the JSON of the API, the scripts and
// styles of the web UI) for the browsers that accept it: the list of
// torrents is mostly names and paths repeated over and over.
func compress(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead || r.Header.Get("Range") != "" || !acceptsGzip(r.Header.Get("Accept-Encoding")) {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Add("Vary", "Accept-Encoding")
		gw := &gzipWriter{ResponseWriter: w}
		defer gw.close()
		next.ServeHTTP(gw, r)
	})
}

func acceptsGzip(header string) bool {
	for _, part := range strings.Split(header, ",") {
		coding, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		if !strings.EqualFold(strings.TrimSpace(coding), "gzip") {
			continue
		}
		q, found := strings.CutPrefix(strings.ReplaceAll(params, " ", ""), "q=")
		if !found {
			return true
		}
		v, err := strconv.ParseFloat(q, 64)
		return err != nil || v > 0
	}
	return false
}

func compressible(contentType string) bool {
	t, _, _ := mime.ParseMediaType(contentType)
	return strings.HasPrefix(t, "text/") || t == "application/json" || t == "application/javascript" || t == "image/svg+xml"
}

// gzipWriter decides when the headers are written whether the response is
// worth compressing.
type gzipWriter struct {
	http.ResponseWriter
	gz      *gzip.Writer
	started bool
}

func (g *gzipWriter) WriteHeader(code int) {
	if !g.started {
		g.started = true
		h := g.Header()
		small := false
		if n, err := strconv.Atoi(h.Get("Content-Length")); err == nil && n < 1024 {
			small = true
		}
		if code != http.StatusNoContent && code != http.StatusNotModified && !small &&
			h.Get("Content-Encoding") == "" && compressible(h.Get("Content-Type")) {
			h.Del("Content-Length")
			h.Set("Content-Encoding", "gzip")
			g.gz = gzipWriters.Get().(*gzip.Writer)
			g.gz.Reset(g.ResponseWriter)
		}
	}
	g.ResponseWriter.WriteHeader(code)
}

func (g *gzipWriter) Write(p []byte) (int, error) {
	if !g.started {
		if g.Header().Get("Content-Type") == "" {
			g.Header().Set("Content-Type", http.DetectContentType(p))
		}
		g.WriteHeader(http.StatusOK)
	}
	if g.gz != nil {
		return g.gz.Write(p)
	}
	return g.ResponseWriter.Write(p)
}

func (g *gzipWriter) close() {
	if g.gz == nil {
		return
	}
	g.gz.Close()
	g.gz.Reset(nil)
	gzipWriters.Put(g.gz)
}
