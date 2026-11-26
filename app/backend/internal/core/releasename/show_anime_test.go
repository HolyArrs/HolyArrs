package releasename

import (
	"reflect"
	"slices"
	"testing"
)

// animeShowCases are anime release titles with a CJK alt-title next to the
// Latin one, or in the [Group][Title][Episode] layout. The row type is the
// same as ShowCases.
var animeShowCases = []ShowCase{
	// A CJK alt-title in the title text, before or after the Latin one.
	{ID: "A19", Name: "[7³ACG] New Panty & Stocking with Garterbelt パンティ & ストッキングwithガーターベルト S2 [BDRip 1080p AV1 OPUS]", Kind: KindSeasonPack, Title: "New Panty & Stocking with Garterbelt", Show: sn(s1(2)), Resolution: 1080, Source: "bluray", Codec: "av1", Audio: "opus", Group: "7³ACG"},
	{ID: "A20", Name: "[Kumomaru] 新吊带袜天使 New Panty & Stocking S2 [1080p]", Kind: KindSeasonPack, Title: "New Panty & Stocking", Show: sn(s1(2)), Resolution: 1080, Source: "web_dl", SrcConf: Likely, Group: "Kumomaru"},
	{ID: "A21", Name: "[Kumomaru] Kaze no Tsubasa 風の翼 - 04 [1080p]", Kind: KindAbsolute, Title: "Kaze no Tsubasa", Show: ShowNumbers{Absolute: []int{4}}, Resolution: 1080, Source: "web_dl", SrcConf: Likely, Group: "Kumomaru"},
	{ID: "A22", Name: "Kaze no Tsubasa 風の翼 S01E02 1080p WEB-DL-GRP", Kind: KindEpisode, Title: "Kaze no Tsubasa", Show: sn(s1(1), 2), Resolution: 1080, Source: "web_dl", Group: "GRP"},
	{ID: "A23", Name: "[Kumomaru] 風の翼 ストッキングwith - 04 [1080p]", Kind: KindAbsolute, Title: "風の翼 ストッキングwith", Show: ShowNumbers{Absolute: []int{4}}, Resolution: 1080, Source: "web_dl", SrcConf: Likely, Group: "Kumomaru"},

	// Latin words after a mid-title CJK word stay. Mixed CJK/Latin words and pure-CJK or CJK-first titles still work.
	{ID: "A40", Name: "[Grp] Sword Art Online アリシゼーション Alicization - 05 [1080p]", Kind: KindAbsolute, Title: "Sword Art Online Alicization", Show: ShowNumbers{Absolute: []int{5}}, Resolution: 1080, Source: "web_dl", SrcConf: Likely, Group: "Grp"},
	{ID: "A42", Name: "[Grp] The Promised Neverland 約束のネバーランド The Final Season - 05 [1080p]", Kind: KindAbsolute, Title: "The Promised Neverland The Final Season", Show: ShowNumbers{Absolute: []int{5}}, Resolution: 1080, Source: "web_dl", SrcConf: Likely, Group: "Grp"},
	{ID: "A43", Name: "[Grp] 進撃の巨人 Attack on Titan - 05 [1080p]", Kind: KindAbsolute, Title: "Attack on Titan", Show: ShowNumbers{Absolute: []int{5}}, Resolution: 1080, Source: "web_dl", SrcConf: Likely, Group: "Grp"},
	{ID: "A44", Name: "[Grp] 86 Eighty-Six 86-エイティシックス - 05 [1080p]", Kind: KindAbsolute, Title: "86 Eighty-Six", Show: ShowNumbers{Absolute: []int{5}}, Resolution: 1080, Source: "web_dl", SrcConf: Likely, Group: "Grp"},
	{ID: "A45", Name: "[Grp] Mob Psycho 100 モブサイコ100 - 05 [1080p]", Kind: KindAbsolute, Title: "Mob Psycho 100", Show: ShowNumbers{Absolute: []int{5}}, Resolution: 1080, Source: "web_dl", SrcConf: Likely, Group: "Grp"},
	{ID: "A46", Name: "[Grp] Fate/ゼロ Fate Zero - 05 [1080p]", Kind: KindAbsolute, Title: "Fate Zero", Show: ShowNumbers{Absolute: []int{5}}, Resolution: 1080, Source: "web_dl", SrcConf: Likely, Group: "Grp"},

	// Fansub names with the title in its own bracket after the group:
	// [Group][Title][01-12][tags]. The indexer turned the hyphen of the range
	// into a space.
	{ID: "A60", Name: "[DBD Raws][新吊带袜天使 New PANTY STOCKING with GARTERBELT][01 13TV全集 特典映像][1080P][BDRip][HEVC 10bit][简繁日双语外挂][FLAC AC3][MKV](新しいパンティ＆ストッキングwithガーターベルト)", Hints: Hints{Show: true}, Kind: KindAbsolute, Title: "New PANTY STOCKING with GARTERBELT", Show: ShowNumbers{Absolute: span(1, 13)}, Resolution: 1080, Source: "bluray", Codec: "h265", Audio: "flac", Group: "DBD Raws"},
	{ID: "A61", Name: "[Hoshizora][Starfall Kitchen][01-12][1080P][WEB-DL]", Kind: KindAbsolute, Title: "Starfall Kitchen", Show: ShowNumbers{Absolute: span(1, 12)}, Resolution: 1080, Source: "web_dl", Group: "Hoshizora"},
	{ID: "A62", Name: "[Hoshizora][Starfall Kitchen][01 12][1080P][WEB-DL]", Kind: KindAbsolute, Title: "Starfall Kitchen", Show: ShowNumbers{Absolute: span(1, 12)}, Resolution: 1080, Source: "web_dl", Group: "Hoshizora"},
	{ID: "A63", Name: "[Hoshizora][Starfall Kitchen][05v2][1080P][HEVC]", Kind: KindAbsolute, Title: "Starfall Kitchen", Show: ShowNumbers{Absolute: []int{5}}, Resolution: 1080, Source: "web_dl", SrcConf: Likely, Codec: "h265", Group: "Hoshizora", Fix: 1},
	{ID: "A64", Name: "[Kumomaru][風の翼 Kaze no Tsubasa][03][1080P]", Kind: KindAbsolute, Title: "Kaze no Tsubasa", Show: ShowNumbers{Absolute: []int{3}}, Resolution: 1080, Source: "web_dl", SrcConf: Likely, Group: "Kumomaru"},
	{ID: "A65", Name: "[Kumomaru][風の翼][03][1080P]", Kind: KindAbsolute, Title: "風の翼", Show: ShowNumbers{Absolute: []int{3}}, Resolution: 1080, Source: "web_dl", SrcConf: Likely, Group: "Kumomaru"},

	// The episode bracket may have tags after the number. A second number
	// there ends a range only when it is not a tag. Tag-only brackets before
	// the episode bracket are skipped.
	{ID: "A50", Name: "[Hoshizora][Starfall Kitchen][03 1080P][WEB-DL]", Kind: KindAbsolute, Title: "Starfall Kitchen", Show: ShowNumbers{Absolute: []int{3}}, Resolution: 1080, Source: "web_dl", Group: "Hoshizora"},
	{ID: "A51", Name: "[Hoshizora][Starfall Kitchen][03 2160p][WEB-DL]", Kind: KindAbsolute, Title: "Starfall Kitchen", Show: ShowNumbers{Absolute: []int{3}}, Resolution: 2160, Source: "web_dl", Group: "Hoshizora"},
	{ID: "A52", Name: "[Hoshizora][Starfall Kitchen][3 1280x720][WEB-DL]", Hints: Hints{Show: true}, Kind: KindAbsolute, Title: "Starfall Kitchen", Show: ShowNumbers{Absolute: []int{3}}, Resolution: 720, Source: "web_dl", Group: "Hoshizora"},
	{ID: "A53", Name: "[Hoshizora][Starfall Kitchen][3 1920x1080][WEB-DL]", Hints: Hints{Show: true}, Kind: KindAbsolute, Title: "Starfall Kitchen", Show: ShowNumbers{Absolute: []int{3}}, Resolution: 1080, Source: "web_dl", Group: "Hoshizora"},
	{ID: "A54", Name: "[Hoshizora][Starfall Kitchen][03 BD][1080P]", Kind: KindAbsolute, Title: "Starfall Kitchen", Show: ShowNumbers{Absolute: []int{3}}, Resolution: 1080, Source: "bluray", Group: "Hoshizora"},
	{ID: "A55", Name: "[Hoshizora][Starfall Kitchen][03 5.1][1080P]", Kind: KindAbsolute, Title: "Starfall Kitchen", Show: ShowNumbers{Absolute: []int{3}}, Resolution: 1080, Source: "web_dl", SrcConf: Likely, Audio: "5.1", Group: "Hoshizora"},
	{ID: "A56", Name: "[Hoshizora][Starfall Kitchen][03 4K][WEB-DL]", Kind: KindAbsolute, Title: "Starfall Kitchen", Show: ShowNumbers{Absolute: []int{3}}, Resolution: 2160, Source: "web_dl", Group: "Hoshizora"},
	{ID: "A57", Name: "[Hoshizora][Starfall Kitchen][1080p][01][WEB-DL]", Kind: KindAbsolute, Title: "Starfall Kitchen", Show: ShowNumbers{Absolute: []int{1}}, Resolution: 1080, Source: "web_dl", Group: "Hoshizora"},
	{ID: "A58", Name: "[Hoshizora][Starfall Kitchen][10bit][01][1080p]", Kind: KindAbsolute, Title: "Starfall Kitchen", Show: ShowNumbers{Absolute: []int{1}}, Resolution: 1080, Source: "web_dl", SrcConf: Likely, Group: "Hoshizora"},
}

func TestAnimeShowNames(t *testing.T) {
	for _, c := range animeShowCases {
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

// A bracket with only a resolution, a year or a codec is a tag, not the
// episode numbers of a [Group][Title][01] name, so such names parse as they
// did before. A number that is a resolution or codec name, a range that runs
// backwards, and a bare number on a movie search are not episodes either.
func TestBracketTitleNeedsEpisodeBracket(t *testing.T) {
	for _, c := range []struct {
		name string
		h    Hints
	}{
		{"[Kumomaru][Starfall Kitchen][1080P][WEB-DL]", Hints{Show: true}},
		{"[Kumomaru][Starfall Kitchen][2019][1080P]", Hints{Show: true}},
		{"[Kumomaru][Starfall Kitchen][1080 x264][WEB-DL]", Hints{Show: true}},
		{"[Kumomaru][Starfall Kitchen][720][1080p]", Hints{Show: true}},
		{"[Kumomaru][Starfall Kitchen][264][1080p]", Hints{Show: true}},
		{"[Kumomaru][Starfall Kitchen][265][1080p]", Hints{Show: true}},
		{"[Kumomaru][Starfall Kitchen][12-01][1080p]", Hints{Show: true}},
		{"[Kumomaru][Starfall Kitchen][2][1080p]", Hints{}},
	} {
		if p := Parse(c.name, c.h); p.Show != nil && len(p.Show.Absolute) > 0 && p.Title.Value == "Starfall Kitchen" {
			t.Errorf("%s: read as episodes %+v", c.name, p.Show)
		}
	}
}

// [Group][Title][2][1080p] is a show only when the search is for a show or
// the number is zero-padded or a range.
func TestBracketTitleShowHint(t *testing.T) {
	if p := Parse("[Kumomaru][Starfall Kitchen][2][1080p]", Hints{Show: true}); p.Show == nil || !slices.Equal(p.Show.Absolute, []int{2}) {
		t.Errorf("show search: %+v", p.Show)
	}
	if p := Parse("[Kumomaru][Starfall Kitchen][02][1080p]", Hints{}); p.Show == nil || !slices.Equal(p.Show.Absolute, []int{2}) {
		t.Errorf("zero-padded: %+v", p.Show)
	}
	if p := Parse("[Kumomaru][Starfall Kitchen][05 12][1080p]", Hints{}); p.Show == nil || !slices.Equal(p.Show.Absolute, span(5, 12)) {
		t.Errorf("range: %+v", p.Show)
	}
}

func TestRomaji(t *testing.T) {
	for in, want := range map[string]string{
		"ゼロ":      "zero",
		"ストッキング":  "sutokkingu",
		"ガーター":    "gata",
		"シャチ":     "shachi",
		"きょう":     "kyou",
		"ジュース":    "jusu",
		"チェック":    "chekku",
		"フィルム":    "firumu",
		"パティ":     "pati",
		"ウィ":      "wi",
		"マッチ":     "matchi",
		"ヴァイオリン":  "vaiorin",
		"Re:ゼロ":   "Re:zero",
		"さんぽ":     "sanpo",
		"ネバーランド":  "nebarando",
		"ファイナルfo": "fainarufo",
	} {
		if got := romaji(in); got != want {
			t.Errorf("romaji(%q) = %q, want %q", in, got, want)
		}
	}
}

// A word that mixes Latin letters with kana is also read with the kana
// spelled out, as a second title. The first reading drops the word as before.
// A word with kanji, or a word of kana alone, gets no second reading.
func TestKanaTitle(t *testing.T) {
	for _, c := range []struct{ name, title, kana string }{
		{"[Grp] Re:ゼロ Starting Life in Another World - 05 [1080p]", "Starting Life in Another World", "Re:zero Starting Life in Another World"},
		{"[Grp][Re:ゼロ Starting Life in Another World][01-12][1080p]", "Starting Life in Another World", "Re:zero Starting Life in Another World"},
		{"[Grp] Starting Life in Another World Re:ゼロ - 05 [1080p]", "Starting Life in Another World", "Starting Life in Another World Re:zero"},
		{"[Grp] Re:ゼロから Starting Life in Another World - 05 [1080p]", "Starting Life in Another World", "Re:zerokara Starting Life in Another World"},
		{"[Grp] Re:ゼロ從 Starting Life in Another World - 05 [1080p]", "Starting Life in Another World", ""},
		{"[Grp] ゼロ Starting Life in Another World - 05 [1080p]", "Starting Life in Another World", ""},
		{"[Grp] Attack on Titan - 05 [1080p]", "Attack on Titan", ""},
		{"[Grp] 進撃の巨人 Attack on Titan - 05 [1080p]", "Attack on Titan", ""},
	} {
		p := Parse(c.name, Hints{Show: true})
		if p.Title.Value != c.title || p.KanaTitle != c.kana {
			t.Errorf("%s: title %q kana %q, want %q %q", c.name, p.Title.Value, p.KanaTitle, c.title, c.kana)
		}
	}
}
