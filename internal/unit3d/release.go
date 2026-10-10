package unit3d

import (
	"regexp"
	"strconv"
	"strings"
)

// release is what a scene-style name says about a torrent. File names and
// tracker titles write the same things differently ("DDP5.1" / "DD+ 5.1",
// "HDR10" / "HDR10+", "H.265" / "x265"): each value is normalized.
type release struct {
	title      string // words before the first year, season or format
	year       string
	season     string // "s19", "s01e02"
	resolution string
	source     string // remux, bluray, webdl, webrip, hdtv
	service    string // dsnp, nf, amzn…
	video      string // h264, h265, av1…
	hdr        set    // dv, hdr, hlg
	audio      set    // ddp, dd, truehd, dts…
	langs      set
	group      string // after the last "-"
}

type set map[string]bool

func (s set) add(v string) { s[v] = true }

func (s set) equal(o set) bool { return len(s) == len(o) && s.subsetOf(o) }

func (s set) subsetOf(o set) bool {
	for v := range s {
		if !o[v] {
			return false
		}
	}
	return true
}

// A value is a whole word: not preceded by a letter or digit, and not
// followed by a letter (digits may follow, as in "DDP5.1" or "DTS5.1").
const (
	pre  = `(?:^|[^a-z0-9])`
	post = `(?:$|[^a-z])`
)

func rx(body string) *regexp.Regexp { return regexp.MustCompile(pre + `(` + body + `)` + post) }

type rule struct {
	re    *regexp.Regexp
	value string
}

var (
	seasonRe     = regexp.MustCompile(pre + `s(\d{1,2})(?:e(\d{1,3}))?` + `(?:$|[^a-z0-9])`)
	yearInNameRe = regexp.MustCompile(pre + `((?:19|20)\d\d)(?:$|[^a-z0-9])`)
	resolutionRe = regexp.MustCompile(pre + `(2160|1080|720|576|480)[pi]` + `(?:$|[^a-z0-9])`)
	fourKRe      = rx(`4k|uhd`)
	groupRe      = regexp.MustCompile(`-([a-z0-9]+)$`)

	sourceRules = []rule{
		{rx(`remux`), "remux"},
		{rx(`web-?dl`), "webdl"},
		{rx(`web-?rip`), "webrip"},
		{rx(`blu-?ray|bdrip|brrip|bd`), "bluray"},
		{rx(`hdtv`), "hdtv"},
	}
	serviceRules = []rule{
		{rx(`dsnp|disney\+?`), "dsnp"},
		{rx(`nf|netflix`), "nf"},
		{rx(`amzn|amazon`), "amzn"},
		{rx(`atvp`), "atvp"},
		{rx(`hmax|max`), "max"},
		{rx(`pcok`), "pcok"},
		{rx(`hulu`), "hulu"},
		{rx(`now`), "now"},
		{rx(`rai(?:play)?`), "rai"},
		{rx(`mediaset|mhd`), "mediaset"},
	}
	videoRules = []rule{
		{rx(`[hx][ .]?265|hevc`), "h265"},
		{rx(`[hx][ .]?264|avc`), "h264"},
		{rx(`av1`), "av1"},
		{rx(`vc-?1`), "vc1"},
		{rx(`mpeg-?2`), "mpeg2"},
	}
	hdrRules = []rule{
		{rx(`dv|dovi|dolby[ .]?vision`), "dv"},
		{regexp.MustCompile(pre + `(hdr(?:10)?\+?)` + `(?:$|[^a-z0-9])`), "hdr"},
		{rx(`hlg`), "hlg"},
	}
	// DD+ before DD, so that "DD+" is not also read as "DD".
	audioRules = []rule{
		{regexp.MustCompile(pre + `(ddp|dd\+|e-?ac-?3)`), "ddp"},
		{rx(`truehd`), "truehd"},
		{rx(`dts(?:-?hd(?:[ .-]?ma)?|-?x)?`), "dts"},
		{rx(`dd|ac-?3`), "dd"},
		{rx(`aac`), "aac"},
		{rx(`flac`), "flac"},
		{rx(`opus`), "opus"},
		{rx(`l?pcm`), "pcm"},
		{rx(`mp3`), "mp3"},
	}
	langRules = []rule{
		{rx(`ita`), "ita"}, {rx(`eng`), "eng"}, {rx(`fre|fra|french`), "fre"}, {rx(`ger|deu|german`), "ger"},
		{rx(`spa|esp|spanish`), "spa"}, {rx(`jpn|jap`), "jpn"}, {rx(`multi`), "multi"},
	}
)

// Words that end a name after a "-" without being a release group.
var notGroup = map[string]bool{"dl": true, "rip": true, "hd": true, "ma": true, "x": true, "ray": true, "1": true, "3": true}

// findAll is FindAllStringSubmatchIndex letting a match start on the
// separator that ended the previous one ("2012.2009" has two years).
func findAll(re *regexp.Regexp, s string) [][]int {
	var out [][]int
	for off := 0; off < len(s); {
		m := re.FindStringSubmatchIndex(s[off:])
		if m == nil {
			break
		}
		for i := range m {
			if m[i] >= 0 {
				m[i] += off
			}
		}
		out = append(out, m)
		off = m[3]
	}
	return out
}

func parseRelease(name string) release {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.TrimSuffix(s, ".torrent")
	r := release{hdr: set{}, audio: set{}, langs: set{}}
	first := len(s) // where the title ends
	mark := func(loc []int) {
		if loc != nil && loc[2] < first {
			first = loc[2]
		}
	}

	if m := seasonRe.FindStringSubmatchIndex(s); m != nil {
		n, _ := strconv.Atoi(s[m[2]:m[3]])
		r.season = "s" + strconv.Itoa(n)
		if m[4] >= 0 {
			e, _ := strconv.Atoi(s[m[4]:m[5]])
			r.season += "e" + strconv.Itoa(e)
		}
		mark([]int{0, 0, m[2] - 1}) // the "s" before the number
	}
	// A year at the very start is part of the title ("2012", "1917").
	for _, m := range findAll(yearInNameRe, s) {
		if m[2] > 0 {
			r.year = s[m[2]:m[3]]
			mark(m)
			break
		}
	}
	if m := resolutionRe.FindStringSubmatchIndex(s); m != nil {
		r.resolution = s[m[2]:m[3]] + "p"
		mark(m)
	} else if m := fourKRe.FindStringSubmatchIndex(s); m != nil {
		r.resolution = "2160p"
		mark(m)
	}
	one := func(rules []rule) string {
		for _, ru := range rules {
			if m := ru.re.FindStringSubmatchIndex(s); m != nil {
				mark(m)
				return ru.value
			}
		}
		return ""
	}
	r.source = one(sourceRules)
	r.video = one(videoRules)
	many := func(rules []rule, into set, text string) {
		for _, ru := range rules {
			if ru.re.MatchString(text) {
				into.add(ru.value)
				text = ru.re.ReplaceAllString(text, " ")
			}
		}
	}
	many(hdrRules, r.hdr, s)
	many(audioRules, r.audio, s)
	many(langRules, r.langs, s)
	// Short codes like "NF", "NOW" or "MAX" only count after the title.
	for _, ru := range serviceRules {
		for _, m := range findAll(ru.re, s) {
			if m[2] >= first {
				r.service = ru.value
				break
			}
		}
		if r.service != "" {
			break
		}
	}
	if m := groupRe.FindStringSubmatch(s); m != nil && !notGroup[m[1]] {
		r.group = m[1]
	}
	if first < len(s) {
		r.title = strings.Join(tokens(s[:first]), " ")
	}
	return r
}

// sameRelease reports whether a file name and a tracker title describe the
// same release: same title, season, year, resolution, source, HDR and
// video codec, the file's audio and languages all in the title, and the
// same group. What only one of the two names says is not compared, except
// for season and HDR: a season pack is not an episode, SDR is not HDR.
func sameRelease(file, tracker string) bool {
	a, b := parseRelease(file), parseRelease(tracker)
	if a.title == "" || a.title != b.title || a.season != b.season || !a.hdr.equal(b.hdr) {
		return false
	}
	// A file name with nothing but the title says too little.
	if a.resolution == "" && a.source == "" && a.video == "" {
		return false
	}
	both := func(x, y string) bool { return x == "" || y == "" || x == y }
	return both(a.year, b.year) && both(a.resolution, b.resolution) && both(a.source, b.source) &&
		both(a.service, b.service) && both(a.video, b.video) && both(a.group, b.group) &&
		a.audio.subsetOf(b.audio) && a.langs.subsetOf(b.langs)
}
