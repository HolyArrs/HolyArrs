package mediafiles

import "testing"

// Tags that name no single language must count as untagged, so the
// file name can supply the language.
func TestNonLanguageTagsAreUntagged(t *testing.T) {
	for _, tag := range []string{"", "und", "UND", "und-Latn", "mul", "zxx", "mis", "unk", "qaa", "qtz", "xx", "???"} {
		if got, ok := normalizeLanguage(tag); ok {
			t.Errorf("%q read as %q", tag, got)
		}
	}
	for tag, want := range map[string]string{"ita": "it", "it-IT": "it", "ger": "de", "deu": "de", "fre": "fr", "fra": "fr",
		"cze": "cs", "ces": "cs", "chi": "zh", "zho": "zh", "pt-BR": "pt", "es-419": "es", "jpn": "ja", "heb": "he", "tam": "ta", "tel": "te", "yue": "yue"} {
		if got, ok := normalizeLanguage(tag); !ok || got != want {
			t.Errorf("%q read as %q, want %q", tag, got, want)
		}
	}
}
