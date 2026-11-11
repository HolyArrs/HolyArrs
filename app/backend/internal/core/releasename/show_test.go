package releasename

import (
	"reflect"
	"slices"
	"testing"
)

// ShowCase is one show or anime row of design/domain/release-names-testset.md.
type ShowCase struct {
	ID         string
	Name       string
	Hints      Hints
	Kind       string
	Title      string
	Year       int
	Show       ShowNumbers // Conf is checked only when set
	Resolution int
	Source     string
	SrcConf    Conf // checked only when set
	Codec      string
	Audio      string
	Langs      []string
	Group      string
	Fix        int
	Flags      []string
}

func sn(seasons []int, eps ...int) ShowNumbers { return ShowNumbers{Seasons: seasons, Episodes: eps} }
func s1(s int) []int                           { return []int{s} }

// ShowCases holds S01 to S36, A01 to A18 and T27.
var ShowCases = []ShowCase{
	{ID: "S01", Name: "Copper.Valley.S01E01.1080p.WEB-DL.DDP5.1.H.264-QUILL", Kind: KindEpisode, Title: "Copper Valley", Show: sn(s1(1), 1), Resolution: 1080, Source: "web_dl", Codec: "h264", Audio: "eac3 5.1", Group: "QUILL"},
	{ID: "S02", Name: "Copper.Valley.S01E02.720p.HDTV.x264-OXBOW", Kind: KindEpisode, Title: "Copper Valley", Show: sn(s1(1), 2), Resolution: 720, Source: "tv", Codec: "h264", Group: "OXBOW"},
	{ID: "S03", Name: "copper_valley_1x03_hdtv_x264-oxbow", Kind: KindEpisode, Title: "Copper Valley", Show: sn(s1(1), 3), Source: "tv", Codec: "h264", Group: "oxbow"},
	{ID: "S04", Name: "Copper.Valley.S01E04E05.1080p.WEB.h264-MOSSFOX", Kind: KindMultiEpisode, Title: "Copper Valley", Show: sn(s1(1), 4, 5), Resolution: 1080, Source: "web_dl", SrcConf: Likely, Codec: "h264", Group: "MOSSFOX"},
	{ID: "S05", Name: "Copper.Valley.S01E06-E08.1080p.WEB-DL.H.265-QUILL", Kind: KindMultiEpisode, Title: "Copper Valley", Show: sn(s1(1), 6, 7, 8), Resolution: 1080, Source: "web_dl", Codec: "h265", Group: "QUILL"},
	{ID: "S06", Name: "Copper.Valley.S01.1080p.BluRay.x265.DDP5.1-LARKSPUR", Kind: KindSeasonPack, Title: "Copper Valley", Show: sn(s1(1)), Resolution: 1080, Source: "bluray", Codec: "h265", Audio: "eac3 5.1", Group: "LARKSPUR"},
	{ID: "S07", Name: "Copper.Valley.S01-S03.COMPLETE.1080p.WEB-DL.x264-GRAVELY", Kind: KindMultiSeason, Title: "Copper Valley", Show: sn([]int{1, 2, 3}), Resolution: 1080, Source: "web_dl", Codec: "h264", Group: "GRAVELY"},
	{ID: "S08", Name: "Copper.Valley.S00E03.Behind.the.Valley.1080p.WEB-DL-QUILL", Kind: KindSpecial, Title: "Copper Valley", Show: ShowNumbers{Seasons: s1(0), Episodes: []int{3}, Special: true}, Resolution: 1080, Source: "web_dl", Group: "QUILL"},
	{ID: "S09", Name: "Harbor.of.Glass.2019.S02E01.1080p.WEB-DL.H.264-QUILL", Kind: KindEpisode, Title: "Harbor of Glass", Year: 2019, Show: sn(s1(2), 1), Resolution: 1080, Source: "web_dl", Codec: "h264", Group: "QUILL"},
	{ID: "S10", Name: "The.Late.Lamp.Show.2026.03.14.Guest.Name.720p.WEB.h264-NIGHTDESK", Kind: KindDaily, Title: "The Late Lamp Show", Show: ShowNumbers{AirDate: "2026-03-14"}, Resolution: 720, Source: "web_dl", SrcConf: Likely, Codec: "h264", Group: "NIGHTDESK"},
	{ID: "S11", Name: "The.Late.Lamp.Show.2026-03-14.1080p.WEB-DL.x264-NIGHTDESK", Kind: KindDaily, Title: "The Late Lamp Show", Show: ShowNumbers{AirDate: "2026-03-14"}, Resolution: 1080, Source: "web_dl", Codec: "h264", Group: "NIGHTDESK"},
	{ID: "S12", Name: "The Late Lamp Show 14.03.2026 1080p WEB x264-NIGHTDESK", Kind: KindDaily, Title: "The Late Lamp Show", Show: ShowNumbers{AirDate: "2026-03-14", Conf: Sure}, Resolution: 1080, Source: "web_dl", SrcConf: Likely, Codec: "h264", Group: "NIGHTDESK"},
	{ID: "S13", Name: "Glass.Orchard.Part.2.1080p.WEB-DL.H.264-QUILL", Hints: Hints{Show: true}, Kind: KindEpisode, Title: "Glass Orchard", Show: sn(s1(1), 2), Resolution: 1080, Source: "web_dl", Codec: "h264", Group: "QUILL"},
	{ID: "S14", Name: "Glass.Orchard.Pt.III.720p.HDTV.x264-OXBOW", Hints: Hints{Show: true}, Kind: KindEpisode, Title: "Glass Orchard", Show: sn(s1(1), 3), Resolution: 720, Source: "tv", Codec: "h264", Group: "OXBOW"},
	{ID: "S15", Name: "Unit.24.S03E02.1080p.WEB-DL.H.265-MOSSFOX", Kind: KindEpisode, Title: "Unit 24", Show: sn(s1(3), 2), Resolution: 1080, Source: "web_dl", Codec: "h265", Group: "MOSSFOX"},
	{ID: "S16", Name: "Copper.Valley.S01E01.PROPER.1080p.WEB-DL.DDP5.1.H.264-QUILL", Kind: KindEpisode, Title: "Copper Valley", Show: sn(s1(1), 1), Resolution: 1080, Source: "web_dl", Codec: "h264", Audio: "eac3 5.1", Group: "QUILL", Fix: 1},
	{ID: "S17", Name: "Copper.Valley.S01E01.REPACK2.1080p.WEB-DL.H.264-QUILL", Kind: KindEpisode, Title: "Copper Valley", Show: sn(s1(1), 1), Resolution: 1080, Source: "web_dl", Codec: "h264", Group: "QUILL", Fix: 2},
	{ID: "S18", Name: "Copper.Valley.S01E01.REPACK.1080p.WEB-DL.H.264-TINCAN", Kind: KindEpisode, Title: "Copper Valley", Show: sn(s1(1), 1), Resolution: 1080, Source: "web_dl", Codec: "h264", Group: "TINCAN", Fix: 1},
	{ID: "S19", Name: "Copper.Valley.S02E10.1080p.WEB-DL.H.264.FRENCH-LESQUAIS", Kind: KindEpisode, Title: "Copper Valley", Show: sn(s1(2), 10), Resolution: 1080, Source: "web_dl", Codec: "h264", Langs: []string{"fr"}, Group: "LESQUAIS"},
	{ID: "S20", Name: "Copper.Valley.S02E10.MULTi.1080p.WEB-DL.H.264-LESQUAIS", Kind: KindEpisode, Title: "Copper Valley", Show: sn(s1(2), 10), Resolution: 1080, Source: "web_dl", Codec: "h264", Group: "LESQUAIS", Flags: []string{"multi"}},
	{ID: "S21", Name: "Copper.Valley.S02E11.1080p.WEB-DL.H.264.HC.KORSUB-GRAVELY", Kind: KindEpisode, Title: "Copper Valley", Show: sn(s1(2), 11), Resolution: 1080, Source: "web_dl", Codec: "h264", Group: "GRAVELY", Flags: []string{"hardcoded_subs"}},
	{ID: "S22", Name: "Copper.Valley.S02.E12.1080p.WEB.x264-OXBOW", Kind: KindEpisode, Title: "Copper Valley", Show: sn(s1(2), 12), Resolution: 1080, Source: "web_dl", SrcConf: Likely, Codec: "h264", Group: "OXBOW"},
	{ID: "S23", Name: "Copper Valley - 2x13 - The Dam Breaks [1080p]", Kind: KindEpisode, Title: "Copper Valley", Show: sn(s1(2), 13), Resolution: 1080},
	{ID: "S24", Name: "Copper.Valley.S03E01.2160p.WEB-DL.DV.H.265-QUILL", Kind: KindEpisode, Title: "Copper Valley", Show: sn(s1(3), 1), Resolution: 2160, Source: "web_dl", Codec: "h265", Group: "QUILL"},
	{ID: "S25", Name: "9.Lives.of.Mara.Penn.S01E09.720p.WEB-DL.H.264-MOSSFOX", Kind: KindEpisode, Title: "9 Lives of Mara Penn", Show: sn(s1(1), 9), Resolution: 720, Source: "web_dl", Codec: "h264", Group: "MOSSFOX"},
	{ID: "S26", Name: "The.Hundred.Doors.S04E00.Christmas.Special.1080p.WEB-DL-QUILL", Kind: KindSpecial, Title: "The Hundred Doors", Show: ShowNumbers{Seasons: s1(4), Episodes: []int{0}, Special: true}, Resolution: 1080, Source: "web_dl", Group: "QUILL"},
	{ID: "S27", Name: "Copper.Valley.S01E01.Sample.1080p.WEB-DL-QUILL", Hints: Hints{Size: 45 << 20}, Kind: KindEpisode, Title: "Copper Valley", Show: sn(s1(1), 1), Resolution: 1080, Source: "web_dl", Group: "QUILL", Flags: []string{"sample"}},
	{ID: "S28", Name: "Copper.Valley.S04.1080p.WEB-DL.H.264-QUILL", Kind: KindSeasonPack, Title: "Copper Valley", Show: sn(s1(4)), Resolution: 1080, Source: "web_dl", Codec: "h264", Group: "QUILL"},
	{ID: "S29", Name: "Copper.Valley.S01E06-08.1080p.WEB-DL.H.264-QUILL", Kind: KindMultiEpisode, Title: "Copper Valley", Show: sn(s1(1), 6, 7, 8), Resolution: 1080, Source: "web_dl", Codec: "h264", Group: "QUILL"},
	{ID: "S30", Name: "Copper.Valley.Season.2.1080p.WEB-DL.H.264-QUILL", Kind: KindSeasonPack, Title: "Copper Valley", Show: sn(s1(2)), Resolution: 1080, Source: "web_dl", Codec: "h264", Group: "QUILL"},
	{ID: "S31", Name: "Copper.Valley.COMPLETE.SERIES.1080p.BluRay.x264-LARKSPUR", Kind: KindComplete, Title: "Copper Valley", Resolution: 1080, Source: "bluray", Codec: "h264", Group: "LARKSPUR"},
	{ID: "S32", Name: "The.Late.Lamp.Show.03.25.2026.720p.HDTV.x264-NIGHTDESK", Kind: KindDaily, Title: "The Late Lamp Show", Show: ShowNumbers{AirDate: "2026-03-25", Conf: Sure}, Resolution: 720, Source: "tv", Codec: "h264", Group: "NIGHTDESK"},
	{ID: "S33", Name: "The Late Lamp Show 04.03.2026 1080p WEB-DL x264-NIGHTDESK", Kind: KindDaily, Title: "The Late Lamp Show", Show: ShowNumbers{AirDate: "2026-03-04", Conf: Likely}, Resolution: 1080, Source: "web_dl", Codec: "h264", Group: "NIGHTDESK"},
	{ID: "S34", Name: "copper.valley.s02e03.720p.hdtv.x264-oxbow", Kind: KindEpisode, Title: "Copper Valley", Show: sn(s1(2), 3), Resolution: 720, Source: "tv", Codec: "h264", Group: "oxbow"},
	{ID: "S35", Name: "Copper.Valley.S01E01E02E03.720p.WEB-DL.x264-TINCAN", Kind: KindMultiEpisode, Title: "Copper Valley", Show: sn(s1(1), 1, 2, 3), Resolution: 720, Source: "web_dl", Codec: "h264", Group: "TINCAN"},
	{ID: "S36", Name: "Copper Valley S03 2160p WEB-DL DV H.265-QUILL", Kind: KindSeasonPack, Title: "Copper Valley", Show: sn(s1(3)), Resolution: 2160, Source: "web_dl", Codec: "h265", Group: "QUILL"},

	{ID: "A01", Name: "[Hoshizora] Starfall Kitchen - 112 [1080p]", Kind: KindAbsolute, Title: "Starfall Kitchen", Show: ShowNumbers{Absolute: []int{112}}, Resolution: 1080, Source: "web_dl", SrcConf: Likely, Group: "Hoshizora"},
	{ID: "A02", Name: "[Hoshizora] Starfall Kitchen - 112v2 [1080p][HEVC]", Kind: KindAbsolute, Title: "Starfall Kitchen", Show: ShowNumbers{Absolute: []int{112}}, Resolution: 1080, Source: "web_dl", SrcConf: Likely, Codec: "h265", Group: "Hoshizora", Fix: 1},
	{ID: "A03", Name: "[Kumomaru] Starfall Kitchen - 113 (1080p WEB-DL AAC) [A1B2C3D4]", Kind: KindAbsolute, Title: "Starfall Kitchen", Show: ShowNumbers{Absolute: []int{113}}, Resolution: 1080, Source: "web_dl", Audio: "aac", Group: "Kumomaru", Flags: []string{"crc"}},
	{ID: "A04", Name: "Starfall.Kitchen.S05E12.1080p.WEB-DL.H.264-Kumomaru", Kind: KindEpisode, Title: "Starfall Kitchen", Show: sn(s1(5), 12), Resolution: 1080, Source: "web_dl", Codec: "h264", Group: "Kumomaru"},
	{ID: "A05", Name: "[Hoshizora] Starfall Kitchen - 101-112 [1080p] (Batch)", Kind: KindAbsolute, Title: "Starfall Kitchen", Show: ShowNumbers{Absolute: span(101, 112)}, Resolution: 1080, Source: "web_dl", SrcConf: Likely, Group: "Hoshizora"},
	{ID: "A06", Name: "[Hoshizora] Starfall Kitchen - OVA 02 [1080p]", Kind: KindSpecial, Title: "Starfall Kitchen", Show: ShowNumbers{Seasons: s1(0), Episodes: []int{2}, Special: true}, Resolution: 1080, Source: "web_dl", SrcConf: Likely, Group: "Hoshizora"},
	{ID: "A07", Name: "[Kumomaru] Starfall Kitchen S2 - 07 [720p]", Kind: KindEpisode, Title: "Starfall Kitchen", Show: sn(s1(2), 7), Resolution: 720, Source: "web_dl", SrcConf: Likely, Group: "Kumomaru"},
	{ID: "A16", Name: "[Erai-raws] Mashle 2nd Season - 05 [1080p]", Kind: KindEpisode, Title: "Mashle", Show: sn(s1(2), 5), Resolution: 1080, Source: "web_dl", SrcConf: Likely, Group: "Erai-raws"},
	{ID: "A17", Name: "[Kumomaru] Mashle Season 2 - 05 [1080p]", Kind: KindEpisode, Title: "Mashle", Show: sn(s1(2), 5), Resolution: 1080, Source: "web_dl", SrcConf: Likely, Group: "Kumomaru"},
	{ID: "A18", Name: "[Kumomaru] Starfall Kitchen 3rd Season - 12v2 [720p]", Kind: KindEpisode, Title: "Starfall Kitchen", Show: sn(s1(3), 12), Resolution: 720, Source: "web_dl", SrcConf: Likely, Group: "Kumomaru", Fix: 1},
	{ID: "A08", Name: "[Hoshizora] Kaze no Tsubasa - 03 [1080p][Dual Audio]", Kind: KindAbsolute, Title: "Kaze no Tsubasa", Show: ShowNumbers{Absolute: []int{3}}, Resolution: 1080, Source: "web_dl", SrcConf: Likely, Group: "Hoshizora", Flags: []string{"dual"}},
	{ID: "A09", Name: "[Kumomaru] 風の翼 - 04 [1080p]", Kind: KindAbsolute, Title: "風の翼", Show: ShowNumbers{Absolute: []int{4}}, Resolution: 1080, Source: "web_dl", SrcConf: Likely, Group: "Kumomaru"},
	{ID: "A10", Name: "[Hoshizora] Starfall Kitchen - 112 [1080p][HardSub]", Kind: KindAbsolute, Title: "Starfall Kitchen", Show: ShowNumbers{Absolute: []int{112}}, Resolution: 1080, Source: "web_dl", SrcConf: Likely, Group: "Hoshizora", Flags: []string{"hardcoded_subs"}},
	{ID: "A11", Name: "Starfall Kitchen - 1080 [1080p]", Hints: Hints{Show: true}, Kind: KindAbsolute, Title: "Starfall Kitchen", Show: ShowNumbers{Absolute: []int{1080}, Conf: Guess}, Resolution: 1080},
	{ID: "A12", Name: "[Kumomaru] Starfall Kitchen - 112 (BD 1080p FLAC)", Kind: KindAbsolute, Title: "Starfall Kitchen", Show: ShowNumbers{Absolute: []int{112}}, Resolution: 1080, Source: "bluray", Audio: "flac", Group: "Kumomaru"},
	{ID: "A13", Name: "[Hoshizora] Starfall Kitchen - 112v3 [1080p]", Kind: KindAbsolute, Title: "Starfall Kitchen", Show: ShowNumbers{Absolute: []int{112}}, Resolution: 1080, Source: "web_dl", SrcConf: Likely, Group: "Hoshizora", Fix: 2},
	{ID: "A14", Name: "[Hoshizora] Starfall Kitchen - 112 [480p]", Kind: KindAbsolute, Title: "Starfall Kitchen", Show: ShowNumbers{Absolute: []int{112}}, Resolution: 480, Source: "web_dl", SrcConf: Likely, Group: "Hoshizora"},
	{ID: "A15", Name: "[Lab Fansub] Starfall Kitchen - 112 [1080p]", Kind: KindAbsolute, Title: "Starfall Kitchen", Show: ShowNumbers{Absolute: []int{112}}, Resolution: 1080, Source: "web_dl", SrcConf: Likely, Group: "Lab Fansub"},

	{ID: "T27", Name: "Unit 24 (2008) S01E01 1080p BluRay x264-OXBOW", Kind: KindEpisode, Title: "Unit 24", Year: 2008, Show: sn(s1(1), 1), Resolution: 1080, Source: "bluray", Codec: "h264", Group: "OXBOW"},
}

func TestShowNames(t *testing.T) {
	for _, c := range ShowCases {
		t.Run(c.ID, func(t *testing.T) {
			p := Parse(c.Name, c.Hints)
			if p.Kind.Value != c.Kind {
				t.Errorf("kind %q, want %q", p.Kind.Value, c.Kind)
			}
			if p.Show == nil {
				t.Fatalf("no show numbers")
			}
			got := *p.Show
			want := c.Show
			if want.Conf != "" && got.Conf != want.Conf {
				t.Errorf("numbers confidence %s, want %s", got.Conf, want.Conf)
			}
			got.Conf, want.Conf = "", ""
			if !reflect.DeepEqual(got, want) {
				t.Errorf("numbers %+v, want %+v", got, want)
			}
			if CleanName(p.Title.Value) != CleanName(c.Title) {
				t.Errorf("title %q, want %q", p.Title.Value, c.Title)
			}
			if p.Year.Value != c.Year {
				t.Errorf("year %d, want %d", p.Year.Value, c.Year)
			}
			if p.Resolution.Value != c.Resolution {
				t.Errorf("resolution %d, want %d", p.Resolution.Value, c.Resolution)
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
			if a := audioString(p.Audio); a != c.Audio {
				t.Errorf("audio %q, want %q", a, c.Audio)
			}
			if !slices.Equal(p.Languages, c.Langs) {
				t.Errorf("languages %v, want %v", p.Languages, c.Langs)
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

// "Season 1 - 2" is a range of seasons and "Season 2 - 1080p" a quality, not
// an episode; a plain "- 17" stays a running number.
func TestSeasonDashNeedsEpisode(t *testing.T) {
	for _, c := range []struct {
		name, kind, title string
		show              ShowNumbers
	}{
		{"[Kumomaru] Starfall Kitchen Season 1 - 2 [Batch]", KindAbsolute, "Starfall Kitchen Season 1", ShowNumbers{Absolute: []int{2}}},
		{"[Kumomaru] Starfall Kitchen Season 2 - 1080p", KindSeasonPack, "Starfall Kitchen", sn(s1(2))},
		{"[SubsPlease] Mashle - 17 (1080p)", KindAbsolute, "Mashle", ShowNumbers{Absolute: []int{17}}},
		// A batch of the season is every episode of it, not the first one.
		{"[Kumomaru] Mashle Season 2 - 01-12 [1080p]", KindMultiEpisode, "Mashle", sn(s1(2), span(1, 12)...)},
		{"[Kumomaru] Mashle Season 2 - 01 - 12 [1080p]", KindMultiEpisode, "Mashle", sn(s1(2), span(1, 12)...)},
		{"[Erai-raws] Mashle 2nd Season - 01 ~ 12 [1080p]", KindMultiEpisode, "Mashle", sn(s1(2), span(1, 12)...)},
		{"[Erai-raws] Mashle 2nd Season - 05 [1080p]", KindEpisode, "Mashle", sn(s1(2), 5)},
	} {
		p := Parse(c.name, Hints{Show: true})
		if p.Kind.Value != c.kind || p.Show == nil || p.Title.Value != c.title {
			t.Errorf("%s: kind %q title %q %+v", c.name, p.Kind.Value, p.Title.Value, p.Show)
			continue
		}
		got := *p.Show
		got.Conf = ""
		if !reflect.DeepEqual(got, c.show) {
			t.Errorf("%s: numbers %+v, want %+v", c.name, got, c.show)
		}
	}
}

// Names that look a little like shows but are movies.
func TestShowLookalikesAreMovies(t *testing.T) {
	for _, n := range []string{
		"Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS",
		"Saga.Part.2.2024.1080p.WEB-DL-QUILL",
		"The.Paper.Comet.2009.COMPLETE.BLURAY-DISCWORKS",
		"Velvet Static (2024) [1080p] [WEBRip] [5.1] [YTSLIKE]",
		"300.Lanterns.2015.720p.BluRay.x264-OXBOW",
		"Movie.Title.Part.2.1080p.BluRay.x264-GRP",
		"Some Movie - 2019 [1080p]",
		"Film.2010.DD5.1x264-GRP",
		"Film.2010.DD5.1x265-GRP",
	} {
		if p := Parse(n, Hints{}); p.Kind.Value != "movie" || p.Show != nil {
			t.Errorf("%s: kind %q %+v", n, p.Kind.Value, p.Show)
		}
	}
}

// A name judged for a show still never reads a year as a running number.
func TestShowYearIsNoRunningNumber(t *testing.T) {
	p := Parse("Some Movie - 2019 [1080p]", Hints{Show: true})
	if p.Show != nil && slices.Contains(p.Show.Absolute, 2019) {
		t.Fatalf("2019 read as a running number: %+v", p.Show)
	}
}

// A mini-series part with no tags after it is a part even without the hint;
// with tags after it, only a show search reads it as one.
func TestPartNeedsShowOrNoTags(t *testing.T) {
	if p := Parse("Glass Orchard Part 2", Hints{}); p.Kind.Value != KindEpisode {
		t.Errorf("bare part: kind %q", p.Kind.Value)
	}
	if p := Parse("Glass.Orchard.Part.2.1080p.WEB-DL.H.264-QUILL", Hints{}); p.Kind.Value != "movie" {
		t.Errorf("part with tags, no hint: kind %q", p.Kind.Value)
	}
}

// A running number range wider than 500 is not read as a range.
func TestWideRangeIsNoRange(t *testing.T) {
	for _, n := range []string{"[Grp] Some Show - 1-900 [1080p]", "Some.Show.S01E01-E900.1080p.WEB-DL-QUILL"} {
		p := Parse(n, Hints{})
		if p.Show != nil && (len(p.Show.Absolute) > 1 || len(p.Show.Episodes) > 1) {
			t.Errorf("%s: read as a range %+v", n, p.Show)
		}
	}
}

// 1x02 needs a separator or the start before the season number.
func TestCrossNeedsSeparator(t *testing.T) {
	if p := Parse("Copper.Valley.1x03.HDTV.x264-OXBOW", Hints{}); p.Show == nil || !slices.Equal(p.Show.Episodes, []int{3}) {
		t.Errorf("1x03: %+v", p.Show)
	}
	if p := Parse("Copper.Valley.HDTV11x03.x264-OXBOW", Hints{}); p.Show != nil {
		t.Errorf("glued 11x03 read as episode: %+v", p.Show)
	}
}
