package subtitles

import "testing"

// Every common spelling of a kept kind reads as that kind.
func TestLang_Variants(t *testing.T) {
	for in, want := range map[string]string{
		"zh": "zh", "chi": "zh", "Chinese": "zh",
		"zh-CN": "zh-hans", "zh_CN": "zh-hans", "zh-SG": "zh-hans", "zh-Hans": "zh-hans", "zh-Hans-HK": "zh-hans", "chs": "zh-hans", "Chinese (Simplified)": "zh-hans",
		"zh-TW": "zh-hant", "zh-HK": "zh-hant", "zh-MO": "zh-hant", "zh-Hant": "zh-hant", "zh-Hant-TW": "zh-hant", "cht": "zh-hant", "big5": "zh-hant", "Traditional Chinese": "zh-hant",
		"yue": "zh-yue", "yue-HK": "zh-yue", "zh-yue": "zh-yue", "Cantonese": "zh-yue", "zh-ca": "zh-yue",
		"es-419": "es-419", "es-MX": "es-419", "es-AR": "es-419", "Latino": "es-419", "Spanish (Latin America)": "es-419", "es-ES": "es", "spa": "es",
		"fr-CA": "fr-ca", "French (Canadian)": "fr-ca", "fr-FR": "fr", "fr-BE": "fr",
		"pt-PT": "pt-pt", "pt-Latn-PT": "pt-pt", "pt-BR": "pt-br", "pt": "pt",
		"en-CA": "en", "ca": "ca", "sr-Latn": "sr", "xx-Hant": "",
	} {
		if got := NormLanguage(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

// A plain language takes every kind; a kind takes only itself.
func TestLang_VariantMatching(t *testing.T) {
	for _, c := range []struct {
		want, got string
		ok        bool
	}{
		{"zh", "zh-hans", true}, {"zh", "zh-hant", true}, {"zh", "zh-yue", true}, {"zh", "zh", true},
		{"zh-hant", "zh-hant", true}, {"zh-hant", "zh-hans", false}, {"zh-hant", "zh", false}, {"zh-hans", "zh-yue", false},
		{"es", "es-419", true}, {"es-419", "es", false}, {"pt", "pt-pt", true}, {"pt-pt", "pt-br", false}, {"fr", "fr-ca", true}, {"fr-ca", "fr", false},
	} {
		if LangMatches(c.want, c.got) != c.ok {
			t.Errorf("LangMatches(%q, %q) != %v", c.want, c.got, c.ok)
		}
	}
	// A Chinese wish is met by a Traditional file on disk; a Simplified wish is not.
	have := []Have{{Language: "zh-hant", Place: "beside"}}
	if len(Missing([]Want{{Language: "zh", HearingImpaired: HINormal}}, have)) != 0 {
		t.Error("zh wish not met by zh-hant")
	}
	if len(Missing([]Want{{Language: "zh-hans", HearingImpaired: HINormal}}, have)) != 1 {
		t.Error("zh-hans wish met by zh-hant")
	}
}

// File names carry the tag players read, and read back to the same kind.
func TestLang_VariantFileTags(t *testing.T) {
	for code, tag := range map[string]string{"zh-hans": "zh-Hans", "zh-hant": "zh-Hant", "zh-yue": "yue", "es-419": "es-419", "fr-ca": "fr-CA", "pt-pt": "pt-PT", "pt-br": "pt-BR", "zh": "zh"} {
		if got := LanguageTag(code); got != tag {
			t.Errorf("LanguageTag(%q) = %q, want %q", code, got, tag)
		}
		if h, ok := besideName("Movie (2010)", "Movie (2010)."+tag+".srt"); !ok || h.Language != code {
			t.Errorf("%s read back as %+v", tag, h)
		}
	}
	for name, want := range map[string]string{"Movie (2010).chs.srt": "zh-hans", "Movie (2010).cht.forced.srt": "zh-hant", "Movie (2010).big5.srt": "zh-hant", "Movie (2010).zh-TW.srt": "zh-hant"} {
		if h, ok := besideName("Movie (2010)", name); !ok || h.Language != want {
			t.Errorf("%s read as %+v, want %s", name, h, want)
		}
	}
	names := map[string]bool{}
	for _, l := range Languages() {
		names[l.Name] = true
	}
	for _, n := range []string{"Chinese", "Chinese (Simplified)", "Chinese (Traditional)", "Chinese (Cantonese)", "Spanish (Latin America)", "French (Canada)", "Portuguese (Portugal)", "Portuguese (Brazil)"} {
		if !names[n] {
			t.Errorf("%s missing from the picker", n)
		}
	}
}

// Imported side files are renamed with the same tags HolyArrs writes.
func TestLang_SideTag(t *testing.T) {
	for in, want := range map[string]string{"zh-tw": "zh-Hant", "chs": "zh-Hans", "pt-br": "pt-BR", "eng": "en", "yue": "yue", "nonsense": ""} {
		if got := sideTag(in); got != want {
			t.Errorf("sideTag(%q) = %q, want %q", in, got, want)
		}
	}
}
