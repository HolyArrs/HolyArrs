package mediafiles

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// With Pick rules from an old app wanting Ukrainian, an untagged name was
// read as the original language and refused; a Source that says its
// releases carry Ukrainian makes it pass before that fallback.
func TestNoLanguageBeforeOriginalFallback(t *testing.T) {
	rules := PickRulesBody{Language: LanguageRule{Wanted: []string{"uk"}}, Synced: &SyncedProfile{From: "import", App: "radarr"}}
	ti := &TitleRef{OriginalLanguage: "en"}
	name := "The Odyssey 2026 WEBRip 1080p-SPiLNO"
	for _, c := range []struct {
		none []string
		want string
	}{{nil, "fail"}, {[]string{"uk"}, "pass"}} {
		var v Verdict
		v.language(releasename.Parse(name, releasename.Hints{NoLanguage: c.none}), rules, ti)
		if got := v.Reasons[len(v.Reasons)-1].Effect; got != c.want {
			t.Errorf("no-language %v: %s, want %s (%v)", c.none, got, c.want, v.Reasons)
		}
	}
}
