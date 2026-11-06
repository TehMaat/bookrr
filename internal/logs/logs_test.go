package logs

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newWriter(t *testing.T, now *time.Time) *Writer {
	t.Helper()
	w, err := New(filepath.Join(t.TempDir(), "logs"))
	if err != nil {
		t.Fatal(err)
	}
	w.now = func() time.Time { return *now }
	t.Cleanup(func() { w.Close() })
	return w
}

func names(fs []File) string {
	var out []string
	for _, f := range fs {
		out = append(out, f.Name)
	}
	return strings.Join(out, " ")
}

func TestHandlerWritesStderrAndFile(t *testing.T) {
	now := time.Date(2026, 11, 6, 23, 59, 0, 0, time.Local)
	w := newWriter(t, &now)
	var stderr bytes.Buffer
	log := slog.New(Handler(w, &stderr))
	log.Info("sync completato", "ok", 2, "client", "nas")
	log.Debug("not written")
	log.With("client", "seedbox").WithGroup("g").Warn("client non raggiungibile", "err", "timeout")
	now = now.Add(2 * time.Minute)
	log.Error("dopo mezzanotte")

	if s := stderr.String(); !strings.Contains(s, "msg=\"sync completato\"") || !strings.Contains(s, "level=ERROR") || strings.Contains(s, "not written") {
		t.Fatalf("stderr %s", s)
	}
	if got := names(w.Files()); got != "bookrr-2026-11-07.log bookrr-2026-11-06.log" {
		t.Fatalf("files %s", got)
	}
	res, err := w.Read(Query{Day: "2026-11-06"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Entries) != 2 {
		t.Fatalf("entries %+v", res.Entries)
	}
	e := res.Entries[0]
	if e.ID != 0 || e.Level != "INFO" || e.Msg != "sync completato" || e.Time == "" || e.Attrs["client"] != "nas" || e.Attrs["ok"] == nil {
		t.Fatalf("entry %+v", e)
	}
	e = res.Entries[1]
	if g, _ := e.Attrs["g"].(map[string]any); e.Level != "WARN" || e.Attrs["client"] != "seedbox" || g["err"] != "timeout" || e.ID == 0 {
		t.Fatalf("entry %+v", e)
	}

	// The level changes at once.
	w.Configure(30, 0, slog.LevelDebug)
	log.Debug("ora sì")
	if res, _ := w.Read(Query{Day: "2026-11-07"}); len(res.Entries) != 2 || res.Entries[1].Msg != "ora sì" {
		t.Fatalf("debug %+v", res.Entries)
	}
}

func TestReadFilters(t *testing.T) {
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.Local)
	w := newWriter(t, &now)
	log := slog.New(Handler(w, &bytes.Buffer{}))
	log.Info("uno", "hash", "ABC")
	log.Warn("due")
	log.Error("tre", "err", "Connessione rifiutata")

	warn := slog.LevelWarn
	res, err := w.Read(Query{Day: "2026-11-06", Level: &warn})
	if err != nil || len(res.Entries) != 2 || res.Entries[0].Msg != "due" {
		t.Fatalf("level %+v %v", res.Entries, err)
	}
	if res, _ := w.Read(Query{Day: "2026-11-06", Text: "connessione"}); len(res.Entries) != 1 || res.Entries[0].Msg != "tre" {
		t.Fatalf("text %+v", res.Entries)
	}
	if res, _ := w.Read(Query{Day: "2026-11-06", Text: "abc"}); len(res.Entries) != 1 || res.Entries[0].Msg != "uno" {
		t.Fatalf("attribute %+v", res.Entries)
	}
	// The last ones when there are too many.
	res, _ = w.Read(Query{Day: "2026-11-06", Limit: 2})
	if len(res.Entries) != 2 || !res.Truncated || res.Entries[0].Msg != "due" {
		t.Fatalf("limit %+v", res)
	}
	// Only the new lines, even when the filter left the last ones out.
	all, _ := w.Read(Query{Day: "2026-11-06", Text: "uno"})
	log.Info("quattro")
	res, _ = w.Read(Query{Day: "2026-11-06", After: all.Next})
	if len(res.Entries) != 1 || res.Entries[0].Msg != "quattro" || res.Next <= all.Next {
		t.Fatalf("after %+v (next %d)", res, all.Next)
	}
	if again, _ := w.Read(Query{Day: "2026-11-06", After: res.Next}); len(again.Entries) != 0 || again.Next != res.Next {
		t.Fatalf("nothing new %+v", again)
	}
	// A day without files.
	if res, err := w.Read(Query{Day: "2026-01-01"}); err != nil || len(res.Entries) != 0 || res.Next != 0 {
		t.Fatalf("empty day %+v %v", res, err)
	}
}

func TestReadSkipsLineBeingWritten(t *testing.T) {
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.Local)
	w := newWriter(t, &now)
	slog.New(Handler(w, &bytes.Buffer{})).Info("intera")
	f, err := os.OpenFile(filepath.Join(w.Dir(), "bookrr-2026-11-06.log"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(`{"msg":"a me`)
	res, _ := w.Read(Query{Day: "2026-11-06"})
	if len(res.Entries) != 1 {
		t.Fatalf("entries %+v", res.Entries)
	}
	f.WriteString("tà\"}\n")
	f.Close()
	res, _ = w.Read(Query{Day: "2026-11-06", After: res.Next})
	if len(res.Entries) != 1 || res.Entries[0].Msg != "a metà" || res.Entries[0].Level != "" {
		t.Fatalf("finished %+v", res.Entries)
	}
}

func TestMaxSizeAndRestart(t *testing.T) {
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.Local)
	w := newWriter(t, &now)
	w.Configure(30, 100, slog.LevelInfo)
	line := strings.Repeat("x", 59) + "\n" // 60 bytes: one per file
	for range 3 {
		w.Write([]byte(line))
	}
	if got := names(w.Files()); got != "bookrr-2026-11-06.log bookrr-2026-11-06.2.log bookrr-2026-11-06.3.log" {
		t.Fatalf("files %s", got)
	}
	res, _ := w.Read(Query{Day: "2026-11-06"})
	if len(res.Entries) != 3 || res.Entries[2].ID != 120 || res.Next != 180 {
		t.Fatalf("read %+v", res)
	}
	// After a restart: on in the last part, while there is room.
	w.Close()
	w2, _ := New(w.Dir())
	w2.now = w.now
	w2.Configure(30, 130, slog.LevelInfo)
	w2.Write([]byte(line))
	w2.Write([]byte(line))
	w2.Close()
	if got := names(w2.Files()); !strings.HasSuffix(got, "bookrr-2026-11-06.3.log bookrr-2026-11-06.4.log") {
		t.Fatalf("after restart %s", got)
	}
	res, _ = w2.Read(Query{Day: "2026-11-06", After: res.Next})
	if len(res.Entries) != 2 || res.Entries[0].ID != 180 {
		t.Fatalf("new lines %+v", res)
	}
}

func TestCleanup(t *testing.T) {
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.Local)
	w := newWriter(t, &now)
	for _, n := range []string{"bookrr-2026-10-07.log", "bookrr-2026-10-08.log", "bookrr-2026-10-08.2.log", "bookrr-2026-11-06.log", "other.txt"} {
		os.WriteFile(filepath.Join(w.Dir(), n), []byte("{}\n"), 0o644)
	}
	w.Configure(0, 0, slog.LevelInfo)
	if got := names(w.Files()); got != "bookrr-2026-11-06.log bookrr-2026-10-08.log bookrr-2026-10-08.2.log bookrr-2026-10-07.log" {
		t.Fatalf("forever %s", got)
	}
	// 30 days: today and the 29 before, from 2026-10-08.
	w.Configure(30, 0, slog.LevelInfo)
	if got := names(w.Files()); got != "bookrr-2026-11-06.log bookrr-2026-10-08.log bookrr-2026-10-08.2.log" {
		t.Fatalf("30 days %s", got)
	}
	// At midnight.
	now = now.Add(24 * time.Hour)
	w.Write([]byte("{}\n"))
	deadline := time.Now().Add(2 * time.Second)
	for names(w.Files()) != "bookrr-2026-11-07.log bookrr-2026-11-06.log" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := names(w.Files()); got != "bookrr-2026-11-07.log bookrr-2026-11-06.log" {
		t.Fatalf("next day %s", got)
	}
	if _, err := os.Stat(filepath.Join(w.Dir(), "other.txt")); err != nil {
		t.Fatal("other files are not touched")
	}
}

func TestOpenRejectsOtherPaths(t *testing.T) {
	now := time.Now()
	w := newWriter(t, &now)
	os.WriteFile(filepath.Join(filepath.Dir(w.Dir()), "secret.log"), []byte("x"), 0o644)
	for _, n := range []string{"../secret.log", "bookrr-2026-11-06.log/../../secret.log", "secret.log", ""} {
		if f, err := w.Open(n); err == nil {
			f.Close()
			t.Errorf("%q opened", n)
		}
	}
}
