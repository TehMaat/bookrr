// Package logs writes the messages of bookrr to a file a day in the data
// folder (logs/bookrr-2026-11-06.log, JSON Lines), besides stderr, deletes
// the files older than the retention and reads them back for the web UI.
package logs

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"sync"
	"time"
)

const dayLayout = "2006-01-02"

// fileRe is the name of a log file: the day, and the part after the first
// ("bookrr-2026-11-06.2.log") when the file of the day reached the
// maximum size.
var fileRe = regexp.MustCompile(`^bookrr-(\d{4}-\d{2}-\d{2})(?:\.(\d+))?\.log$`)

// Writer writes lines to the file of the day, moving to a new one at
// midnight (local time) and when the file reaches the maximum size.
type Writer struct {
	dir string
	// now is the current time, replaced by the tests.
	now func() time.Time

	mu        sync.Mutex
	f         *os.File
	day       string
	part      int
	size      int64
	retention int   // days, 0 = forever
	maxSize   int64 // bytes, 0 = no limit
	level     slog.LevelVar
	// The last error opening or writing a file, reported once on stderr.
	failed bool
}

// New makes a writer of the files in dir, created if missing.
func New(dir string) (*Writer, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Writer{dir: dir, now: time.Now, retention: 30}, nil
}

// Dir is the folder of the files.
func (w *Writer) Dir() string { return w.dir }

// Configure sets how many days to keep the files (0 = forever), the
// maximum size of a file (0 = no limit) and the minimum level written,
// and deletes the files past the retention now.
func (w *Writer) Configure(retentionDays int, maxSize int64, level slog.Level) {
	w.mu.Lock()
	w.retention, w.maxSize = retentionDays, maxSize
	w.level.Set(level)
	w.mu.Unlock()
	w.Cleanup()
}

// Level is the minimum level written, changed by Configure.
func (w *Writer) Level() slog.Leveler { return &w.level }

func fileName(day string, part int) string {
	if part <= 1 {
		return "bookrr-" + day + ".log"
	}
	return fmt.Sprintf("bookrr-%s.%d.log", day, part)
}

// Write writes a line (slog writes a whole record at a time).
func (w *Writer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	day := w.now().Format(dayLayout)
	newDay := day != w.day
	if newDay || (w.maxSize > 0 && w.size > 0 && w.size+int64(len(p)) > w.maxSize) {
		if err := w.open(day, newDay); err != nil {
			w.report(err)
			return 0, err
		}
		if newDay {
			go w.cleanup(day)
		}
	}
	n, err := w.f.Write(p)
	w.size += int64(n)
	if err != nil {
		w.report(err)
	}
	return n, err
}

// open moves to the file of day: the last part written that day (after a
// restart), or the next one when the current is full.
func (w *Writer) open(day string, newDay bool) error {
	if w.f != nil {
		w.f.Close()
		w.f = nil
	}
	// Until a file is open, every write tries again.
	prev := w.day
	w.day = ""
	part := w.part + 1
	if prev != day {
		newDay = true
	}
	if newDay {
		part = 1
		for _, f := range w.list() {
			if f.Day == day && f.Part > part {
				part = f.Part
			}
		}
	}
	for {
		f, err := os.OpenFile(filepath.Join(w.dir, fileName(day, part)), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return err
		}
		st, err := f.Stat()
		if err != nil {
			f.Close()
			return err
		}
		if newDay && w.maxSize > 0 && st.Size() >= w.maxSize {
			f.Close()
			part++
			continue
		}
		w.f, w.day, w.part, w.size, w.failed = f, day, part, st.Size(), false
		return nil
	}
}

// report writes on stderr the first error of a series: the messages still
// go to stderr, not to the file.
func (w *Writer) report(err error) {
	if !w.failed {
		w.failed = true
		fmt.Fprintf(os.Stderr, "file log not written in %s: %v\n", w.dir, err)
	}
}

// Close closes the current file.
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f == nil {
		return nil
	}
	err := w.f.Close()
	w.f, w.day = nil, ""
	return err
}

// Cleanup deletes the files of the days past the retention: with 30 days,
// today's and those of the 29 days before are kept.
func (w *Writer) Cleanup() { w.cleanup(w.Today()) }

func (w *Writer) cleanup(today string) {
	w.mu.Lock()
	days := w.retention
	w.mu.Unlock()
	t, err := time.Parse(dayLayout, today)
	if days <= 0 || err != nil {
		return
	}
	oldest := t.AddDate(0, 0, 1-days).Format(dayLayout)
	for _, f := range w.list() {
		if f.Day < oldest {
			if err := os.Remove(filepath.Join(w.dir, f.Name)); err != nil && !os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "log %s not deleted: %v\n", f.Name, err)
			}
		}
	}
}

// File is a log file.
type File struct {
	Name string `json:"name"`
	Day  string `json:"day"`
	Part int    `json:"part"`
	Size int64  `json:"size"`
	// Last write, RFC 3339.
	ModifiedAt string `json:"modifiedAt"`
}

// list is the log files in the folder, by day and part.
func (w *Writer) list() []File {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return nil
	}
	var out []File
	for _, e := range entries {
		m := fileRe.FindStringSubmatch(e.Name())
		if m == nil || !e.Type().IsRegular() {
			continue
		}
		part := 1
		if m[2] != "" {
			part, _ = strconv.Atoi(m[2])
		}
		f := File{Name: e.Name(), Day: m[1], Part: part}
		if info, err := e.Info(); err == nil {
			f.Size, f.ModifiedAt = info.Size(), info.ModTime().UTC().Format(time.RFC3339)
		}
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Day != out[j].Day {
			return out[i].Day < out[j].Day
		}
		return out[i].Part < out[j].Part
	})
	return out
}

// Files is the log files, the newest day first, the parts of a day in order.
func (w *Writer) Files() []File {
	w.Cleanup()
	fs := w.list()
	sort.SliceStable(fs, func(i, j int) bool { return fs[i].Day > fs[j].Day })
	if fs == nil {
		fs = []File{}
	}
	return fs
}

// Today is the current day, as the files name it.
func (w *Writer) Today() string { return w.now().Format(dayLayout) }

// Open opens a log file by name, which must be one of the files of the
// folder (no other path).
func (w *Writer) Open(name string) (*os.File, error) {
	if !fileRe.MatchString(name) {
		return nil, os.ErrNotExist
	}
	return os.Open(filepath.Join(w.dir, name))
}

// ValidDay reports whether s is a day as the files name it (2026-11-06).
func ValidDay(s string) bool {
	_, err := time.Parse(dayLayout, s)
	return err == nil
}

// Handler writes the records on stderr as text and in the files as JSON,
// at or above the level of w.
func Handler(w *Writer, stderr io.Writer) slog.Handler {
	opts := &slog.HandlerOptions{Level: w.Level()}
	return &tee{
		level: w.Level(),
		hs:    []slog.Handler{slog.NewTextHandler(stderr, opts), slog.NewJSONHandler(w, opts)},
	}
}

// tee sends a record to every handler.
type tee struct {
	level slog.Leveler
	hs    []slog.Handler
}

func (t *tee) Enabled(_ context.Context, l slog.Level) bool { return l >= t.level.Level() }

func (t *tee) Handle(ctx context.Context, r slog.Record) error {
	var first error
	for _, h := range t.hs {
		if err := h.Handle(ctx, r.Clone()); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (t *tee) WithAttrs(as []slog.Attr) slog.Handler {
	out := &tee{level: t.level}
	for _, h := range t.hs {
		out.hs = append(out.hs, h.WithAttrs(as))
	}
	return out
}

func (t *tee) WithGroup(name string) slog.Handler {
	out := &tee{level: t.level}
	for _, h := range t.hs {
		out.hs = append(out.hs, h.WithGroup(name))
	}
	return out
}
