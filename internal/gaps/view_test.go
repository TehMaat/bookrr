package gaps

import (
	"testing"

	"github.com/tehmaat/bookrr/internal/store"
	"github.com/tehmaat/bookrr/internal/unit3d"
)

// alien is the saga of the example: the user uploaded every film in the
// best quality there is, Alien³ and Alien - La clonazione only in 1080p.
func alien() *Group {
	g := &Group{Kind: KindSaga, Key: "collection:8091", Status: StatusOK, Units: []Unit{
		{Number: 1, Name: "Alien", TmdbID: 348},
		{Number: 2, Name: "Aliens", TmdbID: 679},
		{Number: 3, Name: "Alien³", TmdbID: 8077},
		{Number: 4, Name: "Alien - La clonazione", TmdbID: 8078},
	}}
	q := func(name string) unit3d.Quality { return unit3d.QualityOf(name, "", "", "") }
	it := func(id int64, name string, mine bool, units ...int) item {
		u := map[int]bool{}
		for _, n := range units {
			u[n] = true
		}
		qq := q(name)
		return item{ref: Ref{ID: id, Name: name, Quality: qq.Label, Mine: mine}, quality: qq, units: u}
	}
	g.buildRows([]item{
		it(1, "Alien.1979.2160p.BluRay.REMUX.DV.HDR.ITA.ENG", true, 1),
		it(2, "Aliens.1986.2160p.BluRay.REMUX.DV.HDR.ITA.ENG", true, 2),
		it(3, "Alien.1979.2160p.BluRay.REMUX.HDR.ITA.ENG", true, 1),
		it(4, "Alien3.1992.1080p.BluRay.REMUX.ITA.ENG", true, 3),
		it(5, "Alien.Resurrection.1997.1080p.BluRay.REMUX.ITA.ENG", true, 4),
		it(10, "Alien.1979.1080p.BluRay.REMUX.ITA.ENG", false, 1),
		it(11, "Aliens.1986.1080p.BluRay.REMUX.ITA.ENG", false, 2),
		it(12, "Alien.Resurrection.1997.1080p.BluRay.x264.ITA.ENG", false, 4),
	})
	return g
}

func parts(g *Group) map[int]Part {
	out := map[int]Part{}
	for _, p := range g.Parts {
		out[p.Unit] = p
	}
	return out
}

func TestApplyByPart(t *testing.T) {
	g := alien()
	if len(g.Rows) != 3 || g.Missing != 7 {
		t.Fatalf("grid: %d rows, %d missing", len(g.Rows), g.Missing)
	}

	// With no minimum, the best of the user's versions: complete.
	Apply(g, store.GapPrefs{}, nil)
	p := parts(g)
	if g.PartTotals.Missing != 0 || p[1].State != StateMine || p[1].Best.ID != 1 || p[3].Best.ID != 4 || p[3].Best.Q.Label != "1080p Remux ITA ENG" {
		t.Fatalf("no minimum: %+v %+v", g.PartTotals, g.Parts)
	}

	// At least 2160p: Alien³ and La clonazione are below it.
	g = alien()
	Apply(g, store.GapPrefs{GapThreshold: store.GapThreshold{Resolution: "2160p"}}, nil)
	p = parts(g)
	if g.PartTotals.Missing != 2 || g.PartTotals.Below != 2 || p[3].State != PartBelow || p[3].Best.ID != 4 {
		t.Fatalf("2160p: %+v %+v", g.PartTotals, g.Parts)
	}

	// Alien³ ignored, the encode of others accepted for La clonazione,
	// a torrent no longer on the tracker accepted for nothing.
	g = alien()
	marks := []store.GapMark{
		{Action: ActionIgnore, Unit: 8077},
		{Action: ActionAccept, Unit: 8078, Torrent: 12},
		{Action: ActionAccept, Unit: 679, Torrent: 99},
	}
	Apply(g, store.GapPrefs{GapThreshold: store.GapThreshold{Resolution: "2160p"}}, marks)
	p = parts(g)
	if g.PartTotals.Missing != 0 || p[3].Mark != MarkIgnored || p[4].Mark != MarkAccepted || p[4].Accepted != 12 {
		t.Fatalf("marks: %+v %+v", g.PartTotals, g.Parts)
	}
	// Marks for every row apply to the grid too: Alien³ and La clonazione
	// are no longer missing in the 2160p rows.
	if g.Missing != 3 {
		t.Fatalf("grid with marks: %d missing", g.Missing)
	}

	// Remux at least: the encode of others is below it, the user's Remux are fine.
	if !Meets(g.Units[3].Versions[0].Q, store.GapThreshold{Type: "Remux"}) {
		t.Errorf("remux below remux: %+v", g.Units[3].Versions[0])
	}
	enc := unit3d.QualityOf("300.2014.1080p.BluRay.x264.ITA.ENG", "Encode", "1080p", "")
	if Meets(enc, store.GapThreshold{Type: "Remux"}) || !Meets(enc, store.GapThreshold{Type: "Encode", Lang: "ITA"}) ||
		Meets(enc, store.GapThreshold{Lang: "FRE"}) || !Meets(unit3d.QualityOf("x.1080p.BluRay.x264", "Encode", "", ""), store.GapThreshold{Lang: "ITA"}) {
		t.Errorf("meets: %+v", enc)
	}
}

func TestApplyGrid(t *testing.T) {
	// A row hidden does not count.
	g := alien()
	Apply(g, store.GapPrefs{}, []store.GapMark{{Action: ActionHide, Quality: g.Rows[1].Quality.Key}})
	if !g.Rows[1].Hidden || g.Missing != 7-g.Rows[1].Missing || g.Hidden {
		t.Fatalf("hidden row: %+v, %d missing", g.Rows[1], g.Missing)
	}

	// What others uploaded in the same quality counts as present.
	g = alien()
	Apply(g, store.GapPrefs{OthersCount: true}, nil)
	last := g.Rows[2] // 1080p Remux: Alien and Aliens of others
	if last.Missing != 0 || last.Available != 2 || g.Missing != 5 {
		t.Fatalf("others count: %+v, %d missing", last, g.Missing)
	}

	// A torrent accepted in one row only.
	g = alien()
	Apply(g, store.GapPrefs{}, []store.GapMark{{Action: ActionAccept, Quality: g.Rows[2].Quality.Key, Unit: 348, Torrent: 10}})
	if c := g.Rows[2].Cells[0]; c.Mark != MarkAccepted || g.Rows[2].Missing != 1 || g.Rows[0].Cells[0].Mark != "" {
		t.Fatalf("accepted in a row: %+v", g.Rows[2])
	}

	// Marked complete, the whole group: until a new film comes out.
	g = alien()
	mark := []store.GapMark{{Action: ActionComplete, Value: CompleteValue(*g)}}
	Apply(g, store.GapPrefs{GapThreshold: store.GapThreshold{Resolution: "2160p"}}, mark)
	if !g.Completed || g.Missing != 0 || g.PartTotals.Missing != 0 || g.Rows[0].Cells[2].Mark != MarkComplete {
		t.Fatalf("complete: %+v %d", g.PartTotals, g.Missing)
	}
	g = alien()
	g.Units = append(g.Units, Unit{Number: 5, Name: "Alien: Romulus", TmdbID: 945961})
	g.buildRows(nil)
	Apply(g, store.GapPrefs{}, mark)
	if g.Completed || g.PartTotals.Missing != 1 || g.PartTotals.Absent != 1 {
		t.Fatalf("new film after complete: %v %+v", g.Completed, g.PartTotals)
	}

	// Hidden group, minimum of its own.
	g = alien()
	Apply(g, store.GapPrefs{}, []store.GapMark{{Action: ActionHide}, {Action: ActionThreshold, Value: `{"type":"Remux","resolution":"2160p"}`}})
	if !g.Hidden || !g.CustomThreshold || g.Threshold.Resolution != "2160p" || g.PartTotals.Missing != 2 {
		t.Fatalf("hidden, own minimum: %+v %+v", g.Threshold, g.PartTotals)
	}

	// Results of an older check: no view by part.
	g = alien()
	g.Format = 0
	Apply(g, store.GapPrefs{}, nil)
	if !g.Stale || g.Parts != nil {
		t.Fatalf("stale: %v %+v", g.Stale, g.Parts)
	}
}

func TestValidThreshold(t *testing.T) {
	got, err := ValidThreshold(store.GapThreshold{Type: "remux", Resolution: "1080P", Lang: " ita "})
	if err != nil || got != (store.GapThreshold{Type: "Remux", Resolution: "1080p", Lang: "ITA"}) {
		t.Fatalf("%+v %v", got, err)
	}
	if got, err := ValidThreshold(store.GapThreshold{Type: "webdl"}); err != nil || got.Type != "WEB-DL" {
		t.Fatalf("%+v %v", got, err)
	}
	for _, bad := range []store.GapThreshold{{Type: "Bluray"}, {Resolution: "1081p"}, {Lang: "it4"}} {
		if _, err := ValidThreshold(bad); err == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
}
