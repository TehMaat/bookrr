package unit3d

import "testing"

func TestFullDisc(t *testing.T) {
	const gb = 1 << 30
	for _, c := range []struct {
		name   string
		size   int64
		bluRay bool
		want   bool
	}{
		// What only a disc says.
		{"Dune.Part.Two.2024.COMPLETE.UHD.BLURAY-SURCODE", 0, false, true},
		{"Movie.2020.MULTi.COMPLETE.BLURAY-XXX", 0, false, true},
		{"Film 2020 BD50 ITA ENG", 0, false, true},
		{"Film.2020.1080p.BDFull.AVC.ITA.ENG", 0, false, true},
		{"Film 2020 Full Disc 2160p", 0, false, true},
		{"Film.2020.UHD.BD66.HEVC.TrueHD.Atmos", 0, false, true},
		// A Blu-ray with the codec of the disc: by its size.
		{"Oppenheimer 2023 2160p UHD Blu-ray HEVC DTS-HD MA 5.1-GRP", 70 * gb, false, true},
		{"Oppenheimer 2023 2160p UHD Blu-ray HEVC DTS-HD MA 5.1-GRP", 20 * gb, false, false},
		{"The.Matrix.1999.1080p.BluRay.AVC.DTS-HD.MA.5.1-FGT", 40 * gb, false, true},
		{"The.Matrix.1999.1080p.BluRay.AVC.DTS-HD.MA.5.1-FGT", 0, false, false},
		{"Film 2020 1080p Blu-ray VC-1 DTS", 22 * gb, false, true},
		// Or filed under Movies/BluRay by its indexer.
		{"The.Matrix.1999.1080p.BluRay.AVC.DTS-HD.MA.5.1-FGT", 0, true, true},
		{"The Matrix 1999 1080p AVC DTS-HD MA", 0, true, true},
		{"The Matrix 1999 1080p", 0, true, false},
		// Never a disc: remuxes, encodes, rips, web sources.
		{"The.Matrix.1999.1080p.BluRay.REMUX.AVC.DTS-HD.MA.5.1-FGT", 30 * gb, true, false},
		{"Film.2020.2160p.UHD.BluRay.x265.10bit-GRP", 50 * gb, true, false},
		{"Film.2020.1080p.BluRay.H.264-GRP", 30 * gb, true, false},
		{"Film.2020.1080p.BDRip.AVC", 30 * gb, true, false},
		{"Film.2020.2160p.WEB-DL.HEVC", 30 * gb, false, false},
		{"Film.2020.1080p.BluRay.DTS", 30 * gb, false, false},
		{"Film 2020 BD50 Remux", 0, false, false},
	} {
		if got := FullDisc(c.name, c.size, c.bluRay); got != c.want {
			t.Errorf("FullDisc(%q, %d, %v) = %v", c.name, c.size, c.bluRay, got)
		}
	}
}

func TestDiscQuality(t *testing.T) {
	if q := DiscQuality("Dune.Part.Two.2024.COMPLETE.UHD.BLURAY.ITA.ENG-GRP"); q.Label != "2160p Full Disc ITA ENG" {
		t.Fatalf("got %+v", q)
	}
}
