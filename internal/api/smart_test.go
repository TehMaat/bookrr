package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/tehmaat/bookrr/internal/config"
	"github.com/tehmaat/bookrr/internal/store"
)

const smartReport = `Device Model:     WDC WD40EFRX-68N32N0
Serial Number:    WD-WCC7K1234567
Firmware Version: 82.00A82
User Capacity:    4,000,787,030,016 bytes [4.00 TB]
Rotation Rate:    5400 rpm
SMART overall-health self-assessment test result: PASSED
  9 Power_On_Hours          0x0032   061   061   000    Old_age   Always       -       28734
`

type smartResponse struct {
	Created bool       `json:"created"`
	Disk    store.Disk `json:"disk"`
}

func postSmart(t *testing.T, h http.Handler, query, body string) (int, smartResponse) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/api/disks/smart"+query, strings.NewReader(body)))
	var out smartResponse
	if rec.Code < 300 {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
	}
	return rec.Code, out
}

func TestImportSmart(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	h := New(config.Config{}, st, nil, nil, nil, fstest.MapFS{}, "test").Handler()

	// A dry run saves nothing.
	code, res := postSmart(t, h, "?dryRun=1", smartReport)
	if code != 200 || !res.Created || res.Disk.ID != 0 || res.Disk.Label != "WDC WD40EFRX-68N32N0" {
		t.Fatalf("dry run: %d %+v", code, res)
	}
	if ds, _ := st.ListDisks(ctx); len(ds) != 0 {
		t.Fatalf("dry run saved %d disks", len(ds))
	}

	code, res = postSmart(t, h, "?label=Archivio%2001", smartReport)
	if code != 201 || !res.Created {
		t.Fatalf("create: %d %+v", code, res)
	}
	d := res.Disk
	if d.Label != "Archivio 01" || d.Kind != "HDD" || d.Capacity != 4000787030016 || d.Health != "PASSED" ||
		d.PowerOnHours != 28734 || d.Firmware != "82.00A82" || d.SmartAt == nil {
		t.Fatalf("created disk: %+v", d)
	}

	// The user's own fields survive a later import matched by serial.
	d.Kind, d.Place, d.Notes = "USB", "Cassetto", "nota"
	if _, err := st.UpdateDisk(ctx, d); err != nil {
		t.Fatal(err)
	}
	newer := strings.Replace(smartReport, "28734", "30000", 1)
	newer = strings.Replace(newer, "WD-WCC7K1234567", " wd-wcc7k1234567 ", 1)
	code, res = postSmart(t, h, "", newer)
	if code != 200 || res.Created || res.Disk.ID != d.ID {
		t.Fatalf("update: %d %+v", code, res)
	}
	if u := res.Disk; u.Label != "Archivio 01" || u.Kind != "USB" || u.Place != "Cassetto" || u.Notes != "nota" || u.PowerOnHours != 30000 {
		t.Fatalf("updated disk: %+v", u)
	}

	// Without a serial number the disk must be named explicitly.
	noSerial := strings.Replace(smartReport, "Serial Number:    WD-WCC7K1234567\n", "", 1)
	if code, _ := postSmart(t, h, "", noSerial); code != 400 {
		t.Fatalf("no serial: %d", code)
	}
	if code, res := postSmart(t, h, "?id="+strconv.FormatInt(d.ID, 10), noSerial); code != 200 || res.Disk.ID != d.ID {
		t.Fatalf("by id: %d %+v", code, res)
	}
	if code, _ := postSmart(t, h, "?id=999", smartReport); code != 404 {
		t.Fatalf("missing id: %d", code)
	}
	if code, _ := postSmart(t, h, "", "non è smart"); code != 400 {
		t.Fatalf("garbage: %d", code)
	}
	if ds, _ := st.ListDisks(ctx); len(ds) != 1 {
		t.Fatalf("got %d disks, want 1", len(ds))
	}
}
