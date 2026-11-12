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
		spec(13, OwnSpec{Name: "Title word", Kind: "edition", Value: `dune`}),
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
		{2, "Film.2020.1080p.WEB-DL-X", "en", nil, true},            // no language word: the original
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
		{13, "Dune.2021.1080p.BluRay-X", "", nil, false}, // no edition: no fit, the title is not read
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

// TestKindGroups: a kind passes when no required test in it failed and
// any test in it passed, so a passing required test carries its kind; a
// quality modifier is its own kind beside the source; "other than the
// original" fits any language while the original is not known; a year
// test falls back to the title's year.
func TestKindGroups(t *testing.T) {
	t.Cleanup(func() { SetOwnTraits(nil) })
	spec := func(id int64, s ...OwnSpec) OwnTrait {
		return OwnTrait{Meta: database.Meta{ID: id}, Name: "t", Match: "name", Source: "import", Specs: s}
	}
	SetOwnTraits([]OwnTrait{
		spec(1, OwnSpec{Name: "A", Kind: "name", Value: `\bAAA\b`}, OwnSpec{Name: "B", Kind: "name", Value: `\bBBB\b`, Required: true}, OwnSpec{Name: "4K", Kind: "resolution", Value: "2160"}),
		spec(2, OwnSpec{Name: "Blu-ray", Kind: "source", Value: "bluray,remux,disc_image"}, OwnSpec{Name: "Remux", Kind: "modifier", Value: "remux"}),
		spec(3, OwnSpec{Name: "Not original", Kind: "language", Value: "!original"}),
		spec(4, OwnSpec{Name: "90s", Kind: "year", Value: "1990-1999"}),
	})
	cases := []struct {
		id        int64
		name      string
		original  string
		titleYear int
		want      bool
	}{
		{1, "Film.2020.BBB.2160p.WEB-DL-X", "", 0, true}, // the required test passed: its kind passes
		{1, "Film.2020.AAA.BBB.2160p.WEB-DL-X", "", 0, true},
		{1, "Film.2020.AAA.2160p.WEB-DL-X", "", 0, false},
		{1, "Film.2020.BBB.1080p.WEB-DL-X", "", 0, false},
		{2, "Film.2020.1080p.BluRay.x264-X", "", 0, false},
		{2, "Film.2020.1080p.BluRay.Remux.AVC-X", "", 0, true},
		{3, "Film.2020.German.1080p.WEB-DL-X", "", 0, true},
		{3, "Film.2020.German.1080p.WEB-DL-X", "de", 0, false},
		{3, "Film.2020.1080p.WEB-DL-X", "", 0, false}, // no list never fits
		{4, "Film.1080p.WEB-DL-X", "", 1994, true},
		{4, "Film.1080p.WEB-DL-X", "", 2004, false},
		{4, "Film.2004.1080p.WEB-DL-X", "", 1994, false}, // the name's year comes first
	}
	for _, c := range cases {
		p := releasename.Parse(c.name, releasename.Hints{})
		p.OriginalLanguage, p.TitleYear = c.original, c.titleYear
		if got, why := ownMatches(c.id, p); got != c.want {
			t.Errorf("trait %d on %s (orig %q year %d) = %v (%s), want %v", c.id, c.name, c.original, c.titleYear, got, why, c.want)
		}
	}
}

// TestImportedUpgradeOrder: Pick rules brought from an old app check an
// upgrade as it did: a better quality only below the Good enough step, a
// points gain only below the Good enough points, each on its own.
func TestImportedUpgradeOrder(t *testing.T) {
	b := GoodEveryday()
	for i := range b.Steps {
		b.Steps[i].Allowed = true
	}
	b.Traits = []TraitPoints{{Trait: "hevc", Points: 100}}
	b.GoodEnoughAt, b.UpgradeMinPoints = "1080p_web", 1
	far := 10000
	b.GoodEnoughPoints = &far
	b.Synced = &SyncedProfile{From: "import", App: "radarr"}
	low := 100
	cases := []struct {
		name, cur, rel string
		goodPoints     *int
		want           bool
	}{
		{"below the step, a higher quality", "Film.1999.720p.WEB-DL.H264-X", "Film.1999.1080p.WEB-DL.H264-Y", &far, true},
		{"at the step, a higher quality is not taken", "Film.1999.1080p.WEB-DL.H264-X", "Film.1999.1080p.BluRay.Remux.AVC-Y", &far, false},
		{"at the step, more points are", "Film.1999.1080p.WEB-DL.H264-X", "Film.1999.1080p.WEB-DL.H265-Y", &far, true},
		{"below the step with the points reached, more points are not", "Film.1999.720p.WEB-DL.H265-X", "Film.1999.720p.WEB-DL.H265.Atmos-Y", &low, false},
		{"below the step with the points reached, a higher quality is", "Film.1999.720p.WEB-DL.H265-X", "Film.1999.1080p.WEB-DL.H264-Y", &low, true},
		{"a lower quality never", "Film.1999.1080p.WEB-DL.H264-X", "Film.1999.720p.WEB-DL.H265-Y", &far, false},
		{"the same points are not a gain", "Film.1999.1080p.WEB-DL.H265-X", "Film.1999.1080p.WEB-DL.H265-Y", &far, false},
	}
	for _, c := range cases {
		b.GoodEnoughPoints = c.goodPoints
		cur := CurrentOf(c.cur, 0, time.Time{}, b)
		p := releasename.Parse(c.rel, releasename.Hints{})
		if got := Gains(ReleaseFacts{Name: c.rel, Parse: p}, b, &cur); got != c.want {
			t.Errorf("%s: %v, want %v (cur %+v)", c.name, got, c.want, cur)
		}
	}
	// The same rules not from an import keep HolyArrs's own order.
	b.Synced, b.GoodEnoughPoints = nil, &far
	cur := CurrentOf("Film.1999.1080p.WEB-DL.H264-X", 0, time.Time{}, b)
	rel := "Film.1999.1080p.BluRay.Remux.AVC-Y"
	if !Gains(ReleaseFacts{Name: rel, Parse: releasename.Parse(rel, releasename.Hints{})}, b, &cur) {
		t.Error("rules made here stopped taking a higher step below Good enough points")
	}
}

// TestEveryLanguage: rules wanting every language pass a Release in a
// language that is not the original.
func TestEveryLanguage(t *testing.T) {
	b := GoodEveryday()
	var v Verdict
	p := releasename.Parse("Film.2020.German.1080p.WEB-DL-X", releasename.Hints{})
	v.language(p, b, &TitleRef{OriginalLanguage: "en"})
	if !v.failed() {
		t.Fatal("the ready-made rules took a German copy of an English film")
	}
	b.Language.Wanted = PickLanguages()
	if err := CheckPickRules("x", []string{"movie"}, b); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"Film.2020.German.1080p.WEB-DL-X", "Film.2020.Hindi.1080p.WEB-DL-X", "Film.2020.Ukrainian.1080p.WEB-DL-X"} {
		v = Verdict{}
		v.language(releasename.Parse(n, releasename.Hints{}), b, &TitleRef{OriginalLanguage: "en"})
		if v.failed() {
			t.Fatalf("every language refused %s: %+v", n, v.Reasons)
		}
	}
}

// TestCurrentFileKnowsItsTitle: the file a title holds is scored with the
// title's original language and year, as a new Release is, so the same
// name never looks like a gain over itself.
func TestCurrentFileKnowsItsTitle(t *testing.T) {
	t.Cleanup(func() { SetOwnTraits(nil) })
	SetOwnTraits([]OwnTrait{
		{Meta: database.Meta{ID: 1}, Name: "English", Match: "name", Source: "import", Specs: []OwnSpec{{Name: "en", Kind: "language", Value: "en"}}},
		{Meta: database.Meta{ID: 2}, Name: "90s", Match: "name", Source: "import", Specs: []OwnSpec{{Name: "90s", Kind: "year", Value: "1990-1999"}}},
	})
	b := GoodEveryday()
	b.Traits = []TraitPoints{{Trait: OwnTraitKey(1), Points: 50}, {Trait: OwnTraitKey(2), Points: 50}}
	b.UpgradeMinPoints = 1
	b.Synced = &SyncedProfile{From: "import", App: "radarr"}
	cur := CurrentOf("Film.1080p.WEB-DL.H264-X", 4<<30, time.Time{}, b)
	n := "Film.1080p.WEB-DL.H264-Y"
	title := &TitleRef{ID: 1, Name: "Film", Year: 1994, Runtime: 100, OriginalLanguage: "en"}
	v := Judge(ReleaseFacts{Name: n, Parse: releasename.Parse(n, releasename.Hints{}), Size: 4 << 30, Protocol: "usenet"},
		SlotSet{Match: Match{Title: title, Conf: releasename.Sure}, SlotIDs: []int64{1}, RuntimeMinutes: 100, Out: true, Current: &cur}, b)
	if v.Passed {
		t.Fatalf("the same copy passed as better: points %d, %+v", v.Points, v.Reasons)
	}
}

// TestImportedLanguageReadsTheOriginal: under rules brought from an old
// app a Release with no language word is in the title's original
// language, so wanting another language refuses it, as there.
func TestImportedLanguageReadsTheOriginal(t *testing.T) {
	b := GoodEveryday()
	b.Language = LanguageRule{Wanted: []string{"de"}}
	p := releasename.Parse("Film.2020.1080p.WEB-DL-X", releasename.Hints{})
	var v Verdict
	v.language(p, b, &TitleRef{OriginalLanguage: "en"})
	if v.failed() {
		t.Fatal("rules made here refused a Release with no language word")
	}
	b.Synced = &SyncedProfile{From: "import", App: "radarr"}
	v = Verdict{}
	v.language(p, b, &TitleRef{OriginalLanguage: "en"})
	if !v.failed() {
		t.Fatal("imported rules wanting German took an English film's plain Release")
	}
	v = Verdict{}
	v.language(p, b, &TitleRef{OriginalLanguage: "de"})
	if v.failed() {
		t.Fatal("imported rules wanting German refused a German film's plain Release")
	}
}
