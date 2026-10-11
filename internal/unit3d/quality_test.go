package unit3d

import (
	"reflect"
	"testing"
)

func TestQualityCompare(t *testing.T) {
	mine := QualityOf("Serie.S01.2160p.WEB-DL.DDP5.1.ITA.ENG.DV.HDR.H.265-ME", "WEB-DL", "2160p", "")
	if mine.Label != "2160p WEB-DL DV HDR ITA ENG" {
		t.Fatalf("label %q", mine.Label)
	}
	mediaInfo := "General\nComplete name : x.mkv\n\nVideo\nFormat : HEVC\n\nAudio #1\nFormat : E-AC-3\nLanguage : Italian\n\n" +
		"Audio #2\nLanguage : English (US)\n\nText #1\nLanguage : French\n"
	cases := []struct {
		name, typ, res, mediaInfo string
		same                      bool
	}{
		// The examples of the issue.
		{"Serie S02 2160p WEB-DL DV HDR ITA", "WEB-DL", "2160p", "", false},
		{"Serie S02 2160p WEB-DL HDR ITA ENG", "WEB-DL", "2160p", "", false},
		// Written differently, other codecs or group: same quality.
		{"Serie S02 2160p AMZN WEB-DL DD+ 5.1 ITA ENG DoVi HDR10+ H.265-OTHER", "WEB-DL", "2160p", "", true},
		{"Serie.S03.2160p.WEB-DL.ENG.ITA.DV.HDR10.x265", "WEB-DL", "2160p", "", true},
		// Type and resolution come from the tracker.
		{"Serie S02 2160p WEB-DL DV HDR ITA ENG", "WEBRip", "2160p", "", false},
		{"Serie S02 2160p WEB-DL DV HDR ITA ENG", "WEB-DL", "1080p", "", false},
		{"Serie S02 2160p WEB-DL DV HDR ITA ENG", "Remux", "2160p", "", false},
		{"Serie S02 2160p WEB-DL DV HDR HLG ITA ENG", "WEB-DL", "2160p", "", false},
		// No languages in the name: those of the audio tracks.
		{"Serie.S02.2160p.WEB-DL.DV.HDR", "WEB-DL", "2160p", mediaInfo, true},
		{"Serie.S02.2160p.WEB-DL.MULTI.DV.HDR", "Web-DL", "2160p", mediaInfo, true},
		{"Serie.S02.2160p.WEB-DL.DV.HDR", "WEB-DL", "2160p", "", false},
		// The name wins over the media info.
		{"Serie.S02.2160p.WEB-DL.ITA.DV.HDR", "WEB-DL", "2160p", mediaInfo, false},
	}
	for _, tc := range cases {
		q := QualityOf(tc.name, tc.typ, tc.res, tc.mediaInfo)
		if got := q.Key == mine.Key; got != tc.same {
			t.Errorf("%s (%s %s): same %v, want %v\n  %q vs %q", tc.name, tc.res, tc.typ, got, tc.same, q.Key, mine.Key)
		}
	}
	// Without type and resolution from the tracker, the name says them.
	if q := QualityOf("Serie.S01.1080p.BluRay.REMUX.AVC.ITA.ENG", "", "", ""); q.Label != "1080p Remux ITA ENG" {
		t.Errorf("label %q", q.Label)
	}
	if g := mine.Group(); g != "2160p WEB-DL" {
		t.Errorf("group %q", g)
	}
}

func TestParsePack(t *testing.T) {
	cases := []struct {
		name     string
		ok       bool
		seasons  []int
		complete bool
	}{
		{"Serie.S01.2160p.WEB-DL.ITA.ENG-GRP", true, []int{1}, false},
		{"Serie (2019) S02 1080p WEB-DL ITA", true, []int{2}, false},
		{"Serie S01-S05 1080p BluRay Remux", true, []int{1, 2, 3, 4, 5}, false},
		{"Serie.S01-05.1080p.WEB-DL", true, []int{1, 2, 3, 4, 5}, false},
		{"Serie S1 - S3 720p HDTV", true, []int{1, 2, 3}, false},
		{"Serie.S01.S02.S03.1080p.WEB-DL", true, []int{1, 2, 3}, false},
		{"Serie Season 1-3 1080p", true, []int{1, 2, 3}, false},
		{"Serie Seasons 2 to 4 1080p", true, []int{2, 3, 4}, false},
		{"Serie Stagioni 1 e 2 1080p", true, []int{1, 2}, false},
		{"Serie Stagione 3 Completa 1080p WEB-DL", true, []int{3}, false},
		{"Serie.S04.COMPLETE.2160p.WEB-DL", true, []int{4}, false},
		{"Serie Complete Series 1080p BluRay", true, nil, true},
		{"Serie.COMPLETE.1080p.WEB-DL.ITA", true, nil, true},
		// Episodes, a few episodes, nothing about seasons: not packs.
		{"Serie.S01E01.2160p.WEB-DL", false, nil, false},
		{"Serie S01E01-E10 1080p", false, nil, false},
		{"Serie S01 E05 1080p", false, nil, false},
		{"Serie.S01E01-S05E10.1080p", false, nil, false},
		{"Film 2019 2160p WEB-DL", false, nil, false},
		{"Film.2019.1080p.x264.DD5.1", false, nil, false},
	}
	for _, tc := range cases {
		p, ok := ParsePack(tc.name)
		if ok != tc.ok || !reflect.DeepEqual(p.Seasons, tc.seasons) || p.Complete != tc.complete {
			t.Errorf("%s: got %v %+v, want %v %v complete %v", tc.name, ok, p, tc.ok, tc.seasons, tc.complete)
		}
	}
	// The seasons end the title and are compared as a whole.
	r := parseRelease("Serie.Test.S01-S05.1080p.WEB-DL")
	if r.title != "serie test" || r.season != "s1-s5" || r.resolution != "1080p" {
		t.Errorf("%+v", r)
	}
	if sameRelease("Serie.Test.S01-S05.1080p.WEB-DL-GRP", "Serie Test S01 1080p WEB-DL-GRP") {
		t.Error("a pack of five seasons is not the first season")
	}
}
