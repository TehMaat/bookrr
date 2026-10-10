package unit3d

import "testing"

func TestSameRelease(t *testing.T) {
	cm := "Criminal Minds S19 2160p UHD DSNP WEB-DL DD+ 5.1 ITA ENG SUBS DV HDR10+ H.265-MaTiTa"
	astra := "Ad astra 2019 2160p UHD VU REMUX TrueHD 7.1 Atmos DD 5.1 DTS 5.1 DD 2.0 ENG ITA SUBS HDR10 H.265-MaTiTa"
	cases := []struct {
		file, tracker string
		want          bool
	}{
		{"Criminal.Minds.S19.2160p.DSNP.WEB-DL.DDP5.1.ITA.ENG.SUBS.DV.HDR10.H.265-MaTiTa", cm, true},
		{"AD.ASTRA.2019.REMUX.2160P.VU.HDR10.DTS.ITA.TRUEHD.ENG.SUBS.ITA.ENG", astra, true},
		{"criminal_minds_s19_2160p_dsnp_web-dl_eac3_ita_eng_dv_hdr10_x265-MaTiTa", cm, true},
		// Something different.
		{"Criminal.Minds.S19.1080p.DSNP.WEB-DL.DDP5.1.ITA.ENG.SUBS.DV.HDR10.H.265-MaTiTa", cm, false},
		{"Criminal.Minds.S18.2160p.DSNP.WEB-DL.DDP5.1.ITA.ENG.SUBS.DV.HDR10.H.265-MaTiTa", cm, false},
		{"Criminal.Minds.S19E01.2160p.DSNP.WEB-DL.DDP5.1.ITA.ENG.SUBS.DV.HDR10.H.265-MaTiTa", cm, false},
		{"Criminal.Minds.S19.2160p.DSNP.WEB-DL.DDP5.1.ITA.ENG.SUBS.HDR10.H.265-MaTiTa", cm, false},
		{"Criminal.Minds.S19.2160p.DSNP.WEB-DL.DDP5.1.ITA.ENG.SUBS.DV.HDR10.H.265-Altri", cm, false},
		{"Criminal.Minds.S19.2160p.DSNP.WEB-DL.DDP5.1.ITA.ENG.SUBS.DV.HDR10.H.264-MaTiTa", cm, false},
		{"Criminal.Minds.S19.2160p.NF.WEB-DL.DDP5.1.ITA.ENG.SUBS.DV.HDR10.H.265-MaTiTa", cm, false},
		{"Criminal.Minds.S19.2160p.DSNP.WEBRip.DDP5.1.ITA.ENG.SUBS.DV.HDR10.H.265-MaTiTa", cm, false},
		{"Criminal.Minds.Evolution.S19.2160p.DSNP.WEB-DL.DDP5.1.ITA.ENG.SUBS.DV.HDR10.H.265-MaTiTa", cm, false},
		{"Criminal.Minds.S19.2160p.DSNP.WEB-DL.DTS.ITA.ENG.SUBS.DV.HDR10.H.265-MaTiTa", cm, false},
		{"Criminal.Minds.S19.2160p.DSNP.WEB-DL.DDP5.1.ITA.ENG.FRE.DV.HDR10.H.265-MaTiTa", cm, false},
		{"AD.ASTRA.2018.REMUX.2160P.VU.HDR10.DTS.ITA.TRUEHD.ENG", astra, false},
		{"AD.ASTRA.2019.2160P.VU.HDR10.DTS.ITA.TRUEHD.ENG.x264", astra, false},
		{"Ad Astra", astra, false},
	}
	for _, tc := range cases {
		if got := sameRelease(tc.file, tc.tracker); got != tc.want {
			t.Errorf("%s\n  vs %s: got %v\n  file %+v\n  tracker %+v", tc.file, tc.tracker, got, parseRelease(tc.file), parseRelease(tc.tracker))
		}
	}
}

func TestParseRelease(t *testing.T) {
	r := parseRelease("2012.2009.1080p.BluRay.DTS-HD.MA.5.1.x264-GRP")
	if r.title != "2012" || r.year != "2009" || r.resolution != "1080p" || r.source != "bluray" || r.video != "h264" ||
		!r.audio["dts"] || len(r.audio) != 1 || r.group != "grp" {
		t.Fatalf("%+v", r)
	}
	if r := parseRelease("Mad.Max.Fury.Road.2015.2160p.WEB-DL"); r.title != "mad max fury road" || r.service != "" || r.group != "" {
		t.Fatalf("%+v", r)
	}
}
