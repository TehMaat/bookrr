// Package edition is the version (edition) of a film for its release name on
// Wayfinderr: Extended, Director's Cut... (Wayfinderr issue #108, bookrr #90).
package edition

import (
	"regexp"
	"strings"
	"unicode"
)

// MaxLen is the longest version Wayfinderr keeps.
const MaxLen = 60

// Versions are the versions offered, the same as Wayfinderr's (moverr's list).
var Versions = []string{
	"Theatrical",
	"Extended",
	"Director's Cut",
	"Unrated",
	"Special Edition",
	"Remastered",
	"IMAX",
	"Final Cut",
	"Uncut",
	"Anniversary Edition",
}

// Normalize trims a version, joins its spaces and caps it ("" none).
func Normalize(v string) string {
	v = strings.Join(strings.Fields(v), " ")
	if r := []rune(v); len(r) > MaxLen {
		v = strings.TrimSpace(string(r[:MaxLen]))
	}
	return v
}

// words is a name as lower-case words, apostrophes dropped: "Director’s.Cut" -> ["directors", "cut"].
func words(s string) []string {
	s = strings.NewReplacer("'", "", "’", "", "‘", "", "`", "", "´", "").Replace(strings.ToLower(s))
	return strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
}

// Same reports whether two versions give the same release name ("Director's Cut", "directors cut").
func Same(a, b string) bool { return strings.Join(words(a), ".") == strings.Join(words(b), ".") }

// The editions a release name can say, the first that matches wins
// ("Extended Director's Cut" is a Director's Cut).
var editions = []struct {
	re      *regexp.Regexp
	version string
}{
	{regexp.MustCompile(`\bdirectors? ?cut\b`), "Director's Cut"},
	{regexp.MustCompile(`\bfinal ?cut\b`), "Final Cut"},
	{regexp.MustCompile(`\bextended\b`), "Extended"},
	{regexp.MustCompile(`\bunrated\b`), "Unrated"},
	{regexp.MustCompile(`\buncut\b`), "Uncut"},
	{regexp.MustCompile(`\bspecial edition\b`), "Special Edition"},
	{regexp.MustCompile(`\b(\d+(st|nd|rd|th) )?anniversary( edition)?\b`), "Anniversary Edition"},
	{regexp.MustCompile(`\bremastered\b`), "Remastered"},
	{regexp.MustCompile(`\bimax\b`), "IMAX"},
	{regexp.MustCompile(`\btheatrical( cut| edition)?\b`), "Theatrical"},
}

// Of is the version a release name says after the film's title (one of
// titles, when the name starts with it), "" when it says none:
// "Orbita.Silenziosa.2010.Extended.Cut.2160p" -> "Extended". The words of
// the title are never taken for a version ("The.Final.Cut.2004").
func Of(name string, titles ...string) string {
	w := words(name)
	for _, t := range titles {
		tw := words(t)
		if len(tw) == 0 || len(tw) > len(w) {
			continue
		}
		match := true
		for i := range tw {
			if w[i] != tw[i] {
				match = false
				break
			}
		}
		if match {
			w = w[len(tw):]
			break
		}
	}
	text := strings.Join(w, " ")
	for _, e := range editions {
		if e.re.MatchString(text) {
			return e.version
		}
	}
	return ""
}
