package gaps

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"unicode"

	"github.com/tehmaat/bookrr/internal/store"
	"github.com/tehmaat/bookrr/internal/unit3d"
)

// Actions of the user's choices (store.GapMark).
const (
	ActionHide      = "hide"      // the collection, or one of its quality rows
	ActionComplete  = "complete"  // the collection, as it is now
	ActionAccept    = "accept"    // a torrent of others for a part
	ActionIgnore    = "ignore"    // a part
	ActionThreshold = "threshold" // a minimum quality of the collection's own
)

// Part is how a part stands in the view by part: its best version against
// the minimum quality, whatever the quality rows.
type Part struct {
	Unit  int    `json:"unit"`  // Unit.Number
	State string `json:"state"` // mine, available, below, absent
	// The version shown: the user's best that reaches the minimum, else
	// the best of others that does, else the best of all.
	Best     *Version `json:"best"`
	Mark     string   `json:"mark"`
	Accepted int64    `json:"accepted"`
}

// Types from best to worst, for the minimum quality.
var TypeLadder = []string{"Full Disc", "Remux", "Encode", "WEB-DL", "WEBRip", "HDTV"}

// Resolutions the minimum quality can ask for.
var Resolutions = []string{"480p", "576p", "720p", "1080p", "2160p", "4320p"}

var typeRanks = map[string]int{"fulldisc": 6, "remux": 5, "encode": 4, "webdl": 3, "web": 3, "webrip": 2, "hdtv": 1}

// typeRank orders the types ("WEB-DL" = "WebDL"); unknown ones are lowest.
func typeRank(t string) int {
	return typeRanks[strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, t)]
}

// hdrRank: Dolby Vision is worth more than the other formats.
func hdrRank(hdr []string) int {
	n := 0
	for _, h := range hdr {
		if strings.EqualFold(h, "DV") {
			n += 2
		} else {
			n++
		}
	}
	return n
}

// betterVersion orders versions best first: type, resolution, HDR, number
// of languages; then the user's, then the oldest.
func betterVersion(a, b Version) bool {
	if x, y := typeRank(a.Q.Type), typeRank(b.Q.Type); x != y {
		return x > y
	}
	if x, y := resolutionRank(a.Q.Resolution), resolutionRank(b.Q.Resolution); x != y {
		return x > y
	}
	if x, y := hdrRank(a.Q.HDR), hdrRank(b.Q.HDR); x != y {
		return x > y
	}
	if x, y := len(a.Q.Langs), len(b.Q.Langs); x != y {
		return x > y
	}
	if a.Mine != b.Mine {
		return a.Mine
	}
	return a.ID < b.ID
}

// Meets reports whether a quality reaches the minimum. A torrent whose
// languages are unknown (or only "MULTI") is given the benefit of the doubt.
func Meets(q unit3d.Quality, t store.GapThreshold) bool {
	if t.Type != "" && typeRank(q.Type) < typeRank(t.Type) {
		return false
	}
	if t.Resolution != "" && resolutionRank(q.Resolution) < resolutionRank(t.Resolution) {
		return false
	}
	if t.Lang == "" || len(q.Langs) == 0 || (len(q.Langs) == 1 && strings.EqualFold(q.Langs[0], "MULTI")) {
		return true
	}
	for _, l := range q.Langs {
		if strings.EqualFold(l, t.Lang) {
			return true
		}
	}
	return false
}

// ValidThreshold checks a minimum quality and writes it the usual way
// ("remux" → "Remux", "ita" → "ITA").
func ValidThreshold(t store.GapThreshold) (store.GapThreshold, error) {
	out := store.GapThreshold{}
	if t.Type = strings.TrimSpace(t.Type); t.Type != "" {
		for _, v := range TypeLadder {
			if typeRank(v) == typeRank(t.Type) && typeRank(v) > 0 {
				out.Type = v
			}
		}
		if out.Type == "" {
			return out, errors.New("tipo minimo non valido: " + t.Type)
		}
	}
	if t.Resolution = strings.ToLower(strings.TrimSpace(t.Resolution)); t.Resolution != "" {
		for _, v := range Resolutions {
			if v == t.Resolution {
				out.Resolution = v
			}
		}
		if out.Resolution == "" {
			return out, errors.New("risoluzione minima non valida: " + t.Resolution)
		}
	}
	if t.Lang = strings.ToUpper(strings.TrimSpace(t.Lang)); t.Lang != "" {
		if len(t.Lang) < 2 || len(t.Lang) > 8 || strings.IndexFunc(t.Lang, func(r rune) bool { return r < 'A' || r > 'Z' }) >= 0 {
			return out, errors.New("lingua non valida: " + t.Lang)
		}
		out.Lang = t.Lang
	}
	return out, nil
}

// CompleteValue is the value of a "complete" mark: the parts of the
// collection now.
func CompleteValue(g Group) string {
	ids := make([]string, len(g.Units))
	for i, u := range g.Units {
		ids[i] = strconv.FormatInt(UnitID(u), 10)
	}
	return strings.Join(ids, ",")
}

type markKey struct {
	quality string
	unit    int64
}

// Apply adds the user's options and choices to a stored group: the hidden
// rows, the marks of the cells, the view by part against the minimum
// quality, and the totals that leave out what the user does not want
// counted. marks are those of this group.
func Apply(g *Group, prefs store.GapPrefs, marks []store.GapMark) {
	g.Threshold = prefs.GapThreshold
	var completed map[int64]bool
	hiddenRows := map[string]bool{}
	accepted := map[markKey]int64{}
	ignored := map[markKey]bool{}
	for _, m := range marks {
		k := markKey{m.Quality, m.Unit}
		switch m.Action {
		case ActionHide:
			if m.Quality == "" {
				g.Hidden = true
			} else {
				hiddenRows[m.Quality] = true
			}
		case ActionComplete:
			completed = map[int64]bool{}
			for _, f := range strings.Split(m.Value, ",") {
				if n, err := strconv.ParseInt(f, 10, 64); err == nil {
					completed[n] = true
				}
			}
		case ActionAccept:
			accepted[k] = m.Torrent
		case ActionIgnore:
			ignored[k] = true
		case ActionThreshold:
			var t store.GapThreshold
			if json.Unmarshal([]byte(m.Value), &t) == nil {
				g.Threshold, g.CustomThreshold = t, true
			}
		}
	}
	ids := map[int]int64{}
	for i := range g.Units {
		u := &g.Units[i]
		u.ID = UnitID(*u)
		ids[u.Number] = u.ID
		if u.Versions == nil {
			u.Versions = []Version{}
		}
	}
	if completed != nil {
		g.Completed = true
		for _, u := range g.Units {
			if !completed[u.ID] {
				g.Completed = false // a part came out after
			}
		}
	}
	units := map[int64]Unit{}
	for _, u := range g.Units {
		units[u.ID] = u
	}
	// The choice on a part in a row ("" for every row and the view by
	// part): the row's own first. An accepted torrent no longer on the
	// tracker does not count.
	markOf := func(quality string, unit int64, cell []Ref) (mark string, torrent int64, scope string) {
		for _, q := range []string{quality, ""} {
			k := markKey{q, unit}
			if ignored[k] {
				return MarkIgnored, 0, q
			}
			if t, ok := accepted[k]; ok && holds(units[unit], cell, t) {
				return MarkAccepted, t, q
			}
			if q == "" {
				break
			}
		}
		if completed[unit] {
			return MarkComplete, 0, ""
		}
		return "", 0, ""
	}

	g.Missing, g.Available, g.OtherQuality, g.Absent = 0, 0, 0, 0
	for ri := range g.Rows {
		r := &g.Rows[ri]
		r.Hidden = hiddenRows[r.Quality.Key]
		r.Missing, r.Available, r.OtherQuality, r.Absent = 0, 0, 0, 0
		for ci := range r.Cells {
			c := &r.Cells[ci]
			c.Mark, c.Accepted, c.MarkQuality = "", 0, ""
			if c.State == StateMine {
				continue
			}
			if c.Mark, c.Accepted, c.MarkQuality = markOf(r.Quality.Key, ids[c.Unit], c.Torrents); c.Mark != "" {
				continue
			}
			switch c.State {
			case StateAvailable:
				r.Available++
			case StateOtherQuality:
				r.OtherQuality++
			case StateAbsent:
				r.Absent++
			}
			if c.State != StateAvailable || !prefs.OthersCount {
				r.Missing++
			}
		}
		if !r.Hidden {
			g.Missing += r.Missing
			g.Available += r.Available
			g.OtherQuality += r.OtherQuality
			g.Absent += r.Absent
		}
	}

	g.PartTotals = Totals{}
	g.Stale = g.Status == StatusOK && g.Format < Format
	if g.Stale || g.Status != StatusOK {
		g.Parts = nil
		return
	}
	g.Parts = make([]Part, 0, len(g.Units))
	for ui := range g.Units {
		u := &g.Units[ui]
		for i := range u.Versions {
			u.Versions[i].Meets = Meets(u.Versions[i].Q, g.Threshold)
		}
		p := partOf(*u, g.Threshold)
		if p.State != StateMine {
			p.Mark, p.Accepted, _ = markOf("", u.ID, nil)
		}
		if p.State != StateMine && p.Mark == "" {
			switch p.State {
			case StateAvailable:
				g.PartTotals.Available++
			case PartBelow:
				g.PartTotals.Below++
			case StateAbsent:
				g.PartTotals.Absent++
			}
			if p.State != StateAvailable || !prefs.OthersCount {
				g.PartTotals.Missing++
			}
		}
		g.Parts = append(g.Parts, p)
	}
}

// holds reports whether a part (or the cell, for groups of older checks)
// still has the torrent.
func holds(u Unit, cell []Ref, id int64) bool {
	for _, v := range u.Versions {
		if v.ID == id {
			return true
		}
	}
	for _, r := range cell {
		if r.ID == id {
			return true
		}
	}
	return false
}

// partOf is how a part stands against the minimum quality.
func partOf(u Unit, t store.GapThreshold) Part {
	p := Part{Unit: u.Number, State: StateAbsent}
	var mine, others, mineAny, all *Version
	pick := func(best **Version, v *Version) {
		if *best == nil || betterVersion(*v, **best) {
			*best = v
		}
	}
	for i := range u.Versions {
		v := &u.Versions[i]
		ok := Meets(v.Q, t)
		switch {
		case v.Mine && ok:
			pick(&mine, v)
		case ok:
			pick(&others, v)
		}
		if v.Mine {
			pick(&mineAny, v)
		}
		pick(&all, v)
	}
	switch {
	case mine != nil:
		p.State, p.Best = StateMine, mine
	case others != nil:
		p.State, p.Best = StateAvailable, others
	case mineAny != nil:
		p.State, p.Best = PartBelow, mineAny
	case all != nil:
		p.State, p.Best = PartBelow, all
	}
	if p.Best != nil {
		b := *p.Best
		p.Best = &b
	}
	return p
}
