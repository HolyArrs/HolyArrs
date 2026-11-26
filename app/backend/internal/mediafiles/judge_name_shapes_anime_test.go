package mediafiles

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// A CJK alt-title before or after the Latin title is left out of the Title
// that gets matched.
func TestNameShapesCJKAltTitle(t *testing.T) {
	lib := []TitleRef{{ID: 1, Name: "New PANTY & STOCKING with GARTERBELT", Year: 2025, Kind: "show", OriginalLanguage: "ja"}}
	for _, rel := range []string{
		"[7³ACG] New Panty & Stocking with Garterbelt パンティ & ストッキングwithガーターベルト S2 [BDRip 1080p AV1 OPUS]",
		"[Kumomaru] 新吊带袜天使 New Panty & Stocking with Garterbelt S2 [1080p]",
	} {
		p := releasename.Parse(rel, releasename.Hints{Show: true})
		if m := MatchTitle(p, lib); m.Title == nil {
			t.Errorf("%q: no match; parsed title %q; %s", rel, p.Title.Value, m.Detail)
		}
	}
}

// Latin words after a CJK word in the middle of the title stay in the Title.
func TestNameShapesCJKAltTitleMidTitle(t *testing.T) {
	for _, c := range []struct {
		rel, lib, kind string
		show           bool
	}{
		{"[Grp] Sword Art Online アリシゼーション Alicization - 05 [1080p]", "Sword Art Online Alicization", "show", true},
		{"Star Wars スター・ウォーズ Episode IV 2010 1080p BluRay x264-GRP", "Star Wars Episode IV", "movie", false},
		{"[Grp] The Promised Neverland 約束のネバーランド The Final Season - 05 [1080p]", "The Promised Neverland The Final Season", "show", true},
	} {
		p := releasename.Parse(c.rel, releasename.Hints{Show: c.show})
		lib := []TitleRef{{ID: 1, Name: c.lib, Year: 2010, Kind: c.kind, OriginalLanguage: "ja"}}
		if m := MatchTitle(p, lib); m.Title == nil {
			t.Errorf("%q: no match; parsed title %q; %s", c.rel, p.Title.Value, m.Detail)
		}
	}
}

// A fansub name with the title in its own bracket and a CJK alt-title before
// the Latin one still finds the show.
func TestNameShapesBracketTitle(t *testing.T) {
	lib := []TitleRef{{ID: 1, Name: "New PANTY STOCKING with GARTERBELT", Year: 2025, Kind: "show", OriginalLanguage: "ja"}}
	for _, rel := range []string{
		"[DBD Raws][新吊带袜天使 New PANTY STOCKING with GARTERBELT][01 13TV全集 特典映像][1080P][BDRip][HEVC 10bit][简繁日双语外挂][FLAC AC3][MKV](新しいパンティ＆ストッキングwithガーターベルト)",
		"[DBD Raws][New PANTY STOCKING with GARTERBELT][01-13][1080P][BDRip]",
		"[DBD Raws] New PANTY STOCKING with GARTERBELT - 05 [1080p]",
	} {
		p := releasename.Parse(rel, releasename.Hints{Show: true})
		if m := MatchTitle(p, lib); m.Title == nil {
			t.Errorf("%q: no match; parsed title %q; %s", rel, p.Title.Value, m.Detail)
		}
	}
}

// A word that mixes Latin letters and kana ("Re:ゼロ") is read with the kana
// spelled out, and that spelling has to match a name of the show exactly.
func TestNameShapesKanaSpelling(t *testing.T) {
	zero := TitleRef{ID: 1, Name: "Re:ZERO -Starting Life in Another World-", Year: 2016, Kind: "show", OriginalLanguage: "ja",
		Aliases: []string{"Re:Zero - Starting Life in Another World", "Re:ゼロから始める異世界生活"}}
	aliasOnly := TitleRef{ID: 1, Name: "Re: Life in a Different World from Zero", Year: 2016, Kind: "show", OriginalLanguage: "ja",
		Aliases: []string{"Re:Zero - Starting Life in Another World"}}
	for _, c := range []struct {
		rel  string
		lib  TitleRef
		want bool
	}{
		{"[Grp] Re:ゼロ Starting Life in Another World - 05 [1080p]", zero, true},
		{"[Grp] Re:ゼロ Starting Life in Another World - 05 [1080p]", aliasOnly, true},
		{"[Grp][Re:ゼロ Starting Life in Another World][01-12][1080p]", aliasOnly, true},
		// The kana change which name it spells, so a different show stays out.
		{"[Grp] Re:ゼロ Starting Life in Another World - 05 [1080p]",
			TitleRef{ID: 2, Name: "Re:Life in Another World", Year: 2016, Kind: "show", Aliases: []string{"Starting Over"}}, false},
		{"[Grp] Re:ゼロ Starting Life in Another World - 05 [1080p]",
			TitleRef{ID: 3, Name: "Starting Life in Another World", Year: 2016, Kind: "show"}, true}, // the old reading still matches
		{"[Grp] Re:ゼロ Starting Life in Another World - 05 [1080p]",
			TitleRef{ID: 4, Name: "Re:Zero Starting Over", Year: 2016, Kind: "show", Aliases: []string{"Zero"}}, false},
		// A word with kanji is left out whole, as before.
		{"[Grp] Re:ゼロ從 Starting Life in Another World - 05 [1080p]", aliasOnly, false},
		// A word of kana alone is not spelled out.
		{"[Grp] ゼロ Starting Life in Another World - 05 [1080p]",
			TitleRef{ID: 5, Name: "Zero Starting Life in Another World", Year: 2016, Kind: "show"}, false},
	} {
		p := releasename.Parse(c.rel, releasename.Hints{Show: true})
		if m := MatchTitle(p, []TitleRef{c.lib}); (m.Title != nil) != c.want {
			t.Errorf("%q vs %q: matched %v, want %v; parsed title %q kana %q", c.rel, c.lib.Name, m.Title != nil, c.want, p.Title.Value, p.KanaTitle)
		}
	}
}
