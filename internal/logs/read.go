package logs

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// Entry is a line of a log file.
type Entry struct {
	// Position of the line in the day (bytes from the start of its first
	// file): it identifies the line, and Query.After reads past it.
	ID    int64          `json:"id"`
	Time  string         `json:"time"`
	Level string         `json:"level"`
	Msg   string         `json:"msg"`
	Attrs map[string]any `json:"attrs"`
}

// Query selects the lines of a day.
type Query struct {
	Day string
	// Minimum level; lines of any level when nil.
	Level *slog.Level
	// Text to find in the line, any case.
	Text string
	// Only the lines past this position of the day (Result.Next of an
	// earlier read): the new ones.
	After int64
	// At most this many lines, the last ones; 0 = no limit.
	Limit int
}

// Result is the lines read, oldest first.
type Result struct {
	Entries []Entry `json:"entries"`
	// Position past the last complete line of the day, for Query.After.
	Next int64 `json:"next"`
	// More lines matched than Limit: the first ones were left out.
	Truncated bool `json:"truncated"`
}

// Read reads the lines of a day that match q, from all the files of the
// day in order.
func (w *Writer) Read(q Query) (Result, error) {
	res := Result{Entries: []Entry{}, Next: q.After}
	text := strings.ToLower(strings.TrimSpace(q.Text))
	var base int64 // position of the start of the current file in the day
	for _, f := range w.list() {
		if f.Day != q.Day {
			continue
		}
		if q.After >= base+f.Size {
			// Read already (only the last file of the day grows).
			base += f.Size
			continue
		}
		end, err := w.readFile(f.Name, base, q, text, &res)
		if err != nil {
			return res, err
		}
		res.Next = max(res.Next, end)
		if end < base+f.Size {
			// A line being written: the next read starts from it.
			break
		}
		// The file may have grown while it was read.
		base = end
	}
	if q.Limit > 0 && len(res.Entries) > q.Limit {
		res.Entries = res.Entries[len(res.Entries)-q.Limit:]
		res.Truncated = true
	}
	return res, nil
}

// readFile adds the lines of a file past q.After; it returns the position
// past its last complete line.
func (w *Writer) readFile(name string, base int64, q Query, text string, res *Result) (int64, error) {
	f, err := os.Open(filepath.Join(w.dir, name))
	if os.IsNotExist(err) {
		return base, nil
	}
	if err != nil {
		return base, err
	}
	defer f.Close()
	pos := base
	if q.After > base {
		if _, err := f.Seek(q.After-base, io.SeekStart); err != nil {
			return base, err
		}
		pos = q.After
	}
	r := bufio.NewReaderSize(f, 64<<10)
	for {
		line, err := r.ReadBytes('\n')
		if len(line) == 0 || line[len(line)-1] != '\n' {
			// End of the file, or a line not finished yet.
			if err != nil && err != io.EOF {
				return pos, err
			}
			return pos, nil
		}
		id := pos
		pos += int64(len(line))
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		if text != "" && !strings.Contains(strings.ToLower(string(line)), text) {
			continue
		}
		e := parse(line)
		e.ID = id
		if q.Level != nil && levelOf(e.Level) < *q.Level {
			continue
		}
		res.Entries = append(res.Entries, e)
		if q.Limit > 0 && len(res.Entries) > 2*q.Limit {
			res.Entries = append(res.Entries[:0], res.Entries[len(res.Entries)-q.Limit:]...)
			res.Truncated = true
		}
	}
}

// parse reads a JSON line of slog; a line that is not one is all message.
func parse(line []byte) Entry {
	var m map[string]any
	d := json.NewDecoder(bytes.NewReader(line))
	d.UseNumber()
	if err := d.Decode(&m); err != nil {
		return Entry{Level: "INFO", Msg: string(line), Attrs: map[string]any{}}
	}
	e := Entry{Attrs: m}
	take := func(k string) string {
		v, _ := m[k].(string)
		delete(m, k)
		return v
	}
	e.Time, e.Level, e.Msg = take(slog.TimeKey), take(slog.LevelKey), take(slog.MessageKey)
	return e
}

// levelOf reads a level as slog writes it ("INFO", "WARN+2"); an unknown
// one is INFO.
func levelOf(s string) slog.Level {
	var l slog.Level
	if err := l.UnmarshalText([]byte(s)); err != nil {
		return slog.LevelInfo
	}
	return l
}

// ParseLevel reads a minimum level ("debug", "info", "warn", "error").
func ParseLevel(s string) (slog.Level, error) {
	var l slog.Level
	err := l.UnmarshalText([]byte(strings.TrimSpace(s)))
	return l, err
}
