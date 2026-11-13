package library

import (
	"strings"
	"testing"
)

// TestConvertFormat: an import takes language, release type, edition and
// year tests by meaning; TRaSH sync keeps skipping them; a kind that
// cannot be taken skips the whole format.
func TestConvertFormat(t *testing.T) {
	more := ConvertOptions{MoreKinds: true}
	cases := []struct {
		app   string
		in    FormatSpec
		opt   ConvertOptions
		kind  string
		value string
		skip  string
	}{
		{"sonarr", FormatSpec{Implementation: "LanguageSpecification", Language: "de"}, more, "language", "de", ""},
		{"sonarr", FormatSpec{Implementation: "LanguageSpecification", Language: "en", Except: true}, more, "language", "!en", ""},
		{"radarr", FormatSpec{Implementation: "LanguageSpecification", Language: "any", Except: true}, more, "", "", "language"},
		{"sonarr", FormatSpec{Implementation: "LanguageSpecification", Language: ""}, more, "", "", "language"},
		{"sonarr", FormatSpec{Implementation: "LanguageSpecification", Language: "de"}, ConvertOptions{}, "", "", "language"},
		{"sonarr", FormatSpec{Implementation: "ReleaseTypeSpecification", Value: "3"}, more, "release_type", "season_pack", ""},
		{"radarr", FormatSpec{Implementation: "ReleaseTypeSpecification", Value: "3"}, more, "", "", "release type"},
		{"radarr", FormatSpec{Implementation: "EditionSpecification", Value: `\bIMAX\b`}, more, "edition", `\bIMAX\b`, ""},
		{"radarr", FormatSpec{Implementation: "YearSpecification", Min: 1990, Max: 1999}, more, "year", "1990-1999", ""},
		{"radarr", FormatSpec{Implementation: "YearSpecification", Min: 2000, Max: 1999}, more, "", "", "year"},
		{"radarr", FormatSpec{Implementation: "QualityModifierSpecification", Value: "5"}, more, "modifier", "remux", ""},
		{"radarr", FormatSpec{Implementation: "QualityModifierSpecification", Value: "3"}, more, "", "", "quality modifier"},
		{"sonarr", FormatSpec{Implementation: "IndexerFlagSpecification", Value: "1"}, more, "", "", "indexer flag"},
		{"sonarr", FormatSpec{Implementation: "ReleaseTitleSpecification", Value: strings.Repeat("a", 1001)}, more, "", "", "unreadable pattern"},
	}
	for _, c := range cases {
		specs, skipped := ConvertFormat(c.app, []FormatSpec{c.in}, c.opt)
		if c.skip != "" {
			if len(skipped) != 1 || skipped[0] != c.skip || len(specs) != 0 {
				t.Errorf("%s %+v: specs %+v skipped %v, want skipped %s", c.app, c.in, specs, skipped, c.skip)
			}
			continue
		}
		if len(skipped) != 0 || len(specs) != 1 || specs[0].Kind != c.kind || specs[0].Value != c.value {
			t.Errorf("%s %+v: specs %+v skipped %v, want %s=%s", c.app, c.in, specs, skipped, c.kind, c.value)
		}
	}
	if _, skipped := ConvertFormat("sonarr", make([]FormatSpec, 101), more); len(skipped) != 1 {
		t.Errorf("101 tests: skipped %v", skipped)
	}
}
