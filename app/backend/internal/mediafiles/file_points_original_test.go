package mediafiles

import (
	"slices"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// notOriginalRules are Pick rules with the imported "Not Original" format
// (other than the original language, and not the original language) at
// -999999, beside a built-in trait.
func notOriginalRules(t *testing.T) PickRulesBody {
	t.Helper()
	SetOwnTraits([]OwnTrait{{Meta: database.Meta{ID: 1}, Name: "Not Original", Source: "import", Specs: []OwnSpec{
		{Name: "Not Original", Kind: "language", Value: "!original", Required: true},
		{Name: "Original", Kind: "language", Value: "original", Negate: true, Required: true},
	}}})
	t.Cleanup(func() { SetOwnTraits(nil) })
	rules := GoodEveryday()
	rules.Traits = []TraitPoints{{Trait: OwnTraitKey(1), Points: -999999}, {Trait: "web_dl", Points: 25}}
	return rules
}

func titleIn(lang string) Title { return Title{Kind: "show", OriginalLanguage: &lang} }

// TestOwnedFileReadsTitleOriginal: a file already in the Library is scored
// with its Title's original language, so "Not Original" fits only a file
// that lacks it, as it does for a Release in a search.
func TestOwnedFileReadsTitleOriginal(t *testing.T) {
	rules := notOriginalRules(t)
	rings := "The Lord of the Rings - The Rings of Power - S01E01 - A Shadow of the Past (1080p AMZN WEB-DL H265 SDR DDP Atmos 5.1 English - HONE)"
	diaries := "The Apothecary Diaries - S01E01 - Maomao (1080p CR WEB-DL H264 AAC 2.0 Dual Audio - GRP)"
	cases := []struct {
		what  string
		name  string
		title Title
		audio []string
		fits  bool
	}{
		{"English show, English audio", rings, titleIn("en"), []string{"en"}, false},
		{"English show, English in the name only", rings, titleIn("en"), nil, false},
		{"Japanese show, Japanese and English audio", diaries, titleIn("ja"), []string{"ja", "en"}, false},
		{"Japanese show, English audio only", diaries, titleIn("ja"), []string{"en"}, true},
	}
	for _, c := range cases {
		fp := ScoreTitleFile(c.name, 0, c.audio, c.title, rules)
		fits := slices.ContainsFunc(fp.Traits, func(ts TraitScore) bool { return ts.Trait == OwnTraitKey(1) })
		want := 25
		if c.fits {
			want -= 999999
		}
		if fits != c.fits || fp.Points != want {
			t.Errorf("%s: Not Original fits %v, %d points %+v; want fits %v, %d points", c.what, fits, fp.Points, fp.Traits, c.fits, want)
		}
		if cur := CurrentOfTitleFile(c.name, 0, time.Time{}, c.audio, c.title, rules); cur.Points != fp.Points {
			t.Errorf("%s: a search reads the file as %d points, the file view as %d", c.what, cur.Points, fp.Points)
		}
	}
}

// TestOwnedGoodEnoughReadsTitleOriginal: the good enough points of a file
// already in the Library also read its Title's original language.
func TestOwnedGoodEnoughReadsTitleOriginal(t *testing.T) {
	rules := notOriginalRules(t)
	zero := 0
	rules.GoodEnoughPoints = &zero
	name := "Sherlock.2010.S01E01.1080p.AMZN.WEB-DL.H265.SDR.DDP.5.1.English-HONE"
	if !OwnedReachesGoodEnoughFor(name, nil, titleIn("en"), rules) || !ReachesGoodEnoughFor(name, titleIn("en"), rules) {
		t.Error("an English file of an English show is below good enough")
	}
	if OwnedReachesGoodEnoughFor(name, nil, titleIn("ja"), rules) {
		t.Error("an English file of a Japanese show reaches good enough past Not Original")
	}
}
