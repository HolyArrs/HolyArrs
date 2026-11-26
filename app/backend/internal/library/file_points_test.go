package library

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// TestScoreSlotFileReadsTitleOriginal: a Slot's file is scored with its
// Title's original language, so the imported "Not Original" format does
// not fit an English file of an English show.
func TestScoreSlotFileReadsTitleOriginal(t *testing.T) {
	mediafiles.SetOwnTraits([]mediafiles.OwnTrait{{Meta: database.Meta{ID: 1}, Name: "Not Original", Source: "import", Specs: []mediafiles.OwnSpec{
		{Name: "Not Original", Kind: "language", Value: "!original", Required: true},
		{Name: "Original", Kind: "language", Value: "original", Negate: true, Required: true},
	}}})
	t.Cleanup(func() { mediafiles.SetOwnTraits(nil) })
	rules := mediafiles.GoodEveryday()
	rules.Traits = []mediafiles.TraitPoints{{Trait: mediafiles.OwnTraitKey(1), Points: -999999}, {Trait: "web_dl", Points: 25}}
	f := mediafiles.File{RelativePath: "Season 1/The Rings of Power - S01E01 - A Shadow of the Past (1080p AMZN WEB-DL H265 DDP 5.1 English - HONE).mkv",
		MediaInfo: map[string]any{"audio_languages": []any{"en"}}}
	for lang, want := range map[string]int{"en": 25, "ja": 25 - 999999} {
		ti := mediafiles.Title{Kind: "show", OriginalLanguage: &lang}
		if got := ScoreSlotFile(f, mediafiles.Version{}, ti, rules).Points; got != want {
			t.Errorf("English file of a %s-original show: %d points, want %d", lang, got, want)
		}
	}
}
