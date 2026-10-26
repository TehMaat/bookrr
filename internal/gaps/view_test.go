package gaps

import (
	"strings"
	"testing"

	"github.com/tehmaat/bookrr/internal/store"
	"github.com/tehmaat/bookrr/internal/unit3d"
)

// it is a torrent holding these parts.
func it(id int64, name string, mine bool, units ...int) item {
	u := map[int]bool{}
	for _, n := range units {
		u[n] = true
	}
	q := unit3d.QualityOf(name, "", "", "")
	return item{ref: Ref{ID: id, Name: name, Quality: q.Label, Mine: mine}, quality: q, units: u}
}

// alien is the saga of the example: the user uploaded every film in the
// best quality there is, Alien³ and Alien - La clonazione only in 1080p.
// extra are more torrents, of the films or of more units.
func alien(extra ...item) *Group {
	g := &Group{Kind: KindSaga, Key: "collection:8091", Status: StatusOK, Units: []Unit{
		{Number: 1, Name: "Alien", TmdbID: 348},
		{Number: 2, Name: "Aliens", TmdbID: 679},
		{Number: 3, Name: "Alien³", TmdbID: 8077},
		{Number: 4, Name: "Alien - La clonazione", TmdbID: 8078},
	}}
	g.buildRows(append([]item{
		it(1, "Alien.1979.2160p.BluRay.REMUX.DV.HDR.ITA.ENG", true, 1),
		it(2, "Aliens.1986.2160p.BluRay.REMUX.DV.HDR.ITA.ENG", true, 2),
		it(3, "Alien.1979.2160p.BluRay.REMUX.HDR.ITA.ENG", true, 1),
		it(4, "Alien3.1992.1080p.BluRay.REMUX.ITA.ENG", true, 3),
		it(5, "Alien.Resurrection.1997.1080p.BluRay.REMUX.ITA.ENG", true, 4),
		it(10, "Alien.1979.1080p.BluRay.REMUX.ITA.ENG", false, 1),
		it(11, "Aliens.1986.1080p.BluRay.REMUX.ITA.ENG", false, 2),
		it(12, "Alien.Resurrection.1997.1080p.BluRay.x264.ITA.ENG", false, 4),
	}, extra...))
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
	Apply(g, store.GapPrefs{}, nil, nil)
	p := parts(g)
	if g.PartTotals.Missing != 0 || p[1].State != StateMine || p[1].Best.ID != 1 || p[3].Best.ID != 4 || p[3].Best.Q.Label != "1080p Remux ITA ENG" {
		t.Fatalf("no minimum: %+v %+v", g.PartTotals, g.Parts)
	}

	// At least 2160p: Alien³ and La clonazione are below it.
	g = alien()
	Apply(g, store.GapPrefs{GapThreshold: store.GapThreshold{Resolution: "2160p"}}, nil, nil)
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
	Apply(g, store.GapPrefs{GapThreshold: store.GapThreshold{Resolution: "2160p"}}, marks, nil)
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
	Apply(g, store.GapPrefs{}, []store.GapMark{{Action: ActionHide, Quality: g.Rows[1].Quality.Key}}, nil)
	if !g.Rows[1].Hidden || g.Missing != 7-g.Rows[1].Missing || g.Hidden {
		t.Fatalf("hidden row: %+v, %d missing", g.Rows[1], g.Missing)
	}

	// What others uploaded in the same quality counts as present.
	g = alien()
	Apply(g, store.GapPrefs{OthersCount: true}, nil, nil)
	last := g.Rows[2] // 1080p Remux: Alien and Aliens of others
	if last.Missing != 0 || last.Available != 2 || g.Missing != 5 {
		t.Fatalf("others count: %+v, %d missing", last, g.Missing)
	}

	// A torrent accepted in one row only.
	g = alien()
	Apply(g, store.GapPrefs{}, []store.GapMark{{Action: ActionAccept, Quality: g.Rows[2].Quality.Key, Unit: 348, Torrent: 10}}, nil)
	if c := g.Rows[2].Cells[0]; c.Mark != MarkAccepted || g.Rows[2].Missing != 1 || g.Rows[0].Cells[0].Mark != "" {
		t.Fatalf("accepted in a row: %+v", g.Rows[2])
	}

	// Marked complete, the whole group: until a new film comes out.
	g = alien()
	mark := []store.GapMark{{Action: ActionComplete, Value: CompleteValue(*g)}}
	Apply(g, store.GapPrefs{GapThreshold: store.GapThreshold{Resolution: "2160p"}}, mark, nil)
	if !g.Completed || g.Missing != 0 || g.PartTotals.Missing != 0 || g.Rows[0].Cells[2].Mark != MarkComplete {
		t.Fatalf("complete: %+v %d", g.PartTotals, g.Missing)
	}
	g = alien()
	g.Units = append(g.Units, Unit{Number: 5, Name: "Alien: Romulus", TmdbID: 945961})
	g.buildRows(nil)
	Apply(g, store.GapPrefs{}, mark, nil)
	if g.Completed || g.PartTotals.Missing != 1 || g.PartTotals.Absent != 1 {
		t.Fatalf("new film after complete: %v %+v", g.Completed, g.PartTotals)
	}

	// Hidden group, minimum of its own.
	g = alien()
	Apply(g, store.GapPrefs{}, []store.GapMark{{Action: ActionHide}, {Action: ActionThreshold, Value: `{"type":"Remux","resolution":"2160p"}`}}, nil)
	if !g.Hidden || !g.CustomThreshold || g.Threshold.Resolution != "2160p" || g.PartTotals.Missing != 2 {
		t.Fatalf("hidden, own minimum: %+v %+v", g.Threshold, g.PartTotals)
	}

	// Results of an older check: no view by part.
	g = alien()
	g.Format = 0
	Apply(g, store.GapPrefs{}, nil, nil)
	if !g.Stale || g.Parts != nil {
		t.Fatalf("stale: %v %+v", g.Stale, g.Parts)
	}
}

func TestApplyBanned(t *testing.T) {
	banned := BannedSet([]string{"FLUX"})

	// Alien³ in 2160p Remux HDR only by FLUX: missing even when what others
	// uploaded counts, banned.
	g := alien(it(20, "Alien3.1992.2160p.BluRay.REMUX.HDR.ITA.ENG-FLUX", false, 3))
	Apply(g, store.GapPrefs{OthersCount: true}, nil, banned)
	var row Row
	for _, r := range g.Rows {
		if r.Quality.Label == "2160p Remux HDR ITA ENG" {
			row = r
		}
	}
	if c := row.Cells[2]; c.State != StateBanned || !c.Torrents[0].Banned || c.Torrents[0].Group != "FLUX" ||
		row.Banned != 1 || row.Missing != 3 || g.Banned != 1 {
		t.Fatalf("grid: %+v", row)
	}
	if g.Rows[2].Available != 2 || g.Rows[2].Missing != 0 {
		t.Fatalf("others not banned: %+v", g.Rows[2])
	}

	// At least 2160p, Alien³ is there only by FLUX (and below by the user).
	g = alien(it(20, "Alien3.1992.2160p.BluRay.REMUX.HDR.ITA.ENG-FLUX", false, 3), it(21, "Alien.Resurrection.1997.2160p.WEB-DL.ITA.ENG-GRP", false, 4))
	Apply(g, store.GapPrefs{GapThreshold: store.GapThreshold{Resolution: "2160p"}, OthersCount: true}, nil, banned)
	p := parts(g)
	if p[3].State != StateBanned || p[3].Best.ID != 20 || !p[3].Best.Banned || p[4].State != StateAvailable || p[4].Best.ID != 21 ||
		g.PartTotals.Banned != 1 || g.PartTotals.Available != 1 || g.PartTotals.Missing != 1 {
		t.Fatalf("2160p: %+v %+v", g.Parts, g.PartTotals)
	}
	// Without bans, the same release is of others like any other.
	g = alien(it(20, "Alien3.1992.2160p.BluRay.REMUX.HDR.ITA.ENG-FLUX", false, 3))
	Apply(g, store.GapPrefs{GapThreshold: store.GapThreshold{Resolution: "2160p"}}, nil, nil)
	if p := parts(g)[3]; p.State != StateAvailable || p.Best.Banned || p.Best.Group != "FLUX" {
		t.Fatalf("no bans: %+v", p)
	}
	// The user's own releases are never banned.
	g = alien()
	g.Mine = []Ref{{Name: "Alien.1979.2160p.BluRay.REMUX.HDR.ITA.ENG-MaTiTa", Mine: true}}
	Apply(g, store.GapPrefs{}, nil, BannedSet([]string{"MaTiTa"}))
	if g.Mine[0].Group != "MaTiTa" || g.Mine[0].Banned {
		t.Fatalf("mine: %+v", g.Mine[0])
	}
}

func TestValidBannedGroups(t *testing.T) {
	got, err := ValidBannedGroups([]string{" FLUX ", "-grp", "", "flux", "Arc"})
	if err != nil || strings.Join(got, ",") != "Arc,FLUX,grp" {
		t.Fatalf("%v %v", got, err)
	}
	if _, err := ValidBannedGroups([]string{"a b"}); err == nil {
		t.Error("space accepted")
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
