package releasename

import (
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// MusicCase is one music row of design/domain/release-names-testset.md.
type MusicCase struct {
	ID       string
	Name     string
	Kind     string
	Artist   string
	Album    string
	Year     int
	YearTo   int
	Format   string
	Lossless bool
	Bitrate  int
	VBR      string
	Depth    int
	Rate     float64
	Source   string
	Editions []string
	Discs    []int
	DiscN    int
	Various  bool
	Group    string
	Fix      int
	Flags    []string
	Conf     Conf
	Alt      *MusicAlt
}

// MusicCases holds U01 to U30.
var MusicCases = []MusicCase{
	{ID: "U01", Name: "Velvet Cartography - Low Tide Atlas (2019) [FLAC]", Kind: KindAlbum, Artist: "Velvet Cartography", Album: "Low Tide Atlas", Year: 2019, Format: "flac", Lossless: true, Conf: Sure},
	{ID: "U02", Name: "Velvet Cartography - Low Tide Atlas (2019) [FLAC 24-96]", Kind: KindAlbum, Artist: "Velvet Cartography", Album: "Low Tide Atlas", Year: 2019, Format: "flac", Lossless: true, Depth: 24, Rate: 96},
	{ID: "U03", Name: "Velvet_Cartography-Low_Tide_Atlas-CD-FLAC-2019-SPOOLHEAD", Kind: KindAlbum, Artist: "Velvet Cartography", Album: "Low Tide Atlas", Year: 2019, Format: "flac", Lossless: true, Source: "cd", Group: "SPOOLHEAD", Conf: Likely},
	{ID: "U04", Name: "Velvet Cartography - Low Tide Atlas (2019) [MP3 320]", Kind: KindAlbum, Artist: "Velvet Cartography", Album: "Low Tide Atlas", Year: 2019, Format: "mp3", Bitrate: 320},
	{ID: "U05", Name: "Velvet Cartography - Low Tide Atlas (2019) [MP3 V0]", Kind: KindAlbum, Artist: "Velvet Cartography", Album: "Low Tide Atlas", Year: 2019, Format: "mp3", VBR: "V0"},
	{ID: "U06", Name: "Velvet Cartography - Low Tide Atlas (2019) [128kbps]", Kind: KindAlbum, Artist: "Velvet Cartography", Album: "Low Tide Atlas", Year: 2019, Bitrate: 128},
	{ID: "U07", Name: "Velvet Cartography - Low Tide Atlas (2019) [Vinyl 24-192 FLAC]", Kind: KindAlbum, Artist: "Velvet Cartography", Album: "Low Tide Atlas", Year: 2019, Format: "flac", Lossless: true, Depth: 24, Rate: 192, Source: "vinyl"},
	{ID: "U08", Name: "Velvet Cartography - Discography (2011-2024) [FLAC]", Kind: KindDiscography, Artist: "Velvet Cartography", Year: 2011, YearTo: 2024, Format: "flac", Lossless: true},
	{ID: "U09", Name: "Velvet Cartography - Harbour Songs (Deluxe Edition) (2021) [2CD] [FLAC]", Kind: KindAlbum, Artist: "Velvet Cartography", Album: "Harbour Songs", Year: 2021, Format: "flac", Lossless: true, Editions: []string{"deluxe", "edition"}, DiscN: 2},
	{ID: "U10", Name: "Velvet Cartography - Harbour Songs (2021) CD2 [FLAC]", Kind: KindAlbum, Artist: "Velvet Cartography", Album: "Harbour Songs", Year: 2021, Format: "flac", Lossless: true, Discs: []int{2}},
	{ID: "U11", Name: "The 1975 Signals - Room Tone (2023) [FLAC]", Kind: KindAlbum, Artist: "The 1975 Signals", Album: "Room Tone", Year: 2023, Format: "flac", Lossless: true},
	{ID: "U12", Name: "Velvet Cartography - Low Tide Atlas (2019) [FLAC] (image+cue)", Kind: KindAlbum, Artist: "Velvet Cartography", Album: "Low Tide Atlas", Year: 2019, Format: "flac", Lossless: true, Flags: []string{"cue", "single_image"}},
	{ID: "U13", Name: "Velvet Cartography - Low Tide Atlas (2019) [AAC 256]", Kind: KindAlbum, Artist: "Velvet Cartography", Album: "Low Tide Atlas", Year: 2019, Format: "aac", Bitrate: 256},
	{ID: "U14", Name: "Velvet Cartography - Low Tide Atlas (2019) [Opus 160]", Kind: KindAlbum, Artist: "Velvet Cartography", Album: "Low Tide Atlas", Year: 2019, Format: "opus", Bitrate: 160},
	{ID: "U15", Name: "Velvet.Cartography-Low.Tide.Atlas-24BIT-WEB-FLAC-2019-SPOOLHEAD", Kind: KindAlbum, Artist: "Velvet Cartography", Album: "Low Tide Atlas", Year: 2019, Format: "flac", Lossless: true, Depth: 24, Source: "web", Group: "SPOOLHEAD"},
	{ID: "U16", Name: "VA - Summer Static Vol 3 (2020) [MP3 320]", Kind: KindAlbum, Artist: "VA", Album: "Summer Static Vol 3", Year: 2020, Format: "mp3", Bitrate: 320, Various: true},
	{ID: "U17", Name: "Velvet Cartography - Low Tide Atlas (2019) [ALAC] {Remastered}", Kind: KindAlbum, Artist: "Velvet Cartography", Album: "Low Tide Atlas", Year: 2019, Format: "alac", Lossless: true, Editions: []string{"remaster"}},
	{ID: "U18", Name: "Velvet Cartography - Low Tide Atlas (2019 Remaster) [WEB FLAC]", Kind: KindAlbum, Artist: "Velvet Cartography", Album: "Low Tide Atlas", Year: 2019, Format: "flac", Lossless: true, Source: "web", Editions: []string{"remaster"}},
	{ID: "U19", Name: "Prism Hollow - 1999 (2001) [FLAC]", Kind: KindAlbum, Artist: "Prism Hollow", Album: "1999", Year: 2001, Format: "flac", Lossless: true},
	{ID: "U20", Name: "Northern Lanterns - Live at Copper Hall (2018) [MP3 V2]", Kind: KindAlbum, Artist: "Northern Lanterns", Album: "Live at Copper Hall", Year: 2018, Format: "mp3", VBR: "V2"},
	{ID: "U21", Name: "Jay-Rook-Lowlands-WEB-2022-GRP", Kind: KindAlbum, Artist: "Jay", Album: "Rook - Lowlands", Year: 2022, Source: "web", Group: "GRP", Alt: &MusicAlt{Artist: "Jay-Rook", Album: "Lowlands"}},
	{ID: "U22", Name: "Velvet Cartography - Low Tide Atlas (2019) [FLAC] [Log] [Cue]", Kind: KindAlbum, Artist: "Velvet Cartography", Album: "Low Tide Atlas", Year: 2019, Format: "flac", Lossless: true, Flags: []string{"cue", "log"}},
	{ID: "U23", Name: "Velvet Cartography - Low Tide Atlas (2019) [FLAC].exe", Kind: KindAlbum, Artist: "Velvet Cartography", Album: "Low Tide Atlas", Year: 2019, Format: "flac", Lossless: true, Flags: []string{"executable"}},
	{ID: "U24", Name: "Velvet Cartography - Low Tide Atlas - 2019 - MP3 256kbps", Kind: KindAlbum, Artist: "Velvet Cartography", Album: "Low Tide Atlas", Year: 2019, Format: "mp3", Bitrate: 256},
	{ID: "U25", Name: "Velvet Cartography Low Tide Atlas 2019 FLAC", Kind: KindAlbum, Artist: "Velvet Cartography Low Tide Atlas", Year: 2019, Format: "flac", Lossless: true, Conf: Guess},
	{ID: "U26", Name: "Velvet Cartography - Low Tide Atlas (2019) [FLAC 16-44.1]", Kind: KindAlbum, Artist: "Velvet Cartography", Album: "Low Tide Atlas", Year: 2019, Format: "flac", Lossless: true, Depth: 16, Rate: 44.1},
	{ID: "U27", Name: "Velvet Cartography - Low Tide Atlas (2019) [Japan Edition] [FLAC]", Kind: KindAlbum, Artist: "Velvet Cartography", Album: "Low Tide Atlas", Year: 2019, Format: "flac", Lossless: true, Editions: []string{"japan", "edition"}},
	{ID: "U28", Name: "Velvet Cartography - Low Tide Atlas (2019) [96kHz 24bit FLAC]", Kind: KindAlbum, Artist: "Velvet Cartography", Album: "Low Tide Atlas", Year: 2019, Format: "flac", Lossless: true, Depth: 24, Rate: 96},
	{ID: "U29", Name: "Velvet Cartography - Low Tide Atlas (2019) [MP3 320] PROPER", Kind: KindAlbum, Artist: "Velvet Cartography", Album: "Low Tide Atlas", Year: 2019, Format: "mp3", Bitrate: 320, Fix: 1},
	{ID: "U30", Name: "Velvet Cartography - Low Tide Atlas (Part One) (2019) [FLAC]", Kind: KindAlbum, Artist: "Velvet Cartography", Album: "Low Tide Atlas (Part One)", Year: 2019, Format: "flac", Lossless: true},
}

func TestMusicNames(t *testing.T) {
	for _, c := range MusicCases {
		t.Run(c.ID, func(t *testing.T) {
			p := Parse(c.Name, Hints{Music: true})
			m := p.Music
			if m == nil {
				t.Fatal("not read as music")
			}
			check := func(field string, got, want any) {
				if !reflect.DeepEqual(got, want) {
					t.Errorf("%s: got %v, want %v", field, got, want)
				}
			}
			check("kind", p.Kind.Value, c.Kind)
			check("artist", m.Artist, c.Artist)
			check("title", p.Title.Value, c.Artist)
			check("album", m.Album, c.Album)
			check("year", p.Year.Value, c.Year)
			check("year_to", m.YearTo, c.YearTo)
			check("format", m.Format, c.Format)
			check("lossless", m.Lossless, c.Lossless)
			check("bitrate", m.Bitrate, c.Bitrate)
			check("vbr", m.VBR, c.VBR)
			check("depth", m.BitDepth, c.Depth)
			check("rate", m.SampleRate, c.Rate)
			check("source", m.Source, c.Source)
			check("editions", m.Editions, c.Editions)
			check("discs", m.Discs, c.Discs)
			check("disc_count", m.DiscCount, c.DiscN)
			check("various", m.Various, c.Various)
			check("group", p.Group.Value, c.Group)
			check("fix", p.Fix, c.Fix)
			for _, f := range c.Flags {
				if !p.Has(f) {
					t.Errorf("flag %s missing: %v", f, p.Flags)
				}
			}
			if c.Conf != "" {
				check("conf", m.Conf, c.Conf)
			}
			if c.Alt != nil && !slices.Contains(m.Alts, *c.Alt) {
				t.Errorf("alt %+v missing: %+v", *c.Alt, m.Alts)
			}
		})
	}
}

// Without the music hint a name is read as before (movies and shows keep
// their behaviour).
func TestMusicNeedsHint(t *testing.T) {
	p := Parse("Velvet Cartography - Low Tide Atlas (2019) [FLAC]", Hints{})
	if p.Music != nil || p.Kind.Value != "movie" {
		t.Fatalf("read as music without the hint: %+v", p)
	}
}

// Hostile names: 200 kB of brackets, hyphens, digits and tag words read in
// linear time (RE2 patterns, a capped name).
func TestMusicHostileNames(t *testing.T) {
	names := []string{
		strings.Repeat("(", 200_000),
		strings.Repeat("[FLAC 24-96] ", 16_000),
		strings.Repeat("a-", 100_000),
		"Artist - " + strings.Repeat("Album (", 30_000) + strings.Repeat(")", 30_000),
		strings.Repeat("CD1 2CD 24bit 96kHz 320kbps V0 ", 7_000),
		strings.Repeat("é-ü_", 50_000),
		strings.Repeat(" - ", 70_000),
		"Artist - Album" + strings.Repeat(".flac", 40_000),
		strings.Repeat(".", 200_000) + "exe",
		strings.Repeat("200b", 60_000) + strings.Repeat("Ff21", 60_000),
	}
	for i, n := range names {
		start := time.Now()
		p := Parse(n, Hints{Music: true})
		if d := time.Since(start); d > time.Second {
			t.Errorf("music: name %d (%d bytes) took %v", i, len(n), d)
		}
		// The movie and show readers on the same names finish too (linear
		// patterns; the bound is loose for slow and race-detector runs).
		start = time.Now()
		_ = Parse(n, Hints{})
		_ = Parse(n, Hints{Show: true})
		if d := time.Since(start); d > 30*time.Second {
			t.Errorf("movie and show: name %d (%d bytes) took %v", i, len(n), d)
		}
		if p.Music == nil {
			t.Errorf("name %d not read as music", i)
		}
	}
}

func BenchmarkMusicParse(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = Parse(MusicCases[i%len(MusicCases)].Name, Hints{Music: true})
	}
}

// A program extension is seen however long the name is (it is read before
// the name is cut), and a plain long name is not flagged.
func TestMusicLongNameExtensions(t *testing.T) {
	pad := strings.Repeat("a", 5000)
	for _, n := range []string{
		"Artist - Album " + pad + " x.mp3.exe",
		"Artist - Album " + strings.Repeat("b", maxMusicName-14) + ".exe",
	} {
		p := Parse(n, Hints{Music: true})
		if !p.Has("executable") {
			t.Errorf("%d bytes: executable missed", len(n))
		}
	}
	if p := Parse("Artist - Album "+pad+" x.mp3.exe", Hints{Music: true}); !p.Has("double_extension") {
		t.Error("double extension missed")
	}
	if p := Parse("Artist - Album (2019) [FLAC] "+pad, Hints{Music: true}); p.Has("executable") || p.Has("double_extension") {
		t.Errorf("plain long name flagged: %v", p.Flags)
	}
}

// Safety words count wherever they are: in the album name, at the start,
// after the format.
func TestMusicSafetyWordsAnywhere(t *testing.T) {
	for _, c := range [][2]string{{"PASSWORDED", "password_hint"}, {"Password", "password_hint"}, {"SAMPLE", "sample"},
		{"transcode", "transcode"}, {"Transcoded", "transcode"}} {
		word, flag := c[0], c[1]
		for _, n := range []string{
			"Artist - Album " + word + " Songs (2019) [FLAC]",
			word + " Artist - Album (2019) [FLAC]",
			"Artist - Album (2019) [FLAC] " + word,
		} {
			if p := Parse(n, Hints{Music: true}); !p.Has(flag) {
				t.Errorf("%s: flag %s missing %v", n, flag, p.Flags)
			}
		}
	}
}

// Safety words past the 2048-byte cut are still seen (the whole name is
// scanned before it is cut), in the name and in the file list.
func TestMusicSafetyWordsPastCut(t *testing.T) {
	pad := strings.Repeat("a", 3000)
	for word, flag := range map[string]string{" PASSWORDED": "password_hint", " sample": "sample", " transcoded": "transcode"} {
		if p := Parse("Artist - Album (2019) [FLAC] "+pad+word, Hints{Music: true}); !p.Has(flag) {
			t.Errorf("%s past the cut: flag %s missing %v", word, flag, p.Flags)
		}
	}
	for files, flag := range map[string]string{
		"01 Song.flac|" + pad + " PASSWORDED.txt": "password_hint",
		"01 Song.flac|Sample/" + pad + ".flac":    "sample",
		"01 Song.flac|sample.flac":                "sample",
	} {
		p := Parse("Artist - Album (2019) [FLAC]", Hints{Music: true, FileList: strings.Split(files, "|")})
		if !p.Has(flag) {
			t.Errorf("file list %.40q: flag %s missing %v", files, flag, p.Flags)
		}
	}
	// A track whose title merely holds the word is not a sample.
	if p := Parse("Artist - Album (2019) [FLAC]", Hints{Music: true, FileList: []string{"03 Free Sample Of Love.flac"}}); p.Has("sample") {
		t.Errorf("track title flagged as sample: %v", p.Flags)
	}
}

// Safety words hidden with full-width letters or zero-width characters are
// folded (NFKC, format characters removed) before they are looked for.
func TestMusicSafetyWordsFolded(t *testing.T) {
	for _, n := range []string{
		"Artist - Album (2019) [FLAC] ＰＡＳＳＷＯＲＤＥＤ",
		"Artist - Album (2019) [FLAC] pass​worded",
		"Artist - Album (2019) [FLAC] PASS­WORD",
	} {
		if p := Parse(n, Hints{Music: true}); !p.Has("password_hint") {
			t.Errorf("%q: password_hint missing %v", n, p.Flags)
		}
	}
}

// Trailing dots and zero-width characters do not hide a program extension,
// in the name or the file list; the wider program list is seen.
func TestMusicHiddenExtensions(t *testing.T) {
	for _, n := range []string{
		"Artist - Album (2019) [FLAC].exe.",
		"Artist - Album (2019) [FLAC].exe . .",
		"Artist - Album (2019) [FLAC].exe​",
		"Artist - Album (2019) [FLAC].ex​e",
		"Artist - Album (2019) [FLAC].flac.exe​.",
	} {
		if p := Parse(n, Hints{Music: true}); !p.Has("executable") {
			t.Errorf("%q: executable missed %v", n, p.Flags)
		}
	}
	for _, ext := range []string{"com", "pif", "hta", "jar", "wsf", "msi", "scr", "cmd", "bat", "vbs", "ps1", "js", "lnk", "vbe", "jse", "cpl", "msc", "reg"} {
		if p := Parse("Artist - Album (2019) [FLAC]."+ext, Hints{Music: true}); !p.Has("executable") {
			t.Errorf("%s: executable missed", ext)
		}
		if p := Parse("Artist - Album (2019) [FLAC]", Hints{Music: true, FileList: []string{"01 Song.flac", "setup." + ext + ".​"}}); !p.Has("executable") {
			t.Errorf("file list %s: executable missed", ext)
		}
	}
	if p := Parse("Artist - Album (2019) [FLAC]", Hints{Music: true, FileList: []string{"01 Song.flac", "folder.jpg", "Album.cue", "Album.log"}}); p.Has("executable") {
		t.Errorf("plain file list flagged: %v", p.Flags)
	}
}

// Disc ranges fill every disc they name; a trailing CD2 word is a tag, not
// part of the album name.
func TestMusicDiscs(t *testing.T) {
	for n, want := range map[string][]int{
		"Artist - Album (2019) [FLAC] CD1-2":   {1, 2},
		"Artist - Album (2019) CD 1-3 [FLAC]":  {1, 2, 3},
		"Artist - Album (2019) [CD2-3] [FLAC]": {2, 3},
		"Artist - Album CD2 [FLAC]":            {2},
		"Artist - Album CD2":                   {2},
	} {
		p := Parse(n, Hints{Music: true})
		if !reflect.DeepEqual(p.Music.Discs, want) || p.Music.Album != "Album" {
			t.Errorf("%s: album %q discs %v, want Album %v", n, p.Music.Album, p.Music.Discs, want)
		}
	}
}

// Scene names whose album is a number keep it as the album.
func TestMusicSceneNumericAlbum(t *testing.T) {
	for n, want := range map[string][3]string{
		"Taylor_Swift-1989-WEB-2014-GRP":       {"Taylor Swift", "1989", "2014"},
		"Adele-25-WEB-FLAC-2015-GRP":           {"Adele", "25", "2015"},
		"Velvet_Cartography-2019-WEB-FLAC-GRP": {"Velvet Cartography", "", "2019"},
	} {
		p := Parse(n, Hints{Music: true})
		if p.Music.Artist != want[0] || p.Music.Album != want[1] || strconv.Itoa(p.Year.Value) != want[2] {
			t.Errorf("%s: %q / %q / %d, want %v", n, p.Music.Artist, p.Music.Album, p.Year.Value, want)
		}
	}
}

// A common word in a title (Polish/Danish "Tak") is never read
// as a lossless format; an MP3 stays an MP3.
func TestMusicTitleWordIsNotAFormat(t *testing.T) {
	for _, n := range []string{"Kult - Tak (1993) [MP3 320]", "Kim Larsen - Tak for alt (2001) MP3 320"} {
		m := Parse(n, Hints{Music: true}).Music
		if m == nil || m.Lossless || m.Format != "mp3" || m.Album == "" {
			t.Errorf("%s: %+v", n, m)
		}
	}
}

// AIFF is a lossless format word.
func TestMusicAIFFIsLossless(t *testing.T) {
	for _, n := range []string{"Artist - Album (2020) [AIFF]", "Artist - Album (2020) AIF"} {
		if m := Parse(n, Hints{Music: true}).Music; m == nil || !m.Lossless || m.Format != "aiff" {
			t.Errorf("%s: %+v", n, m)
		}
	}
}
