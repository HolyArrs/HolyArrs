package releasename

import (
	"slices"
	"testing"
)

func TestMoreLanguageWords(t *testing.T) {
	want := map[string]string{
		"POLISH": "pl", "DUTCH": "nl", "FLEMISH": "nl", "PORTUGUESE": "pt", "SWEDISH": "sv", "SWE": "sv",
		"DANISH": "da", "NORWEGIAN": "no", "FINNISH": "fi", "CZECH": "cs", "CZE": "cs", "HUNGARIAN": "hu",
		"GREEK": "el", "HEBREW": "he", "HEB": "he", "TURKISH": "tr", "ARABIC": "ar", "HINDI": "hi",
		"CHINESE": "zh", "MANDARIN": "zh", "THAI": "th", "VIETNAMESE": "vi", "UKRAINIAN": "uk", "UKR": "uk",
	}
	codes := map[string]bool{}
	for word, code := range want {
		codes[code] = true
		r := Parse("Quiet.Harbour.2019."+word+".1080p.WEB.H264-GROUP", Hints{})
		if !slices.Equal(r.Languages, []string{code}) || r.Title.Value != "Quiet Harbour" || r.Year.Value != 2019 {
			t.Errorf("%s: langs %v title %q year %d", word, r.Languages, r.Title.Value, r.Year.Value)
		}
		e := Parse("Quiet.Harbour.S01E02."+word+".720p.HDTV.x264-GROUP", Hints{Show: true})
		if !slices.Equal(e.Languages, []string{code}) {
			t.Errorf("episode %s: langs %v", word, e.Languages)
		}
	}
	if len(codes) != 18 {
		t.Fatalf("%d new languages, want 18", len(codes))
	}
}

func TestMoreLanguageWords_TitlesAndClashes(t *testing.T) {
	for name, title := range map[string]string{
		"My.Big.Fat.Greek.Wedding.2002.1080p.BluRay.x264-GROUP": "My Big Fat Greek Wedding",
		"The.Polish.Bride.1998.720p.WEB.H264-GROUP":             "The Polish Bride",
		"Dutch.1991.1080p.WEB.H264-GROUP":                       "Dutch",
		"Thai.Kitchen.Nights.2021.1080p.WEB.H264-GROUP":         "Thai Kitchen Nights",
	} {
		r := Parse(name, Hints{})
		if r.Title.Value != title || len(r.Languages) != 0 {
			t.Errorf("%s: title %q langs %v", name, r.Title.Value, r.Languages)
		}
	}
	// Short words that clash with names stay out, and subtitle tags are not audio.
	for _, word := range []string{"FIN", "DAN", "HUN", "TUR", "POL", "CHI", "PL", "NL", "VOSTFR", "SUBFRENCH", "NORDIC"} {
		r := Parse("Quiet.Harbour.2019."+word+".1080p.WEB.H264-GROUP", Hints{})
		if len(r.Languages) != 0 {
			t.Errorf("%s read as %v", word, r.Languages)
		}
	}
}

func TestMoreLanguageWords_BeforeAPlainWord(t *testing.T) {
	// A new language word before an ordinary word is part of an episode title or a subtitle note.
	for _, name := range []string{
		"Corner.Diner.S02E11.The.Chinese.Restaurant.1080p.WEB.H264-GROUP",
		"Corner.Diner.S01E03.Greek.Tragedy.720p.HDTV.x264-GROUP",
		"[Group] Quiet Harbour - 05 [1080p][Thai Sub]",
		"Quiet.Harbour.2019.1080p.WEB.H264.Greek.Subs-GROUP",
	} {
		if r := Parse(name, Hints{}); len(r.Languages) != 0 {
			t.Errorf("%s read as %v", name, r.Languages)
		}
	}
	for name, want := range map[string][]string{
		"Quiet.Harbour.2019.SWEDISH.DANISH.NORWEGIAN.1080p.BluRay.x264-GROUP": {"sv", "da", "no"},
		"Quiet.Harbour.2019.POLISH.DUBBED.1080p.WEB.H264-GROUP":               {"pl"},
		"Quiet.Harbour.2019.1080p.WEB.H264.CHINESE-GROUP":                     {"zh"},
		"Quiet Harbour (2019) Hindi Dub 720p WEB x264":                        {"hi"},
		"Quiet.Harbour.2019.Dual.Audio.Hindi.English.720p.WEB.x264-GROUP":     {"hi", "en"},
	} {
		if r := Parse(name, Hints{}); !slices.Equal(r.Languages, want) {
			t.Errorf("%s: langs %v, want %v", name, r.Languages, want)
		}
	}
}

func TestDualLanguage_NotWebDL(t *testing.T) {
	for name, dual := range map[string]bool{
		"Quiet.Harbour.2019.1080p.WEB.DL.DD5.1.H264-GROUP":     false,
		"Quiet Harbour 2019 1080p WEB DL x264":                 false,
		"Quiet.Harbour.2019.GERMAN.DL.1080p.BluRay.x264-GROUP": true,
		"Quiet.Harbour.2019.GERMAN.DL.1080p.WEB.DL.H264-GROUP": true,
		"Quiet.Harbour.2019.FRENCH.DUAL.1080p.WEB.H264-GROUP":  true,
	} {
		if r := Parse(name, Hints{}); r.Has("dual") != dual {
			t.Errorf("%s: dual %v, want %v", name, r.Has("dual"), dual)
		}
	}
}
