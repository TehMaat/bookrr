// Package gaps tells what is missing from the collections the user has
// uploaded to the tracker, in the same quality: the seasons of a series
// (and, later, the movies of a saga), and whether others uploaded them.
package gaps

import (
	"sort"
	"strconv"
	"strings"

	"github.com/tehmaat/bookrr/internal/unit3d"
)

// Kinds of collection.
const KindSeries = "series"

// States of a cell of the grid.
const (
	StateMine         = "mine"          // uploaded by the user in this quality
	StateAvailable    = "available"     // uploaded by others in this quality
	StateOtherQuality = "other_quality" // on the tracker, only in other qualities
	StateAbsent       = "absent"        // not on the tracker at all
)

// Statuses of a collection.
const (
	StatusOK           = "ok"
	StatusUnverifiable = "unverifiable" // its parts are unknown (no TMDB id, not on TMDB)
)

// Group is a collection of the user's uploads (a series) with, for every
// quality the user uploaded it in, the grid of its parts (the seasons).
type Group struct {
	Kind   string `json:"kind"`
	Key    string `json:"key"` // "tmdb:1399"
	Title  string `json:"title"`
	Year   string `json:"year"`
	TmdbID int64  `json:"tmdbId"`
	TvdbID int64  `json:"tvdbId"`
	Status string `json:"status"`
	// Why it is unverifiable, or what else should be known.
	Note string `json:"note"`
	// The collection on the tracker (search by TMDB id).
	SearchURL string `json:"searchUrl"`
	Units     []Unit `json:"units"`
	Rows      []Row  `json:"rows"`
	// The user's uploads it was made of.
	Mine []Ref `json:"mine"`
	// Totals over the rows: cells that are not the user's, and how they stand.
	Missing      int    `json:"missing"`
	Available    int    `json:"available"`
	OtherQuality int    `json:"otherQuality"`
	Absent       int    `json:"absent"`
	CheckedAt    string `json:"checkedAt"`
}

// Unit is a part of a collection: a season of a series.
type Unit struct {
	Number   int    `json:"number"`
	Name     string `json:"name"`
	AirDate  string `json:"airDate"`
	Episodes int    `json:"episodes"`
}

// Row is the grid of one quality the user uploaded the collection in.
type Row struct {
	Quality      unit3d.Quality `json:"quality"`
	Cells        []Cell         `json:"cells"`
	Missing      int            `json:"missing"`
	Available    int            `json:"available"`
	OtherQuality int            `json:"otherQuality"`
	Absent       int            `json:"absent"`
}

// Cell is how a part stands in a quality.
type Cell struct {
	Unit  int    `json:"unit"` // Unit.Number
	State string `json:"state"`
	// The torrents that hold it: in this quality for mine and available,
	// in the others for other_quality.
	Torrents []Ref `json:"torrents"`
	// Labels of the qualities it exists in, for other_quality.
	Qualities []string `json:"qualities"`
}

// Ref is a torrent on the tracker.
type Ref struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	URL     string `json:"url"`
	Quality string `json:"quality"` // label
	Mine    bool   `json:"mine"`
}

// item is a tracker torrent with the parts of the collection it holds.
type item struct {
	ref     Ref
	quality unit3d.Quality
	units   map[int]bool
}

// buildRows makes one row per quality of the user's items, best first,
// and fills in the totals of the group.
func (g *Group) buildRows(items []item) {
	qualities := map[string]unit3d.Quality{}
	for _, it := range items {
		if it.ref.Mine {
			qualities[it.quality.Key] = it.quality
		}
	}
	g.Rows = []Row{}
	for _, q := range qualities {
		row := Row{Quality: q, Cells: make([]Cell, 0, len(g.Units))}
		for _, u := range g.Units {
			c := cellFor(u.Number, q, items)
			switch c.State {
			case StateAvailable:
				row.Available++
			case StateOtherQuality:
				row.OtherQuality++
			case StateAbsent:
				row.Absent++
			}
			if c.State != StateMine {
				row.Missing++
			}
			row.Cells = append(row.Cells, c)
		}
		g.Rows = append(g.Rows, row)
	}
	sort.Slice(g.Rows, func(i, j int) bool { return better(g.Rows[i].Quality, g.Rows[j].Quality) })
	g.Missing, g.Available, g.OtherQuality, g.Absent = 0, 0, 0, 0
	for _, r := range g.Rows {
		g.Missing += r.Missing
		g.Available += r.Available
		g.OtherQuality += r.OtherQuality
		g.Absent += r.Absent
	}
}

func cellFor(unit int, q unit3d.Quality, items []item) Cell {
	c := Cell{Unit: unit, Torrents: []Ref{}, Qualities: []string{}}
	var mine, same, other []Ref
	labels := map[string]bool{}
	for _, it := range items {
		if !it.units[unit] {
			continue
		}
		switch {
		case it.quality.Key == q.Key && it.ref.Mine:
			mine = append(mine, it.ref)
		case it.quality.Key == q.Key:
			same = append(same, it.ref)
		default:
			other = append(other, it.ref)
			labels[it.quality.Label] = true
		}
	}
	switch {
	case len(mine) > 0:
		c.State, c.Torrents = StateMine, mine
	case len(same) > 0:
		c.State, c.Torrents = StateAvailable, same
	case len(other) > 0:
		c.State, c.Torrents = StateOtherQuality, other
		for l := range labels {
			c.Qualities = append(c.Qualities, l)
		}
		sort.Strings(c.Qualities)
	default:
		c.State = StateAbsent
	}
	return c
}

// better orders qualities by resolution (higher first), then by label.
func better(a, b unit3d.Quality) bool {
	ra, rb := resolutionRank(a.Resolution), resolutionRank(b.Resolution)
	if ra != rb {
		return ra > rb
	}
	return a.Label < b.Label
}

func resolutionRank(r string) int {
	n, _ := strconv.Atoi(strings.TrimRight(r, "pi"))
	return n
}
