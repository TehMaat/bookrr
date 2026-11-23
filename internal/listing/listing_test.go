package listing

import (
	"fmt"
	"reflect"
	"testing"
)

var names = map[string][]string{
	"movie.2020.1080p.bluray-grp":    {"aaa"},
	"single.2019.2160p.web-dl.mkv":   {"bbb"},
	"show.s01.1080p.bluray.x264-grp": {"ccc"},
}

// files returns path → size of the files of a listing.
func files(l Listing) map[string]int64 {
	out := map[string]int64{}
	for _, e := range l.Entries {
		if !e.Dir {
			out[e.Path] = e.Size
		}
	}
	return out
}

func TestParseFormats(t *testing.T) {
	want := map[string]int64{
		"Films/Movie.2020.1080p.BluRay-GRP/Movie.2020.1080p.BluRay-GRP.mkv": 4123456789,
		"Films/Movie.2020.1080p.BluRay-GRP/Subs/ita.srt":                    1200,
		"Films/Single.2019.2160p.WEB-DL.mkv":                                9000000000,
		"Other/notes 2021.txt":                                              15,
	}
	cases := []struct {
		name, format, root, text string
	}{
		{"find printf", FormatSizes, "", `4123456789 Films/Movie.2020.1080p.BluRay-GRP/Movie.2020.1080p.BluRay-GRP.mkv
1200 Films/Movie.2020.1080p.BluRay-GRP/Subs/ita.srt
9000000000 Films/Single.2019.2160p.WEB-DL.mkv
15 Other/notes 2021.txt
`},
		{"find printf absolute with folders", FormatSizes, "/mnt/archive", `4096 /mnt/archive
4096 /mnt/archive/Films
4096 /mnt/archive/Films/Movie.2020.1080p.BluRay-GRP
4123456789 /mnt/archive/Films/Movie.2020.1080p.BluRay-GRP/Movie.2020.1080p.BluRay-GRP.mkv
4096 /mnt/archive/Films/Movie.2020.1080p.BluRay-GRP/Subs
1200 /mnt/archive/Films/Movie.2020.1080p.BluRay-GRP/Subs/ita.srt
9000000000 /mnt/archive/Films/Single.2019.2160p.WEB-DL.mkv
4096 /mnt/archive/Other
15 /mnt/archive/Other/notes 2021.txt
`},
		{"ls -lR", FormatLsLong, "/mnt/archive", `/mnt/archive:
total 8
drwxr-xr-x 3 user user 4096 Nov 23 10:00 Films
drwxr-xr-x 2 1000 1000 4096 Nov 23 10:00 Other

/mnt/archive/Films:
total 8789072
drwxr-xr-x 3 user user       4096 Nov 23 10:00 Movie.2020.1080p.BluRay-GRP
-rw-r--r-- 1 user user 9000000000 Jan  2  2025 Single.2019.2160p.WEB-DL.mkv

/mnt/archive/Films/Movie.2020.1080p.BluRay-GRP:
total 4026816
-rw-r--r--  1 user user 4123456789 2026-11-23 10:00 Movie.2020.1080p.BluRay-GRP.mkv
drwxr-xr-x  2 user user       4096 2026-11-23 10:00 Subs

/mnt/archive/Films/Movie.2020.1080p.BluRay-GRP/Subs:
total 4
-rw-r--r--@ 1 user  staff  1200 23 nov 10:00 ita.srt

/mnt/archive/Other:
total 4
-rw-r--r-- 1 1000 1000 15 Nov 23 10:00 notes 2021.txt
`},
		{"dir /s", FormatDir, "D:", " Il volume nell'unità D è ARCHIVIO\r\n Numero di serie del volume: 1234-ABCD\r\n\r\n" +
			" Directory di D:\\\r\n\r\n23/11/2026  10:00    <DIR>          Films\r\n23/11/2026  10:00    <DIR>          Other\r\n" +
			"               0 File              0 byte\r\n\r\n" +
			" Directory di D:\\Films\r\n\r\n23/11/2026  10:00    <DIR>          .\r\n23/11/2026  10:00    <DIR>          ..\r\n" +
			"23/11/2026  10:00    <DIR>          Movie.2020.1080p.BluRay-GRP\r\n02/01/2025  09:00     9.000.000.000 Single.2019.2160p.WEB-DL.mkv\r\n" +
			"               1 File  9.000.000.000 byte\r\n\r\n" +
			" Directory di D:\\Films\\Movie.2020.1080p.BluRay-GRP\r\n\r\n23/11/2026  10:00     4.123.456.789 Movie.2020.1080p.BluRay-GRP.mkv\r\n" +
			"23/11/2026  10:00    <DIR>          Subs\r\n\r\n" +
			" Directory di D:\\Films\\Movie.2020.1080p.BluRay-GRP\\Subs\r\n\r\n11/23/2026  10:00 AM             1,200 ita.srt\r\n\r\n" +
			" Directory di D:\\Other\r\n\r\n23/11/2026  10:00                15 notes 2021.txt\r\n" +
			"     Totale file elencati:\r\n               3 File  13.123.458.004 byte\r\n"},
		{"Get-ChildItem -Recurse", FormatPowerShell, "D:", `
    Directory: D:\

Mode                 LastWriteTime         Length Name
----                 -------------         ------ ----
d-----        23/11/2026     10:00                Films
d-----        23/11/2026     10:00                Other

    Directory: D:\Films

Mode                 LastWriteTime         Length Name
----                 -------------         ------ ----
d-----        23/11/2026     10:00                Movie.2020.1080p.BluRay-GRP
-a----        02/01/2025     09:00     9000000000 Single.2019.2160p.WEB-DL.mkv

    Directory: D:\Films\Movie.2020.1080p.BluRay-GRP

Mode                 LastWriteTime         Length Name
----                 -------------         ------ ----
la---          11/23/2026 10:00 AM     4123456789 Movie.2020.1080p.BluRay-GRP.mkv
d----          11/23/2026 10:00 AM                Subs

    Directory: D:\Films\Movie.2020.1080p.BluRay-GRP\Subs

Mode                 LastWriteTime         Length Name
----                 -------------         ------ ----
-a----        23/11/2026     10:00           1200 ita.srt

    Directory: D:\Other

Mode                 LastWriteTime         Length Name
----                 -------------         ------ ----
-a----        23/11/2026     10:00             15 notes 2021.txt
`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l, err := Parse(c.text)
			if err != nil {
				t.Fatal(err)
			}
			if l.Format != c.format || l.Root != c.root {
				t.Errorf("format %q root %q, want %q %q", l.Format, l.Root, c.format, c.root)
			}
			if got := files(l); !reflect.DeepEqual(got, want) {
				t.Errorf("files:\n got %v\nwant %v", got, want)
			}
		})
	}
}

func TestParseWithoutSizes(t *testing.T) {
	for name, text := range map[string]string{
		"dir /s /b": "D:\\Films\r\nD:\\Films\\Movie.2020.1080p.BluRay-GRP\r\nD:\\Films\\Movie.2020.1080p.BluRay-GRP\\a.mkv\r\nD:\\Films\\Single.2019.2160p.WEB-DL.mkv\r\n",
		"find":      "/mnt/a/Films\n/mnt/a/Films/Movie.2020.1080p.BluRay-GRP\n/mnt/a/Films/Movie.2020.1080p.BluRay-GRP/a.mkv\n/mnt/a/Films/Single.2019.2160p.WEB-DL.mkv\n",
		"ls -R":     ".:\nFilms\n\n./Films:\nMovie.2020.1080p.BluRay-GRP\nSingle.2019.2160p.WEB-DL.mkv\n\n./Films/Movie.2020.1080p.BluRay-GRP:\na.mkv\n",
	} {
		t.Run(name, func(t *testing.T) {
			l, err := Parse(text)
			if err != nil {
				t.Fatal(err)
			}
			found, _ := Match(l, names, nil)
			if len(found) != 2 {
				t.Fatalf("found %+v (entries %+v)", found, l.Entries)
			}
			for _, f := range found {
				if f.Sized || f.Compare(100) != CheckUnknown {
					t.Errorf("%+v: no sizes in the listing", f)
				}
			}
		})
	}
}

func TestMatch(t *testing.T) {
	l, err := Parse(`4123456789 Films/Movie.2020.1080p.BluRay-GRP/Movie.2020.1080p.BluRay-GRP.mkv
1200 Films/Movie.2020.1080p.BluRay-GRP/Subs/ita.srt
8000000000 Films/Single.2019.2160p.WEB-DL.mkv
500 Series/Show/Show.S01.1080p.BluRay.x264-GRP/e01.mkv
600 Series/Show/Show.S01.1080p.BluRay.x264-GRP/e02.mkv
700 Series/Show/Extras/x.mkv
15 Other/notes.txt
`)
	if err != nil {
		t.Fatal(err)
	}
	found, others := Match(l, names, nil)
	byHash := map[string]Found{}
	for _, f := range found {
		byHash[f.Hash] = f
	}
	if f := byHash["aaa"]; f.Path != "Films/Movie.2020.1080p.BluRay-GRP" || f.Files != 2 || f.Size != 4123457989 {
		t.Errorf("aaa: %+v", f)
	}
	if f := byHash["bbb"]; f.Path != "Films/Single.2019.2160p.WEB-DL.mkv" || f.Compare(9000000000) != CheckIncomplete || f.Compare(8000000000) != CheckComplete || f.Compare(7000000000) != CheckLarger {
		t.Errorf("bbb: %+v", f)
	}
	if f := byHash["ccc"]; f.Path != "Series/Show/Show.S01.1080p.BluRay.x264-GRP" || f.Size != 1100 {
		t.Errorf("ccc: %+v", f)
	}
	var paths []string
	for _, o := range others {
		paths = append(paths, o.Path)
	}
	if !reflect.DeepEqual(paths, []string{"Other", "Series/Show/Extras"}) {
		t.Errorf("others %v", paths)
	}
}

func TestHumanSizes(t *testing.T) {
	l, err := Parse(`/mnt/a/Films:
total 4
-rw-r--r-- 1 user user 3,9G Nov 23 10:00 Single.2019.2160p.WEB-DL.mkv
`)
	if err != nil {
		t.Fatal(err)
	}
	found, _ := Match(l, names, nil)
	if len(found) != 1 {
		t.Fatalf("found %+v", found)
	}
	f := found[0]
	if f.Compare(4123456789) != CheckComplete || f.Compare(3000000000) != CheckLarger || f.Compare(5000000000) != CheckIncomplete {
		t.Errorf("%+v", f)
	}
}

func TestEmpty(t *testing.T) {
	if _, err := Parse("\n  \n"); err != ErrEmpty {
		t.Fatalf("err %v", err)
	}
}

// Files imported by Radarr or Sonarr with a hardlink have Radarr's names:
// they are found by their sizes.
func TestMatchByContent(t *testing.T) {
	const mb = 1 << 20
	l, err := Parse(fmt.Sprintf(`%d Films/Movie (2020)/Movie (2020) Bluray-1080p.mkv
%d Films/Movie (2020)/Movie (2020) Bluray-1080p.it.srt
%d Films/Other (2021)/Other (2021) WEBDL-2160p.mkv
%d Films/Other (2021)/Third (2019).mkv
%d Series/Show/Season 01/Show - S01E01.mkv
%d Series/Show/Season 01/Show - S01E02.mkv
%d Series/Gap/Season 01/Gap - S01E01.mkv
%d Movie.2020.1080p.BluRay-GRP/Movie.2020.1080p.BluRay-GRP.mkv
%d Unknown/thing.mkv
`, 4000*mb+1, 50000, 9000*mb+7, 3000*mb+3, 700*mb+11, 710*mb+13, 650*mb+17, 4000*mb+1, 123*mb))
	if err != nil {
		t.Fatal(err)
	}
	contents := []Content{
		// Found by name first: not again by content.
		{Hash: "aaa", Files: []int64{4000*mb + 1, 2000}, Via: "size"},
		// The movie Radarr renamed, with an nfo and a sample left out.
		{Hash: "rad", Files: []int64{9000*mb + 7, 1000, 40 * mb}, Via: "size"},
		// Same size as the copy named like aaa: claimed only outside it.
		{Hash: "dup", Files: []int64{4000*mb + 1}, Via: "size"},
		{Hash: "sea", Files: []int64{700*mb + 11, 710*mb + 13, 3000}, Via: "size"},
		// Two episodes, one on the disk.
		{Hash: "gap", Files: []int64{650*mb + 17, 660 * mb}, Via: "size"},
		{Hash: "rdr", Files: []int64{3000*mb + 3}, Via: "radarr"},
		{Hash: "none", Files: []int64{1 * mb, 999 * mb}, Via: "size"},
	}
	found, others := Match(l, names, contents)
	got := map[string]string{}
	for _, f := range found {
		got[f.Hash] = fmt.Sprintf("%s %s %d %s", f.Via, f.Path, f.Files, f.Compare(1))
	}
	want := map[string]string{
		"aaa": "name Movie.2020.1080p.BluRay-GRP 1 larger",
		"dup": "size Films/Movie (2020) 1 complete",
		"rad": "size Films/Other (2021)/Other (2021) WEBDL-2160p.mkv 1 complete",
		"sea": "size Series/Show/Season 01 2 complete",
		"gap": "size Series/Gap/Season 01 1 incomplete",
		"rdr": "radarr Films/Other (2021)/Third (2019).mkv 1 complete",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("found\n got %v\nwant %v", got, want)
	}
	var paths []string
	for _, o := range others {
		paths = append(paths, o.Path)
	}
	if !reflect.DeepEqual(paths, []string{"Unknown"}) {
		t.Errorf("others %v", paths)
	}
}
