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
