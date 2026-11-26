package metadata

import "testing"

func TestOtherLogosPicksBestPNGPerLanguage(t *testing.T) {
	s := func(v string) *string { return &v }
	got := otherLogos([]tmdbLogo{
		{FilePath: "/en.png", Vote: 9, Language: s("en")},
		{FilePath: "/fr-low.png", Vote: 1, Language: s("fr")},
		{FilePath: "/fr-best.png", Vote: 5, Language: s("fr")},
		{FilePath: "/fr.svg", Vote: 9, Language: s("fr")},
		{FilePath: "/none.png", Vote: 9},
	}, "https://img", "")
	if len(got) != 1 || got["fr"] != "https://img/w500/fr-best.png" {
		t.Fatalf("got %v", got)
	}
	if otherLogos(nil, "x", "") != nil {
		t.Fatal("no logos should give nil")
	}
}

func TestOtherLogosRegionAndOriginal(t *testing.T) {
	s := func(v string) *string { return &v }
	got := otherLogos([]tmdbLogo{
		{FilePath: "/pt-pt.png", Vote: 9, Language: s("pt"), Country: s("PT")},
		{FilePath: "/pt-br.png", Vote: 2, Language: s("pt"), Country: s("BR")},
		{FilePath: "/zh-cn.png", Vote: 9, Language: s("zh"), Country: s("CN")},
		{FilePath: "/zh-tw.png", Vote: 1, Language: s("zh"), Country: s("TW")},
		{FilePath: "/ja.png", Vote: 5, Language: s("ja")},
		{FilePath: "/nb.png", Vote: 5, Language: s("nb")},
		{FilePath: "/tl.png", Vote: 5, Language: s("tl")},
	}, "i", "ja")
	want := map[string]string{"pt-br": "i/w500/pt-br.png", "pt-pt": "i/w500/pt-pt.png", "zh-cn": "i/w500/zh-cn.png",
		"zh-tw": "i/w500/zh-tw.png", "ja": "i/w500/ja.png", "nb": "i/w500/nb.png", "fil": "i/w500/tl.png", OrigLogo: "i/w500/ja.png"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: got %q want %q", k, got[k], v)
		}
	}
	// Only another region's logo: still shown over English.
	got = otherLogos([]tmdbLogo{{FilePath: "/zh-cn.png", Language: s("zh"), Country: s("CN")}}, "i", "en")
	if got["zh-tw"] != "i/w500/zh-cn.png" || got[OrigLogo] != "" {
		t.Fatalf("got %v", got)
	}
}

func TestPickCardArtRegion(t *testing.T) {
	s := func(v string) *string { return &v }
	im := CardImages{
		Backdrops: []CardImage{{Path: "bd", Vote: 1}},
		Logos: []CardImage{
			{Path: "en.png", Lang: s("en"), Vote: 9},
			{Path: "br.png", Lang: s("pt"), Country: s("BR"), Vote: 1},
			{Path: "pt.png", Lang: s("pt"), Country: s("PT"), Vote: 8},
		},
	}
	for lang, want := range map[string]string{"pt-br": "br.png", "pt-pt": "pt.png", "de": "en.png", "": "en.png"} {
		if got := PickCardArt(im, lang, ""); got.Logo != want || got.Art != "bd" {
			t.Errorf("%q: got %+v", lang, got)
		}
	}
}
