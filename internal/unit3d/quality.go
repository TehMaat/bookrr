package unit3d

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// Quality is what makes two releases of the same content interchangeable:
// same type (WEB-DL, Remux, Encode…), resolution, HDR formats and audio
// languages. "2160p WEB-DL DV HDR ITA ENG" is not "2160p WEB-DL DV HDR ITA"
// nor "2160p WEB-DL HDR ITA ENG".
type Quality struct {
	Key        string   `json:"key"`   // compares qualities
	Label      string   `json:"label"` // "2160p WEB-DL DV HDR ITA ENG"
	Type       string   `json:"type"`
	Resolution string   `json:"resolution"`
	HDR        []string `json:"hdr"`   // "DV", "HDR", "HLG"
	Langs      []string `json:"langs"` // "ITA", "ENG"…
}

// Group is the resolution and type alone ("2160p WEB-DL"), to filter by.
func (q Quality) Group() string { return strings.TrimSpace(q.Resolution + " " + q.Type) }

// Order of the values in labels; anything else follows alphabetically.
var (
	hdrOrder  = []string{"dv", "hdr", "hlg"}
	langOrder = []string{"ita", "eng"}
)

// Types as the parser reads them from a name, for torrents whose type the
// tracker does not give.
var sourceTypes = map[string]string{"remux": "Remux", "webdl": "WEB-DL", "webrip": "WEBRip", "bluray": "Encode", "hdtv": "HDTV"}

// QualityOf is the quality of a tracker torrent: type and resolution as
// the tracker gives them (from the name if it does not), HDR and languages
// from the name. A name that does not say the languages (or only "MULTI")
// takes the audio languages of its media info, if there is one.
func QualityOf(name, typ, resolution, mediaInfo string) Quality {
	r := parseRelease(name)
	typ = strings.TrimSpace(typ)
	if typ == "" {
		typ = sourceTypes[r.source]
	}
	resolution = strings.TrimSpace(resolution)
	if resolution == "" || strings.EqualFold(resolution, "other") {
		resolution = r.resolution
	}
	langs := r.langs
	if len(langs) == 0 || (len(langs) == 1 && langs["multi"]) {
		if ml := mediaInfoLangs(mediaInfo); len(ml) > 0 {
			langs = ml
		}
	}
	q := Quality{Type: typ, Resolution: strings.ToLower(resolution), HDR: ordered(r.hdr, hdrOrder), Langs: ordered(langs, langOrder)}
	q.Label = strings.Join(append(append([]string{q.Resolution, q.Type}, q.HDR...), q.Langs...), " ")
	q.Label = strings.Join(strings.Fields(q.Label), " ")
	q.Key = strings.Join([]string{q.Resolution, typeKey(q.Type), strings.Join(q.HDR, ","), strings.Join(q.Langs, ",")}, "|")
	return q
}

// typeKey compares types ignoring case and separators ("WEB-DL" = "WebDL").
func typeKey(t string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, t)
}

// ordered lists a set in upper case, the known values first in their order.
func ordered(s set, order []string) []string {
	out := []string{}
	for _, v := range order {
		if s[v] {
			out = append(out, strings.ToUpper(v))
		}
	}
	var rest []string
	for v := range s {
		if !contains(order, v) {
			rest = append(rest, strings.ToUpper(v))
		}
	}
	sort.Strings(rest)
	return append(out, rest...)
}

func contains(vs []string, v string) bool {
	for _, x := range vs {
		if x == v {
			return true
		}
	}
	return false
}

// Language names as MediaInfo writes them, with the codes of the parser.
var mediaLangs = map[string]string{
	"italian": "ita", "italiano": "ita", "english": "eng", "french": "fre", "german": "ger", "spanish": "spa",
	"castilian": "spa", "japanese": "jpn", "portuguese": "por", "russian": "rus", "korean": "kor", "chinese": "chi",
	"mandarin": "chi", "cantonese": "chi", "dutch": "dut", "polish": "pol", "swedish": "swe", "danish": "dan",
	"norwegian": "nor", "finnish": "fin", "turkish": "tur", "arabic": "ara", "hindi": "hin", "czech": "cze",
	"hungarian": "hun", "greek": "gre", "hebrew": "heb", "thai": "tha", "ukrainian": "ukr",
	"it": "ita", "en": "eng", "fr": "fre", "de": "ger", "es": "spa", "ja": "jpn",
}

var mediaSectionRe = regexp.MustCompile(`^(general|video|audio|text|menu|chapters?|image|other)(\s*#\d+)?$`)

// mediaInfoLangs reads the languages of the audio tracks of a MediaInfo
// report ("Audio #1" … "Language : Italian").
func mediaInfoLangs(info string) set {
	out := set{}
	inAudio := false
	for _, line := range strings.Split(info, "\n") {
		line = strings.TrimSpace(line)
		low := strings.ToLower(line)
		if mediaSectionRe.MatchString(low) {
			inAudio = strings.HasPrefix(low, "audio")
			continue
		}
		if !inAudio {
			continue
		}
		key, val, ok := strings.Cut(line, ":")
		if !ok || !strings.EqualFold(strings.TrimSpace(key), "language") {
			continue
		}
		val = strings.ToLower(strings.TrimSpace(val))
		// "English (US)", "Italian / Italiano"
		if i := strings.IndexAny(val, "(/,"); i > 0 {
			val = strings.TrimSpace(val[:i])
		}
		if code, ok := mediaLangs[val]; ok {
			out.add(code)
		} else if len(val) >= 3 {
			out.add(val[:3])
		}
	}
	return out
}

// Pack is the seasons a release name holds, for the season check.
type Pack struct {
	Seasons []int
	// Complete: "Complete" without season numbers, the whole series as it
	// was when uploaded.
	Complete bool
}

// ParsePack reads the seasons of a season pack ("S03"), of a pack of
// several seasons ("S01-S05", "Season 1-3", "Stagioni 1 e 2") or of the
// whole series ("Complete"). Episodes and names without seasons are not
// packs: ok is false.
func ParsePack(name string) (Pack, bool) {
	r := parseRelease(name)
	if !r.pack.isPack() {
		return Pack{}, false
	}
	return Pack{Seasons: r.pack.seasons, Complete: r.pack.complete}, true
}

// Episodic reports whether a release name speaks of seasons or episodes
// ("S01", "S01E02", "Season 2"): it is not a film. "Complete" alone is
// not enough, full discs say it too.
func Episodic(name string) bool {
	r := parseRelease(name)
	return r.season != "" || len(r.pack.seasons) > 0 || r.pack.episode
}

// Title is the title a release name starts with, in lower case
// ("Criminal.Minds.S19.2160p…" → "criminal minds").
func Title(name string) string { return parseRelease(name).title }
