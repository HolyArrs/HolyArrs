package mediafiles

import (
	"slices"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// TestFileLanguagesOrder: a File's audio tracks come first, then the
// Version's languages, then (nil) the name; a language trait scores the
// File by them.
func TestFileLanguagesOrder(t *testing.T) {
	if got := FileLanguages(map[string]any{"audio_languages": []any{"de"}}, []string{"en"}); !slices.Equal(got, []string{"de"}) {
		t.Errorf("tracks: %v", got)
	}
	if got := FileLanguages(map[string]any{"audio_languages": []any{}}, []string{"en"}); !slices.Equal(got, []string{"en"}) {
		t.Errorf("version: %v", got)
	}
	if got := FileLanguages(nil, nil); got != nil {
		t.Errorf("none: %v", got)
	}
	t.Cleanup(func() { SetOwnTraits(nil) })
	SetOwnTraits([]OwnTrait{{Meta: database.Meta{ID: 1}, Name: "German", Source: "import", Specs: []OwnSpec{{Name: "de", Kind: "language", Value: "de"}}}})
	rules := GoodEveryday()
	rules.Traits = []TraitPoints{{Trait: OwnTraitKey(1), Points: 100}}
	name := "Film.2020.English.1080p.WEB-DL.H264-X"
	if c := CurrentOfFile(name, 0, time.Time{}, []string{"de"}, rules); c.Points != 100 {
		t.Errorf("German audio file: %d points", c.Points)
	}
	if c := CurrentOfFile(name, 0, time.Time{}, nil, rules); c.Points != 0 {
		t.Errorf("no audio known: %d points", c.Points)
	}
}
