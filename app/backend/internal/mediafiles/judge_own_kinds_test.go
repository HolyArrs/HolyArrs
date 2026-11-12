package mediafiles

import (
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// TestImportedTestKinds: language (with original, any and except),
// release type, edition and year tests fit as the old apps' do.
func TestImportedTestKinds(t *testing.T) {
	t.Cleanup(func() { SetOwnTraits(nil) })
	spec := func(id int64, s ...OwnSpec) OwnTrait {
		return OwnTrait{Meta: database.Meta{ID: id}, Name: "t", Match: "name", Source: "import", Specs: s}
	}
	SetOwnTraits([]OwnTrait{
		spec(1, OwnSpec{Name: "German", Kind: "language", Value: "de"}),
		spec(2, OwnSpec{Name: "Original", Kind: "language", Value: "original"}),
		spec(3, OwnSpec{Name: "Not original", Kind: "language", Value: "original", Negate: true}),
		spec(4, OwnSpec{Name: "Other than English", Kind: "language", Value: "!en"}),
		spec(5, OwnSpec{Name: "Any", Kind: "language", Value: "any"}),
		spec(6, OwnSpec{Name: "Pack", Kind: "release_type", Value: "season_pack"}),
		spec(7, OwnSpec{Name: "Multi", Kind: "release_type", Value: "multi"}),
		spec(8, OwnSpec{Name: "Single", Kind: "release_type", Value: "single"}),
		spec(9, OwnSpec{Name: "Cut", Kind: "edition", Value: `director'?s?[ ._-]?cut`}),
		spec(10, OwnSpec{Name: "90s", Kind: "year", Value: "1990-1999"}),
		spec(11),
		spec(12, OwnSpec{Name: "German", Kind: "language", Value: "de", Required: true}, OwnSpec{Name: "1080", Kind: "resolution", Value: "1080"}),
	})
	cases := []struct {
		id       int64
		name     string
		original string
		audio    []string
		want     bool
	}{
		{1, "Film.2020.German.1080p.WEB-DL-X", "", nil, true},
		{1, "Film.2020.1080p.WEB-DL-X", "en", nil, false},
		{1, "Film.2020.1080p.WEB-DL-X", "en", []string{"de"}, true}, // file audio wins
		{2, "Film.2020.1080p.WEB-DL-X", "en", nil, true},             // no language word: the original
		{2, "Film.2020.German.1080p.WEB-DL-X", "en", nil, false},
		{2, "Film.2020.1080p.WEB-DL-X", "", nil, false}, // no list at all never fits
		{3, "Film.2020.German.1080p.WEB-DL-X", "en", nil, true},
		{3, "Film.2020.German.1080p.WEB-DL-X", "", nil, true}, // original unknown: negated fits
		{3, "Film.2020.1080p.WEB-DL-X", "", nil, false},       // empty list never fits, negated or not
		{4, "Film.2020.German.1080p.WEB-DL-X", "en", nil, true},
		{4, "Film.2020.English.1080p.WEB-DL-X", "en", nil, false},
		{5, "Film.2020.1080p.WEB-DL-X", "ja", nil, true},
		{6, "Show.S01.1080p.WEB-DL-X", "", nil, true},
		{6, "Show.S01E01.1080p.WEB-DL-X", "", nil, false},
		{7, "Show.S01E01E02.1080p.WEB-DL-X", "", nil, true},
		{8, "Show.S01E01.1080p.WEB-DL-X", "", nil, true},
		{8, "Film.2020.1080p.WEB-DL-X", "", nil, false}, // a movie has no release type
		{9, "Film.2020.Directors.Cut.1080p.BluRay-X", "", nil, true},
		{9, "Film.2020.1080p.BluRay-X", "", nil, false},
		{10, "Film.1994.1080p.BluRay-X", "", nil, true},
		{10, "Film.2004.1080p.BluRay-X", "", nil, false},
		{11, "Anything.At.All", "", nil, true}, // no tests fits everything
		{12, "Film.2020.German.720p.WEB-DL-X", "", nil, false},
		{12, "Film.2020.German.1080p.WEB-DL-X", "", nil, true},
	}
	for _, c := range cases {
		p := releasename.Parse(c.name, releasename.Hints{})
		p.OriginalLanguage, p.AudioLanguages = c.original, c.audio
		if got, why := ownMatches(c.id, p); got != c.want {
			t.Errorf("trait %d on %s (orig %q audio %v) = %v (%s), want %v", c.id, c.name, c.original, c.audio, got, why, c.want)
		}
	}
}

// TestSpecLimits: long patterns and too many tests are refused, \d takes
// other scripts' digits, and a pattern out of time counts as no fit.
func TestSpecLimits(t *testing.T) {
	t.Cleanup(func() { SetOwnTraits(nil) })
	if _, err := CompileSpecPattern(strings.Repeat("a", SpecMaxPattern+1)); err == nil {
		t.Error("a 1001-character pattern was taken")
	}
	if _, err := CompileSpecPattern(strings.Repeat("a", SpecMaxPattern)); err != nil {
		t.Errorf("a 1000-character pattern was refused: %v", err)
	}
	re, _ := CompileSpecPattern(`^\d$`)
	if ok, _ := re.MatchString("٣"); !ok {
		t.Error(`\d does not take an Arabic-Indic digit`)
	}
	many := make([]OwnSpec, SpecMaxTests+1)
	for i := range many {
		many[i] = OwnSpec{Name: "n", Kind: "name", Value: "x"}
	}
	slow := strings.Repeat("a", 40) + "!"
	SetOwnTraits([]OwnTrait{
		{Meta: database.Meta{ID: 1}, Name: "many", Source: "import", Specs: many},
		{Meta: database.Meta{ID: 2}, Name: "slow", Source: "import", Specs: []OwnSpec{{Name: "evil", Kind: "name", Value: `^(a+)+$`}}},
	})
	if ok, _ := ownMatches(1, releasename.Parse("x", releasename.Hints{})); ok {
		t.Error("a trait with 101 tests matched")
	}
	start := time.Now()
	if ok, _ := ownMatches(2, releasename.Result{Original: slow}); ok {
		t.Error("a pattern out of time matched")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("the slow pattern ran %v", d)
	}
}
