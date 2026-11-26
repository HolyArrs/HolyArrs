package sources

import (
	"errors"
	"slices"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A Source's no-language list is empty by default, cleaned when sent and kept when not sent.
func TestSourceNoLanguageInput(t *testing.T) {
	var src domain.Source
	if _, _, err := fill(Input{Name: "n", Format: "torznab", URL: "https://t.example/api"}, &src, true); err != nil {
		t.Fatal(err)
	}
	if src.NoLanguageLanguages == nil || len(src.NoLanguageLanguages) != 0 {
		t.Fatalf("new Source list = %#v, want empty", src.NoLanguageLanguages)
	}
	if _, _, err := fill(Input{Name: "n", Format: "torznab", URL: "https://t.example/api", NoLanguageLanguages: []string{" UK", "ukr", "ITA", ""}}, &src, false); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(src.NoLanguageLanguages, []string{"uk", "it"}) {
		t.Fatalf("list = %v, want [uk it]", src.NoLanguageLanguages)
	}
	// A typed name or unknown code is refused, and the saved list stays.
	for _, bad := range []string{"zz", "Ukrainian", "xyz"} {
		if _, _, err := fill(Input{Name: "n", Format: "torznab", URL: "https://t.example/api", NoLanguageLanguages: []string{"uk", bad}}, &src, false); !errors.Is(err, ErrLanguages) {
			t.Errorf("%q: err = %v, want ErrLanguages", bad, err)
		}
	}
	src.NoLanguageLanguages = []string{"uk"}
	if _, _, err := fill(Input{Name: "n", Format: "torznab", URL: "https://t.example/api"}, &src, false); err != nil || !slices.Equal(src.NoLanguageLanguages, []string{"uk"}) {
		t.Fatalf("unsent list changed: %v %v", src.NoLanguageLanguages, err)
	}
	if v := SourceViewOf(domain.Source{}, Summary{}); v.NoLanguageLanguages == nil {
		t.Fatal("view list is nil, want empty")
	}
}

// The reporter's two trackers: an untagged utp.to name passes a Ukrainian
// copy only when the Source says its releases carry Ukrainian; a toloka.to
// English-only name stays refused either way.
func TestNoLanguageCopyChoice(t *testing.T) {
	need := []string{"uk"}
	for _, c := range []struct {
		name string
		none []string
		pass bool
	}{
		{"The Odyssey 2026 WEBRip 1080p-SPiLNO", []string{"uk"}, true},
		{"The Odyssey 2026 WEBRip 1080p-SPiLNO", nil, false},
		{"The Odyssey 2026 WEB-DL 1080p Eng", []string{"uk"}, false},
		{"The Odyssey 2026 WEB-DL 1080p Ukr Eng", nil, true},
	} {
		p := releasename.Parse(c.name, releasename.Hints{NoLanguage: c.none})
		v := audioChoiceVerdict(need, "en", p, mediafiles.Verdict{Passed: true, Outcome: "chosen"})
		if v.Passed != c.pass {
			t.Errorf("%s with %v: passed %v, want %v (%v)", c.name, c.none, v.Passed, c.pass, v.Reasons)
		}
	}
}
