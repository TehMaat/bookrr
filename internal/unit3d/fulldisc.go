package unit3d

import "strings"

// Names of full discs (the Blu-ray as it is: BDMV folder or ISO): they
// rarely say "Full Disc", and "BluRay" alone is also in the names of the
// encodes and of the remuxes.
var (
	// What only a disc says: "BD50", "BDFull", "Full Disc", "BDMV",
	// "COMPLETE.UHD.BLURAY".
	discMarkRe = rx(`bd[ .-]?(?:25|50|66|100)|bd[ .-]?full|full[ .-]?disc|bdmv|complete[ .-](?:uhd[ .-])?blu-?ray`)
	// What a disc never says: an encoder, a remux, a rip, a web source.
	notDiscRe    = rx(`remux|[hx][ .]?26[45]|x26[45]|bd-?rip|br-?rip|hd-?rip|dvd-?rip|web-?dl|web-?rip|hdtv|xvid|divx|av1|upscal(?:e|ed)?|mkv|mp4`)
	discBluRayRe = rx(`blu-?ray|bd`)
	// The video of a Blu-ray as its names write it, without an encoder.
	discCodecRe = rx(`avc|hevc|vc-?1|mpeg-?2`)
)

// Sizes past which a "BluRay AVC/HEVC" with no encoder in the name is a
// disc: a BD25 holds a 1080p film in about 20 GB, a UHD disc is a BD50
// at least.
const (
	discMinSize    = 15 << 30
	uhdDiscMinSize = 35 << 30
)

// FullDisc reports whether a release is a full disc (Blu-ray or UHD
// Blu-ray), from its name, its size (0 if unknown) and whether its
// indexer files it under Movies/BluRay (2050 for Newznab and Torznab).
func FullDisc(name string, size int64, bluRayCategory bool) bool {
	s := strings.ToLower(strings.TrimSpace(name))
	switch {
	case notDiscRe.MatchString(s):
		return false
	case discMarkRe.MatchString(s):
		return true
	case !discBluRayRe.MatchString(s) && !(bluRayCategory && discCodecRe.MatchString(s)):
		return false
	case bluRayCategory:
		return true
	case !discCodecRe.MatchString(s):
		return false
	}
	min := int64(discMinSize)
	if parseRelease(s).resolution == "2160p" {
		min = uhdDiscMinSize
	}
	return size >= min
}

// DiscQuality is the quality of a full disc found on an indexer, typed
// "Full Disc" whatever its name says ("2160p Full Disc DV HDR ITA ENG").
func DiscQuality(name string) Quality {
	return QualityOf(name, "Full Disc", "", "")
}
