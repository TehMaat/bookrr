package edition

import "testing"

func TestOf(t *testing.T) {
	cases := []struct{ name, title, want string }{
		{"Orbita.Silenziosa.2010.Extended.Cut.2160p.UHD.BluRay-XYZ", "Orbita Silenziosa", "Extended"},
		{"Orbita.Silenziosa.2010.DIRECTORS.CUT.1080p.BluRay", "Orbita Silenziosa", "Director's Cut"},
		{"Orbita Silenziosa (Director’s Cut) (2010)", "Orbita Silenziosa", "Director's Cut"},
		{"Orbita.Silenziosa.2010.Extended.Directors.Cut.1080p", "Orbita Silenziosa", "Director's Cut"},
		{"Orbita.Silenziosa.2010.Theatrical.Cut.2160p.WEB-DL", "Orbita Silenziosa", "Theatrical"},
		{"Orbita.Silenziosa.2010.25th.Anniversary.Edition.BluRay", "Orbita Silenziosa", "Anniversary Edition"},
		{"Orbita.Silenziosa.2010.IMAX.2160p", "Orbita Silenziosa", "IMAX"},
		{"Orbita.Silenziosa.2010.1080p.BluRay.ITA-ABC", "Orbita Silenziosa", ""},
		// The title's own words are not a version
		{"The.Final.Cut.2004.1080p.BluRay", "The Final Cut", ""},
		{"Uncut.Gems.2019.1080p", "Uncut Gems", ""},
		// Without a title to strip, the whole name is read
		{"Orbita.Silenziosa.2010.UNRATED.1080p", "", "Unrated"},
	}
	for _, c := range cases {
		if got := Of(c.name, c.title); got != c.want {
			t.Errorf("Of(%q, %q) = %q, want %q", c.name, c.title, got, c.want)
		}
	}
}

func TestSameAndNormalize(t *testing.T) {
	if !Same("Director's Cut", "directors cut") || Same("Extended", "Unrated") || !Same("", "") {
		t.Error("Same")
	}
	if got := Normalize("  Extended   Cut "); got != "Extended Cut" {
		t.Errorf("Normalize = %q", got)
	}
	long := ""
	for i := 0; i < 80; i++ {
		long += "x"
	}
	if got := Normalize(long); len(got) != MaxLen {
		t.Errorf("Normalize long = %d", len(got))
	}
}
