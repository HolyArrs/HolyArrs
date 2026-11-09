package releasename

import (
	"reflect"
	"slices"
	"testing"
)

const (
	mb = int64(1) << 20
	gb = int64(1) << 30
)

// Case is one movie row of design/domain/release-names-testset.md.
// Zero values mean "none" or "unknown" ("-" in the test set).
type Case struct {
	ID         string
	Name       string
	Hints      Hints
	Title      string
	Year       int
	YearConf   Conf // checked only when set
	Resolution int
	ResConf    Conf // checked only when set
	Source     string
	SrcConf    Conf // checked only when set
	Codec      string
	HDR        []string
	Audio      string // format, then channels if the name gives them
	Langs      []string
	Edition    []string
	Group      string
	Fix        int
	Flags      []string
}

// MovieCases holds M01 to M22 and the movie rows of the tricky table
// (T01 to T26, T28, T31, T32). T27 is an episode and is left out.
var MovieCases = []Case{
	{ID: "M01", Name: "The.Lantern.Keepers.2021.1080p.WEB-DL.DDP5.1.H.265-QUILL", Title: "The Lantern Keepers", Year: 2021, Resolution: 1080, Source: "web_dl", Codec: "h265", Audio: "eac3 5.1", Group: "QUILL"},
	// The test set lists "aac 2.0"; the name gives no channels, so only the format is read.
	{ID: "M02", Name: "The.Lantern.Keepers.2021.1080p.WEBRip.x264.AAC-TINCAN", Title: "The Lantern Keepers", Year: 2021, Resolution: 1080, Source: "web_rip", Codec: "h264", Audio: "aac", Group: "TINCAN"},
	// The test set lists "dts 5.1"; the name gives no channels.
	{ID: "M03", Name: "Harbor.of.Glass.2018.Directors.Cut.1080p.BluRay.x264.DTS-MOSSFOX", Title: "Harbor of Glass", Year: 2018, Resolution: 1080, Source: "bluray", Codec: "h264", Audio: "dts", Edition: []string{"Directors Cut"}, Group: "MOSSFOX"},
	{ID: "M04", Name: "Harbor.of.Glass.2018.EXTENDED.REMASTERED.720p.BluRay.x264-OXBOW", Title: "Harbor of Glass", Year: 2018, Resolution: 720, Source: "bluray", Codec: "h264", Edition: []string{"Extended", "Remastered"}, Group: "OXBOW"},
	{ID: "M05", Name: "Quiet.Engines.2023.2160p.UHD.BluRay.REMUX.DV.HDR10.HEVC.TrueHD.7.1.Atmos-LARKSPUR", Title: "Quiet Engines", Year: 2023, Resolution: 2160, Source: "remux", Codec: "h265", HDR: []string{"dv", "hdr10"}, Audio: "truehd 7.1 atmos", Group: "LARKSPUR"},
	{ID: "M06", Name: "Quiet.Engines.2023.2160p.WEB-DL.DDP5.1.HDR10Plus.H.265-QUILL", Title: "Quiet Engines", Year: 2023, Resolution: 2160, Source: "web_dl", Codec: "h265", HDR: []string{"hdr10plus"}, Audio: "eac3 5.1", Group: "QUILL"},
	{ID: "M07", Name: "Quiet.Engines.2023.1080p.BluRay.REMUX.AVC.DTS-HD.MA.5.1-LARKSPUR", Title: "Quiet Engines", Year: 2023, Resolution: 1080, Source: "remux", Codec: "h264", Audio: "dts_hd_ma 5.1", Group: "LARKSPUR"},
	{ID: "M08", Name: "Moss.and.Iron.2016.3D.HSBS.1080p.BluRay.x264-PIXELORCHARD", Title: "Moss and Iron", Year: 2016, Resolution: 1080, Source: "bluray", Codec: "h264", Group: "PIXELORCHARD", Flags: []string{"three_d"}},
	{ID: "M09", Name: "Moss.&.Iron.2016.1080p.AMZN.WEB-DL.DDP5.1.H.264-GRAVELY", Title: "Moss and Iron", Year: 2016, Resolution: 1080, Source: "web_dl", Codec: "h264", Audio: "eac3 5.1", Group: "GRAVELY"},
	{ID: "M10", Name: "The.Paper.Comet.2009.720p.HDTV.x264-OXBOW", Title: "The Paper Comet", Year: 2009, Resolution: 720, Source: "tv", Codec: "h264", Group: "OXBOW"},
	{ID: "M11", Name: "The.Paper.Comet.2009.DVDRip.XviD-OLDREEL", Title: "The Paper Comet", Year: 2009, Resolution: 480, ResConf: Likely, Source: "dvd", Codec: "xvid", Group: "OLDREEL"},
	{ID: "M12", Name: "The.Paper.Comet.2009.COMPLETE.BLURAY-DISCWORKS", Title: "The Paper Comet", Year: 2009, Resolution: 1080, ResConf: Likely, Source: "disc_image", Group: "DISCWORKS", Flags: []string{"disc_image"}},
	{ID: "M13", Name: "Velvet.Static.2024.HDCAM.x264-GRAVELY", Title: "Velvet Static", Year: 2024, Source: "cam", Codec: "h264", Group: "GRAVELY"},
	{ID: "M14", Name: "Velvet.Static.2024.HDTS.720p.x264-TINCAN", Title: "Velvet Static", Year: 2024, Resolution: 720, Source: "telesync", Codec: "h264", Group: "TINCAN"},
	{ID: "M15", Name: "Velvet Static (2024) [1080p] [WEBRip] [5.1] [YTSLIKE]", Title: "Velvet Static", Year: 2024, Resolution: 1080, Source: "web_rip", Audio: "5.1", Group: "YTSLIKE"},
	{ID: "M16", Name: "Northbound.Tide.1999.REMASTERED.1080p.BluRay.10bit.x265.AAC-QUILL", Title: "Northbound Tide", Year: 1999, Resolution: 1080, Source: "bluray", Codec: "h265", Audio: "aac", Edition: []string{"Remastered"}, Group: "QUILL"},
	{ID: "M17", Name: "Northbound.Tide.1999.IMAX.2160p.WEB-DL.DV.HDR.H.265-QUILL", Title: "Northbound Tide", Year: 1999, Resolution: 2160, Source: "web_dl", Codec: "h265", HDR: []string{"dv", "hdr10"}, Edition: []string{"IMAX"}, Group: "QUILL"},
	{ID: "M18", Name: "Northbound.Tide.1999.1080p.WEB-DL.HLG.H.265-QUILL", Title: "Northbound Tide", Year: 1999, Resolution: 1080, Source: "web_dl", Codec: "h265", HDR: []string{"hlg"}, Group: "QUILL"},
	{ID: "M19", Name: "The.Last.Orchard.2020.UNRATED.1080p.WEB.h264-MOSSFOX", Title: "The Last Orchard", Year: 2020, Resolution: 1080, Source: "web_dl", SrcConf: Likely, Codec: "h264", Edition: []string{"Unrated"}, Group: "MOSSFOX"},
	{ID: "M20", Name: "The.Last.Orchard.2020.Theatrical.Cut.1080p.WEB-DL.AV1.Opus-PIXELORCHARD", Title: "The Last Orchard", Year: 2020, Resolution: 1080, Source: "web_dl", Codec: "av1", Audio: "opus", Edition: []string{"Theatrical"}, Group: "PIXELORCHARD"},
	{ID: "M21", Name: "Saltwater.Choir.2022.1080p.WEB-DL.DDP5.1.Atmos.H.264-QUILL.mkv", Title: "Saltwater Choir", Year: 2022, Resolution: 1080, Source: "web_dl", Codec: "h264", Audio: "eac3 5.1 atmos", Group: "QUILL"},
	{ID: "M22", Name: "Saltwater.Choir.2022.720p.WEB-DL.x264-TINCAN", Title: "Saltwater Choir", Year: 2022, Resolution: 720, Source: "web_dl", Codec: "h264", Group: "TINCAN"},

	{ID: "T01", Name: "1871.2023.1080p.WEB-DL.H.264-QUILL", Title: "1871", Year: 2023, Resolution: 1080, Source: "web_dl", Codec: "h264", Group: "QUILL"},
	{ID: "T02", Name: "1871.1080p.WEB-DL.H.264-QUILL", Title: "1871", YearConf: Guess, Resolution: 1080, Source: "web_dl", Codec: "h264", Group: "QUILL"},
	{ID: "T03", Name: "300.Lanterns.2015.720p.BluRay.x264-OXBOW", Title: "300 Lanterns", Year: 2015, Resolution: 720, Source: "bluray", Codec: "h264", Group: "OXBOW"},
	{ID: "T04", Name: "Night.Shift.2012.1080p.WEB-DL-QUILL", Title: "Night Shift", Year: 2012, Resolution: 1080, Source: "web_dl", Group: "QUILL"},
	{ID: "T05", Name: "Night.Shift.2020.1080p.WEB-DL-QUILL", Title: "Night Shift", Year: 2020, Resolution: 1080, Source: "web_dl", Group: "QUILL"},
	{ID: "T06", Name: "Night.Shift.2015.1080p.WEB-DL-QUILL", Title: "Night Shift", Year: 2015, Resolution: 1080, Source: "web_dl", Group: "QUILL"},
	{ID: "T07", Name: "Le.Jardin.des.Horloges.2017.FRENCH.1080p.BluRay.x264-LESQUAIS", Title: "Le Jardin des Horloges", Year: 2017, Resolution: 1080, Source: "bluray", Codec: "h264", Langs: []string{"fr"}, Group: "LESQUAIS"},
	// The test set lists "de, fr": DL means the original language is kept too,
	// which the parser reports as the "dual" flag; the judge adds the original.
	{ID: "T08", Name: "The.Clock.Garden.2017.GERMAN.DL.1080p.BluRay.x264-NORDFILM", Title: "The Clock Garden", Year: 2017, Resolution: 1080, Source: "bluray", Codec: "h264", Langs: []string{"de"}, Group: "NORDFILM", Flags: []string{"dual"}},
	{ID: "T09", Name: "The.Clock.Garden.2017.iTALiAN.DUBBED.1080p.WEB-DL-TINCAN", Title: "The Clock Garden", Year: 2017, Resolution: 1080, Source: "web_dl", Langs: []string{"it"}, Group: "TINCAN", Flags: []string{"dubbed"}},
	{ID: "T10", Name: "Kōri.no.Shiro.2020.1080p.WEB-DL.H.264-Kumomaru", Title: "Kori no Shiro", Year: 2020, Resolution: 1080, Source: "web_dl", Codec: "h264", Group: "Kumomaru"},
	{ID: "T11", Name: "氷の城.2020.1080p.WEB-DL-Kumomaru", Title: "氷の城", Year: 2020, Resolution: 1080, Source: "web_dl", Group: "Kumomaru"},
	{ID: "T12", Name: "Ледяной.Замок.2020.1080p.WEB-DL.RUS-SNEGOVIK", Title: "Ледяной Замок", Year: 2020, Resolution: 1080, Source: "web_dl", Langs: []string{"ru"}, Group: "SNEGOVIK"},
	{ID: "T13", Name: "The.Lantern.Keepers.2021.1080p.WEB-DL.H.264-QUILL.exe", Title: "The Lantern Keepers", Year: 2021, Resolution: 1080, Source: "web_dl", Codec: "h264", Group: "QUILL", Flags: []string{"executable"}},
	{ID: "T14", Name: "The.Lantern.Keepers.2021.1080p.WEB-DL.H.264-QUILL", Hints: Hints{FileList: []string{"setup.exe", "readme.txt"}}, Title: "The Lantern Keepers", Year: 2021, Resolution: 1080, Source: "web_dl", Codec: "h264", Group: "QUILL", Flags: []string{"executable"}},
	{ID: "T15", Name: "The.Lantern.Keepers.2021.1080p.WEB-DL.H.264-QUILL.mkv.lnk", Title: "The Lantern Keepers", Year: 2021, Resolution: 1080, Source: "web_dl", Codec: "h264", Group: "QUILL", Flags: []string{"double_extension", "executable"}},
	{ID: "T16", Name: "The.Lantern.Keepers.2021.1080p.BluRay.x264-QUILL.PASSWORDED", Title: "The Lantern Keepers", Year: 2021, Resolution: 1080, Source: "bluray", Codec: "h264", Group: "QUILL", Flags: []string{"password_hint"}},
	// size_mismatch (T17, T18) needs the runtime, so the judge reports it, not the parser.
	{ID: "T17", Name: "The.Lantern.Keepers.2021.2160p.WEB-DL-QUILL", Hints: Hints{Size: 650 * mb}, Title: "The Lantern Keepers", Year: 2021, Resolution: 2160, Source: "web_dl", Group: "QUILL"},
	{ID: "T18", Name: "The.Lantern.Keepers.2021.1080p.WEB-DL-QUILL", Hints: Hints{Size: 300 * mb}, Title: "The Lantern Keepers", Year: 2021, Resolution: 1080, Source: "web_dl", Group: "QUILL"},
	{ID: "T19", Name: "The.Lantern.Keepers.2021.SAMPLE.1080p.WEB-DL-QUILL", Hints: Hints{Size: 60 * mb}, Title: "The Lantern Keepers", Year: 2021, Resolution: 1080, Source: "web_dl", Group: "QUILL", Flags: []string{"sample"}},
	{ID: "T20", Name: "The.Lantern.Keepers.2021.Featurettes.1080p.WEB-DL-QUILL", Title: "The Lantern Keepers", Year: 2021, Resolution: 1080, Source: "web_dl", Group: "QUILL", Flags: []string{"extras"}},
	{ID: "T21", Name: "The.Lantern.Keepers.2021.720p.1080p.WEB-DL-TINCAN", Title: "The Lantern Keepers", Year: 2021, Resolution: 1080, ResConf: Guess, Source: "web_dl", Group: "TINCAN"},
	{ID: "T22", Name: "The.Lantern.Keepers.2021.1080p.WEB-DL.x264.UPSCALED-GRAVELY", Title: "The Lantern Keepers", Year: 2021, Resolution: 1080, Source: "web_dl", Codec: "h264", Group: "GRAVELY", Flags: []string{"upscaled"}},
	{ID: "T23", Name: "The.Lantern.Keepers.2021.PROPER.1080p.WEB-DL.H.265-QUILL", Title: "The Lantern Keepers", Year: 2021, Resolution: 1080, Source: "web_dl", Codec: "h265", Group: "QUILL", Fix: 1},
	{ID: "T24", Name: "The.Lantern.Keepers.2021.PROPER.1080p.WEB-DL.H.265-QUILL", Title: "The Lantern Keepers", Year: 2021, Resolution: 1080, Source: "web_dl", Codec: "h265", Group: "QUILL", Fix: 1},
	{ID: "T25", Name: "The.Lantern.Keepers.2021.720p.WEB-DL-QUILL", Title: "The Lantern Keepers", Year: 2021, Resolution: 720, Source: "web_dl", Group: "QUILL"},
	{ID: "T26", Name: "Lantern Keepers, The (2021) 1080p WEB-DL", Title: "The Lantern Keepers", Year: 2021, Resolution: 1080, Source: "web_dl"},
	{ID: "T28", Name: "www.somesite.example - The.Lantern.Keepers.2021.1080p.WEB-DL-QUILL", Title: "The Lantern Keepers", Year: 2021, Resolution: 1080, Source: "web_dl", Group: "QUILL", Flags: []string{"site_prefix"}},
	// Movie names the show patterns must not take.
	{ID: "T31", Name: "Movie.Title.Part.2.1080p.BluRay.x264-GRP", Title: "Movie Title Part 2", YearConf: Unknown, Resolution: 1080, Source: "bluray", Codec: "h264", Group: "GRP"},
	{ID: "T32", Name: "Some Movie - 2019 [1080p]", Title: "Some Movie", Year: 2019, Resolution: 1080},
}

func audioString(a Audio) string {
	s := a.Format
	if a.Channels != "" {
		if s != "" {
			s += " "
		}
		s += a.Channels
	}
	if a.Atmos {
		s += " atmos"
	}
	return s
}

func TestMovieNames(t *testing.T) {
	for _, c := range MovieCases {
		t.Run(c.ID, func(t *testing.T) {
			p := Parse(c.Name, c.Hints)
			if p.Kind.Value != "movie" {
				t.Errorf("kind %q", p.Kind.Value)
			}
			if CleanName(p.Title.Value) != CleanName(c.Title) {
				t.Errorf("title %q, want %q", p.Title.Value, c.Title)
			}
			if p.Year.Value != c.Year {
				t.Errorf("year %d, want %d", p.Year.Value, c.Year)
			}
			if c.YearConf != "" && p.Year.Conf != c.YearConf {
				t.Errorf("year confidence %s, want %s", p.Year.Conf, c.YearConf)
			}
			if p.Resolution.Value != c.Resolution {
				t.Errorf("resolution %d, want %d", p.Resolution.Value, c.Resolution)
			}
			if c.ResConf != "" && p.Resolution.Conf != c.ResConf {
				t.Errorf("resolution confidence %s, want %s", p.Resolution.Conf, c.ResConf)
			}
			if p.Source.Value != c.Source {
				t.Errorf("source %q, want %q", p.Source.Value, c.Source)
			}
			if c.SrcConf != "" && p.Source.Conf != c.SrcConf {
				t.Errorf("source confidence %s, want %s", p.Source.Conf, c.SrcConf)
			}
			if p.Codec.Value != c.Codec {
				t.Errorf("codec %q, want %q", p.Codec.Value, c.Codec)
			}
			if !slices.Equal(p.HDR.Value, c.HDR) {
				t.Errorf("hdr %v, want %v", p.HDR.Value, c.HDR)
			}
			if a := audioString(p.Audio); a != c.Audio {
				t.Errorf("audio %q, want %q", a, c.Audio)
			}
			if !slices.Equal(p.Languages, c.Langs) {
				t.Errorf("languages %v, want %v", p.Languages, c.Langs)
			}
			if !slices.Equal(p.Edition, c.Edition) {
				t.Errorf("edition %v, want %v", p.Edition, c.Edition)
			}
			if p.Group.Value != c.Group {
				t.Errorf("group %q, want %q", p.Group.Value, c.Group)
			}
			if p.Fix != c.Fix {
				t.Errorf("fix %d, want %d", p.Fix, c.Fix)
			}
			if !reflect.DeepEqual(nonNil(p.Flags), nonNil(c.Flags)) {
				t.Errorf("flags %v, want %v", p.Flags, c.Flags)
			}
		})
	}
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func TestCleanName(t *testing.T) {
	for in, want := range map[string]string{
		"The Lantern Keepers":  "lantern keepers",
		"Moss & Iron":          "moss and iron",
		"Kōri no Shiro":        "kori no shiro",
		"A Quiet Place":        "quiet place",
		"Harbor of Glass!":     "harbor of glass",
		"氷の城":                  "氷の城",
		"Ледяной Замок":        "ледяной замок",
		"  Velvet   Static  ":  "velvet static",
		"Ångström Élan Façade": "angstrom elan facade",
	} {
		if got := CleanName(in); got != want {
			t.Errorf("CleanName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEpisodeIsNotMovie(t *testing.T) {
	p := Parse("Unit 24 (2008) S01E01 1080p BluRay x264-OXBOW", Hints{})
	if p.Kind.Value != "episode" {
		t.Fatalf("kind %q, want episode", p.Kind.Value)
	}
}

func TestConflictNote(t *testing.T) {
	p := Parse("The.Lantern.Keepers.2021.720p.1080p.WEB-DL-TINCAN", Hints{})
	if len(p.Notes) == 0 {
		t.Fatal("two resolutions should leave a note")
	}
}

func BenchmarkParse1000(b *testing.B) {
	names := make([]string, 1000)
	for i := range names {
		names[i] = MovieCases[i%len(MovieCases)].Name
	}
	b.ReportAllocs()
	for b.Loop() {
		for _, n := range names {
			_ = Parse(n, Hints{})
		}
	}
}
