package subtitles

import "testing"

// Brazilian Portuguese is its own language: every common spelling reads as
// pt-br, while other regions still fold into their base language.
func TestLang_BrazilianPortugueseOwnCode(t *testing.T) {
	for in, want := range map[string]string{"pt-BR": "pt-br", "pt_br": "pt-br", "PT-br": "pt-br", "pob": "pt-br", "pb": "pt-br", "ptbr": "pt-br",
		"Portuguese (Brazil)": "pt-br", "portuguese (brazilian)": "pt-br", "Brazilian Portuguese": "pt-br", "pt-Latn-BR": "pt-br",
		"pt": "pt", "por": "pt", "pt-PT": "pt", "Portuguese": "pt", "en-US": "en", "es-419": "es", "br": "", "xx-BR": ""} {
		if got := NormLanguage(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
	if LanguageName("pt-br") != "Portuguese (Brazil)" || LanguageName("pt") != "Portuguese" {
		t.Errorf("names %q %q", LanguageName("pt-br"), LanguageName("pt"))
	}
	found := false
	for _, l := range Languages() {
		found = found || l.Code == "pt-br"
	}
	if !found {
		t.Error("pt-br missing from the picker list")
	}
}

// A wish for a base language accepts its regional files; a regional wish
// needs that region.
func TestLang_Matches(t *testing.T) {
	cases := []struct {
		want, got string
		ok        bool
	}{
		{"pt", "pt", true}, {"pt", "pt-br", true}, {"pt-br", "pt-br", true}, {"pt-br", "pt", false},
		{"en", "pt-br", false}, {"pt-br", "en", false}, {"", "pt", false},
	}
	for _, c := range cases {
		if got := LangMatches(c.want, c.got); got != c.ok {
			t.Errorf("LangMatches(%q, %q) = %v", c.want, c.got, got)
		}
	}
}

// File names carry the regional tag, which reads back as the same language.
func TestLang_FileTag(t *testing.T) {
	if LanguageTag("pt-br") != "pt-BR" || LanguageTag("en") != "en" {
		t.Fatalf("tags %q %q", LanguageTag("pt-br"), LanguageTag("en"))
	}
	if got := SubtitleName("Movie (2010)/Movie (2010).mkv", "pt-br", false, false, "hi", "srt"); got != "Movie (2010)/Movie (2010).pt-BR.srt" {
		t.Fatalf("name %q", got)
	}
	h, ok := besideName("Movie (2010)", "Movie (2010).pt-BR.hi.srt")
	if !ok || h.Language != "pt-br" || !h.HearingImpaired {
		t.Fatalf("read back %+v %v", h, ok)
	}
	if h, ok := besideName("Movie (2010)", "Movie (2010).pob.srt"); !ok || h.Language != "pt-br" {
		t.Fatalf("pob %+v %v", h, ok)
	}
}

// A pt wish is met by a pt-BR file beside the video, so nothing met today
// becomes missing; a pt-br wish is not met by a plain pt file.
func TestLang_MeetsUsesRegion(t *testing.T) {
	if len(Missing([]Want{{Language: "pt", HearingImpaired: HINormal}}, []Have{{Language: "pt-br", Place: "beside"}})) != 0 {
		t.Error("pt wish not met by a pt-BR file")
	}
	if len(Missing([]Want{{Language: "pt-br", HearingImpaired: HINormal}}, []Have{{Language: "pt", Place: "beside"}})) != 1 {
		t.Error("pt-br wish met by a plain pt file")
	}
}
