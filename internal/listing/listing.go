// Package listing reads the list of the files of a disk as printed by the
// usual commands (ls, find, dir, Get-ChildItem) on the computer the disk is
// plugged into, and finds the torrents stored there by their name, as the
// buckets of the cloud archives are read.
package listing

import (
	"errors"
	"math"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Formats recognized.
const (
	FormatPowerShell = "Get-ChildItem"
	FormatDir        = "dir"
	FormatLsLong     = "ls -l"
	FormatSizes      = "size and path" // find -printf '%s %P\n', du -ab
	FormatLs         = "ls -R"
	FormatPaths      = "paths" // find, dir /s /b
)

// Entry is a file or a folder of the listing.
type Entry struct {
	// Relative to the folder listed, with "/" between the folders.
	Path string
	Dir  bool
	// Size in bytes; Sized is false when the listing has no sizes.
	Size  int64
	Sized bool
	// Err is how far Size can be from the real size, for sizes like
	// "4.2G" (0 when exact).
	Err int64
}

// Listing is a parsed listing.
type Listing struct {
	Format string
	// The folder listed, as the listing names it ("" when unknown).
	Root    string
	Entries []Entry
}

// ErrEmpty is a listing with no file in it.
var ErrEmpty = errors.New("no file found in the listing: paste the output of ls -lR, find, dir /s or Get-ChildItem -Recurse")

var (
	psHeader    = regexp.MustCompile(`^\s*Mode\s+LastWriteTime\s+Length\s+Name\s*$`)
	psDirLine   = regexp.MustCompile(`^\s*[^\s:]+:\s+(\S.*?)\s*$`)
	psFile      = regexp.MustCompile(`^[-a-ce-zA-Z][-a-zA-Z]{3,5}\s+.+?\s(\d+)\s(.+)$`)
	psFolder    = regexp.MustCompile(`^d[-a-zA-Z]{3,5}\s+\S+\s+\S+(?:\s+[AaPp]\.?[Mm]\.?)?\s+(.+)$`)
	winPath     = `(?:[A-Za-z]:\\|\\\\)`
	dirHeader   = regexp.MustCompile(`^\s+\S.*?\s(` + winPath + `.*?)\s*$`)
	dirEntry    = regexp.MustCompile(`^(\d\S*)\s+(\d\S*(?:\s?[AaPp]\.?[Mm]\.?)?)\s+(.+)$`)
	dirSize     = regexp.MustCompile(`^(\d{1,3}(?:[.,\x{a0}\x{202f} ]\d{3})+|\d+)\s(.+)$`)
	lsLong      = regexp.MustCompile(`^([-dlbcpsD])[-rwxsStTlL+.@]{9}[+.@]?\s+\d+\s+(.*)$`)
	isoDate     = regexp.MustCompile(`^\d{4}-\d\d-\d\d$`)
	timeZone    = regexp.MustCompile(`^[+-]\d{4}$`)
	clockOrYear = regexp.MustCompile(`^(\d{1,2}[:.]\d\d|\d{4})$`)
	sizePath    = regexp.MustCompile(`^(\d+)\s+(\S.*)$`)
	humanSize   = regexp.MustCompile(`^(\d+(?:[.,]\d+)?)([KMGTP]?)i?B?$`)
	lsHeader    = regexp.MustCompile(`^(.+):$`)
	lsTotal     = regexp.MustCompile(`^(total|totale|insgesamt|totaal)\s+\S+$`)
	dirSummary  = regexp.MustCompile(`^\s+\d+\s`)
)

// Parse reads a listing, recognizing its format.
func Parse(text string) (Listing, error) {
	text = strings.TrimPrefix(text, "\ufeff")
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, "\r")
	}
	var l Listing
	switch detect(lines) {
	case FormatPowerShell:
		l = parsePowerShell(lines)
	case FormatDir:
		l = parseDir(lines)
	case FormatLsLong:
		l = parseLs(lines, true)
	case FormatSizes:
		l = parseSizes(lines)
	case FormatLs:
		l = parseLs(lines, false)
	default:
		l = parsePaths(lines)
	}
	l.Entries = clean(l.Entries)
	files := 0
	for _, e := range l.Entries {
		if !e.Dir {
			files++
		}
	}
	if files == 0 {
		return l, ErrEmpty
	}
	return l, nil
}

func detect(lines []string) string {
	var lsl, sized, headers, nonEmpty int
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		nonEmpty++
		switch {
		case psHeader.MatchString(l):
			return FormatPowerShell
		case strings.Contains(l, "<DIR>") || strings.Contains(l, "<JUNCTION>"):
			return FormatDir
		case lsLong.MatchString(l):
			lsl++
		case sizePath.MatchString(l):
			sized++
		case lsHeader.MatchString(l) && !strings.Contains(l, "\\"):
			headers++
		}
	}
	for _, l := range lines {
		if strings.HasPrefix(l, " ") && dirHeader.MatchString(l) {
			return FormatDir
		}
	}
	switch {
	case lsl > 0:
		return FormatLsLong
	case nonEmpty > 0 && sized*2 > nonEmpty:
		return FormatSizes
	case headers > 0:
		return FormatLs
	}
	return FormatPaths
}

// slash turns a path of any system into one with "/" between the folders,
// without the trailing one.
func slash(p string) string {
	p = strings.ReplaceAll(strings.TrimSpace(p), "\\", "/")
	for strings.HasSuffix(p, "/") && len(p) > 1 {
		p = strings.TrimSuffix(p, "/")
	}
	return p
}

// rel is p under root ("" for root itself); a folder outside root keeps its
// full path without the drive.
func rel(root, p string) string {
	if root == "" {
		if p == "." {
			return ""
		}
		return strings.TrimPrefix(p, "./")
	}
	if strings.EqualFold(p, root) {
		return ""
	}
	if len(p) > len(root) && strings.EqualFold(p[:len(root)], root) && (p[len(root)] == '/' || strings.HasSuffix(root, "/")) {
		return strings.TrimPrefix(p[len(root):], "/")
	}
	if len(p) > 2 && p[1] == ':' {
		p = p[2:]
	}
	return strings.TrimLeft(p, "/")
}

func join(dir, name string) string {
	if dir == "" {
		return name
	}
	return dir + "/" + name
}

func parsePowerShell(lines []string) Listing {
	l := Listing{Format: FormatPowerShell}
	dir := ""
	for _, line := range lines {
		t := strings.TrimSpace(line)
		switch {
		case t == "" || psHeader.MatchString(line) || strings.HasPrefix(t, "----"):
		case strings.HasPrefix(line, " ") && psDirLine.MatchString(line) && !strings.HasPrefix(t, "-") && !strings.HasPrefix(t, "d"):
			p := slash(psDirLine.FindStringSubmatch(line)[1])
			if l.Root == "" {
				l.Root = p
			}
			dir = rel(l.Root, p)
		case psFile.MatchString(t):
			m := psFile.FindStringSubmatch(t)
			n, _ := strconv.ParseInt(m[1], 10, 64)
			l.Entries = append(l.Entries, Entry{Path: join(dir, m[2]), Size: n, Sized: true})
		case psFolder.MatchString(t):
			l.Entries = append(l.Entries, Entry{Path: join(dir, psFolder.FindStringSubmatch(t)[1]), Dir: true})
		}
	}
	return l
}

func parseDir(lines []string) Listing {
	l := Listing{Format: FormatDir}
	dir := ""
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if m := dirHeader.FindStringSubmatch(line); m != nil && strings.HasPrefix(line, " ") && !dirSummary.MatchString(line) {
			p := slash(m[1])
			if l.Root == "" {
				l.Root = p
			}
			dir = rel(l.Root, p)
			continue
		}
		m := dirEntry.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		rest := strings.TrimSpace(m[3])
		if strings.HasPrefix(rest, "<") {
			i := strings.Index(rest, ">")
			if i < 0 {
				continue
			}
			name := strings.TrimSpace(rest[i+1:])
			// A junction prints its target in brackets.
			if j := strings.LastIndex(name, " ["); j > 0 && strings.HasSuffix(name, "]") {
				name = name[:j]
			}
			if name == "." || name == ".." || name == "" {
				continue
			}
			l.Entries = append(l.Entries, Entry{Path: join(dir, name), Dir: true})
			continue
		}
		s := dirSize.FindStringSubmatch(rest)
		if s == nil {
			continue
		}
		n, err := strconv.ParseInt(strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, s[1]), 10, 64)
		if err != nil {
			continue
		}
		l.Entries = append(l.Entries, Entry{Path: join(dir, s[2]), Size: n, Sized: true})
	}
	return l
}

// parseLs reads ls -R, with -l when long.
func parseLs(lines []string, long bool) Listing {
	l := Listing{Format: FormatLs}
	if long {
		l.Format = FormatLsLong
	}
	dir, header := "", false
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" || lsTotal.MatchString(t) {
			continue
		}
		// A header is "folder:" at the start of the listing or after an empty line.
		if m := lsHeader.FindStringSubmatch(line); m != nil && (i == 0 || strings.TrimSpace(lines[i-1]) == "") && !lsLong.MatchString(line) {
			p := slash(unquote(m[1]))
			if !header {
				header = true
				l.Root = p
			}
			dir = rel(l.Root, p)
			continue
		}
		if !long {
			if t == "." || t == ".." {
				continue
			}
			l.Entries = append(l.Entries, Entry{Path: join(dir, unquote(t))})
			continue
		}
		m := lsLong.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		kind := m[1]
		size, err, name, ok := lsFields(m[2])
		if !ok || name == "." || name == ".." {
			continue
		}
		if kind == "l" {
			if j := strings.Index(name, " -> "); j > 0 {
				name = name[:j]
			}
		}
		e := Entry{Path: join(dir, unquote(name)), Dir: kind == "d"}
		if kind == "-" {
			e.Size, e.Sized, e.Err = size, true, err
		}
		if kind == "-" || kind == "d" {
			l.Entries = append(l.Entries, e)
		}
	}
	return l
}

// lsFields reads "owner group size date name" of a line of ls -l (the group
// may be missing with -o or -g): the size, how far it can be from the real
// one, and the name.
func lsFields(s string) (int64, int64, string, bool) {
	f := fieldsWithPos(s)
	for _, si := range []int{2, 1} {
		if si >= len(f) {
			continue
		}
		size, err, ok := parseSize(f[si].s)
		if !ok {
			continue
		}
		di := si + 1
		if di+1 >= len(f) {
			continue
		}
		n := 3 // "nov 23 10:00", "23 nov 2025"
		switch {
		case isoDate.MatchString(f[di].s):
			n = 2 // --time-style=long-iso
			if di+2 < len(f) && timeZone.MatchString(f[di+2].s) {
				n = 3 // full-iso
			}
		case di+2 >= len(f) || !clockOrYear.MatchString(f[di+2].s):
			continue
		}
		ni := di + n
		if ni >= len(f) {
			continue
		}
		return size, err, s[f[ni].pos:], true
	}
	return 0, 0, "", false
}

type field struct {
	s   string
	pos int
}

func fieldsWithPos(s string) []field {
	var out []field
	start := -1
	for i, r := range s {
		if r == ' ' || r == '\t' {
			if start >= 0 {
				out = append(out, field{s[start:i], start})
				start = -1
			}
		} else if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		out = append(out, field{s[start:], start})
	}
	return out
}

// parseSize reads 4123456789 or, from ls -h, 4.2G (powers of 1024), with
// how far it can be from the real size.
func parseSize(s string) (int64, int64, bool) {
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n, 0, true
	}
	m := humanSize.FindStringSubmatch(strings.ToUpper(s))
	if m == nil {
		return 0, 0, false
	}
	num := strings.Replace(m[1], ",", ".", 1)
	v, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0, 0, false
	}
	mult := float64(1)
	if m[2] != "" {
		mult = math.Pow(1024, float64(strings.Index("KMGTP", m[2])+1))
	}
	step := 1.0
	if i := strings.Index(num, "."); i >= 0 {
		step = math.Pow(10, -float64(len(num)-i-1))
	}
	// ls -h rounds up.
	return int64(v * mult), int64(step * mult), true
}

func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '\'' && s[len(s)-1] == '\'' || s[0] == '"' && s[len(s)-1] == '"') {
		return s[1 : len(s)-1]
	}
	return s
}

func parseSizes(lines []string) Listing {
	l := Listing{Format: FormatSizes}
	var paths []string
	var sizes []int64
	for _, line := range lines {
		m := sizePath.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		n, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil {
			continue
		}
		paths = append(paths, slash(m[2]))
		sizes = append(sizes, n)
	}
	l.Root = commonDir(paths)
	for i, p := range paths {
		if r := rel(l.Root, p); r != "" {
			l.Entries = append(l.Entries, Entry{Path: r, Size: sizes[i], Sized: true})
		}
	}
	return l
}

func parsePaths(lines []string) Listing {
	l := Listing{Format: FormatPaths}
	var paths []string
	for _, line := range lines {
		if t := strings.TrimSpace(line); t != "" {
			paths = append(paths, slash(unquote(t)))
		}
	}
	l.Root = commonDir(paths)
	for _, p := range paths {
		if r := rel(l.Root, p); r != "" {
			l.Entries = append(l.Entries, Entry{Path: r})
		}
	}
	return l
}

// commonDir is the folder that holds every path ("" when they are relative
// with nothing in common).
func commonDir(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	dir := func(p string) string {
		if i := strings.LastIndex(p, "/"); i >= 0 {
			return p[:i]
		}
		return ""
	}
	under := func(p, c string) bool {
		return strings.EqualFold(p, c) || len(p) > len(c) && strings.EqualFold(p[:len(c)], c) && p[len(c)] == '/'
	}
	// find lists the folder itself too: start from the first path.
	c := paths[0]
	for _, p := range paths[1:] {
		for c != "" && !under(p, c) {
			c = dir(c)
		}
	}
	// A single file, or a folder with nothing under it, is not the root.
	deeper := false
	for _, p := range paths {
		if len(p) > len(c) && under(p, c) {
			deeper = true
			break
		}
	}
	if !deeper {
		c = dir(c)
	}
	if c == "." {
		return ""
	}
	return c
}

// clean drops the duplicates (find lists the folders too) and turns into
// folders the entries that hold others: a size printed for a folder (du)
// is not the size of a file.
func clean(es []Entry) []Entry {
	byPath := map[string]int{}
	var out []Entry
	for _, e := range es {
		e.Path = strings.Trim(path.Clean("/"+e.Path), "/")
		if e.Path == "" || e.Path == "." {
			continue
		}
		if i, ok := byPath[e.Path]; ok {
			out[i].Dir = out[i].Dir || e.Dir
			if !out[i].Sized && e.Sized {
				out[i].Size, out[i].Sized, out[i].Err = e.Size, true, e.Err
			}
			continue
		}
		byPath[e.Path] = len(out)
		out = append(out, e)
	}
	for _, e := range out {
		for p := path.Dir(e.Path); p != "." && p != "/"; p = path.Dir(p) {
			if i, ok := byPath[p]; ok {
				out[i].Dir = true
			}
		}
	}
	for i := range out {
		if out[i].Dir {
			out[i].Size, out[i].Sized, out[i].Err = 0, false, 0
		}
	}
	return out
}

// Found is a torrent found in the listing.
type Found struct {
	Hash string
	// The folder or file named like the torrent, relative to the root; for
	// a torrent found by its content, the folder of its files (or the file).
	Path  string
	Files int
	Size  int64
	// Every file has a size; Err is how far Size can be from the real one.
	Sized bool
	Err   int64
	// How it was found: "name", or the Via of its Content.
	Via string
	// Found by content: main files of the torrent not on the disk.
	Missing int
}

// Content is what a torrent holds, to find it on the disk when its files
// were renamed (by Radarr or Sonarr, with a hardlink, or by hand): a file
// of hundreds of MB has a size in bytes that is practically its own.
type Content struct {
	Hash string
	// Sizes of the files; those under MinContentSize are left out.
	Files []int64
	Via   string // "size", "radarr"…
}

// MinContentSize is the smallest file used to recognize a torrent by its
// content: nfo, subtitles and samples are often left out of a library.
const MinContentSize = 50 << 20

// Other is a file or folder of the listing that holds no torrent.
type Other struct {
	Path  string
	Name  string
	Files int
	Size  int64
	Sized bool
}

type node struct {
	name     string
	path     string
	parent   *node
	children map[string]*node
	file     bool
	files    int
	size     int64
	err      int64
	unsized  int
	taken    bool // named like a torrent
	claimed  bool // a file of a torrent found by its content
	contains *bool
}

func (n *node) child(name string) *node {
	if c, ok := n.children[name]; ok {
		return c
	}
	c := &node{name: name, path: join(n.path, name), parent: n, children: map[string]*node{}}
	n.children[name] = c
	return c
}

func (n *node) sorted() []*node {
	out := make([]*node, 0, len(n.children))
	for _, c := range n.children {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	return out
}

// under reports whether n is inside a folder named like a torrent.
func (n *node) under() bool {
	for p := n; p != nil; p = p.parent {
		if p.taken {
			return true
		}
	}
	return false
}

// Match finds the torrents in the listing: a folder or a file with the
// torrent's name (as qBittorrent names its content), at any depth. names
// maps every torrent name, trimmed and lowercased, to its hashes (see
// store.TorrentNames). The torrents in contents not found by name are then
// looked for by the sizes of their files. What holds no torrent is
// returned as Other, as high up as possible.
func Match(l Listing, names map[string][]string, contents []Content) ([]Found, []Other) {
	root := &node{children: map[string]*node{}}
	var files []*node
	for _, e := range l.Entries {
		n := root
		parts := strings.Split(e.Path, "/")
		for _, p := range parts {
			n = n.child(p)
			if !e.Dir {
				n.files++
				n.size += e.Size
				n.err += e.Err
				if !e.Sized {
					n.unsized++
				}
			}
		}
		if !e.Dir {
			n.file = true
			files = append(files, n)
		}
	}
	hashes := func(n *node) []string {
		return names[strings.ToLower(strings.TrimSpace(n.name))]
	}

	// By name.
	var found []Found
	byName := map[string]bool{}
	var walkNames func(n *node)
	walkNames = func(n *node) {
		if hs := hashes(n); len(hs) > 0 {
			n.taken = true
			for _, h := range hs {
				byName[h] = true
				found = append(found, Found{Hash: h, Path: n.path, Files: n.files, Size: n.size, Sized: n.files > 0 && n.unsized == 0, Err: n.err, Via: "name"})
			}
			return
		}
		for _, c := range n.sorted() {
			walkNames(c)
		}
	}
	walkNames(root)

	// By content: the files outside the folders found by name, by exact size.
	bySize := map[int64][]*node{}
	for _, f := range files {
		if f.unsized == 0 && f.err == 0 && f.size >= MinContentSize && !f.under() {
			bySize[f.size] = append(bySize[f.size], f)
		}
	}
	for _, c := range contents {
		if byName[c.Hash] {
			continue
		}
		var main []int64
		for _, s := range c.Files {
			if s >= MinContentSize {
				main = append(main, s)
			}
		}
		used := map[*node]bool{}
		var got []*node
		for _, s := range main {
			for _, f := range bySize[s] {
				if !used[f] {
					used[f] = true
					got = append(got, f)
					break
				}
			}
		}
		if len(got) == 0 {
			continue
		}
		byName[c.Hash] = true // found once
		f := Found{Hash: c.Hash, Files: len(got), Sized: true, Via: c.Via, Missing: len(main) - len(got)}
		for _, g := range got {
			g.claimed = true
			f.Size += g.size
		}
		var dir *node
		f.Path, dir = contentPath(got, used)
		if dir != nil {
			// Its subtitles and the like go with it.
			dir.claimed = true
		}
		found = append(found, f)
	}

	// What holds no torrent.
	var contains func(n *node) bool
	contains = func(n *node) bool {
		if n.contains != nil {
			return *n.contains
		}
		v := n.taken || n.claimed
		for _, c := range n.children {
			if contains(c) {
				v = true
			}
		}
		n.contains = &v
		return v
	}
	var others []Other
	var walk func(n *node)
	walk = func(n *node) {
		if n.taken || n.claimed {
			return
		}
		if !contains(n) {
			others = append(others, Other{Path: n.path, Name: n.name, Files: n.files, Size: n.size, Sized: n.unsized == 0})
			return
		}
		for _, c := range n.sorted() {
			walk(c)
		}
	}
	for _, c := range root.sorted() {
		walk(c)
	}
	return found, others
}

// contentPath is where a torrent found by its files is on the disk: the
// folder that holds them, unless that folder holds other big files too
// (a folder of many films), then the file itself when it is one.
func contentPath(got []*node, mine map[*node]bool) (string, *node) {
	dir := got[0].parent
	for _, g := range got[1:] {
		for !isAncestor(dir, g) {
			dir = dir.parent
		}
	}
	if dir == nil || dir.parent == nil {
		if len(got) == 1 {
			return got[0].path, nil
		}
		return "", nil
	}
	var others func(n *node) bool
	others = func(n *node) bool {
		if n.file {
			return !mine[n] && n.size >= MinContentSize
		}
		for _, c := range n.children {
			if others(c) {
				return true
			}
		}
		return false
	}
	if others(dir) {
		if len(got) == 1 {
			return got[0].path, nil
		}
		return dir.path, nil
	}
	return dir.path, dir
}

func isAncestor(a, n *node) bool {
	for p := n; p != nil; p = p.parent {
		if p == a {
			return true
		}
	}
	return false
}

// Check compares the size on the disk with the torrent's.
const (
	CheckComplete   = "complete"   // same size
	CheckIncomplete = "incomplete" // smaller: some data is missing
	CheckLarger     = "larger"     // more than the torrent: other files beside it?
	CheckUnknown    = "unknown"    // no size in the listing or for the torrent
)

// Compare tells how the copy found stands against a torrent of size bytes.
func (f Found) Compare(size int64) string {
	if f.Via != "name" && f.Via != "" {
		// Found by its files: the extras (nfo, samples) are often left out.
		if f.Missing > 0 {
			return CheckIncomplete
		}
		return CheckComplete
	}
	if !f.Sized || size <= 0 {
		return CheckUnknown
	}
	d := f.Size - size
	switch {
	case d >= -f.Err && d <= f.Err:
		return CheckComplete
	case d < 0:
		return CheckIncomplete
	}
	return CheckLarger
}
