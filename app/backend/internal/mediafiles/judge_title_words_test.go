package mediafiles

import (
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// TestEpisodeTitleLanguageDropped: a language word that is only part of
// the matched episode's name ("The German Job") is not read as the
// Release's language, so the name reads no language, like Sonarr.
func TestEpisodeTitleLanguageDropped(t *testing.T) {
	b := GoodEveryday()
	b.Language = LanguageRule{Wanted: []string{"de"}}
	title := &TitleRef{Name: "Show Name", Kind: "show", OriginalLanguage: "en"}
	judge := func(name string, eps ...string) Verdict {
		p := releasename.Parse(name, releasename.Hints{Show: true})
		return Judge(ReleaseFacts{Name: name, Parse: p, Size: 2 << 30, Protocol: "torrent"},
			SlotSet{Match: Match{Title: title, Conf: releasename.Sure}, SlotIDs: []int64{1}, RuntimeMinutes: 45, Out: true, EpisodeTitles: eps}, b)
	}
	// langFailed: no language was read from the name.
	langFailed := func(v Verdict) bool {
		for _, r := range v.Reasons {
			if r.Rule == "language" {
				return strings.HasPrefix(r.Detail, "no language in the name")
			}
		}
		return false
	}
	if v := judge("Show.Name.S01E01.The.German.Job.1080p.WEB.h264-GRP", "The German Job"); !langFailed(v) {
		t.Fatalf("episode title word read as German: %+v", v.Reasons)
	}
	if v := judge("Show.Name.S01E01.The.German.Job.GERMAN.1080p.WEB.h264-GRP", "The German Job"); langFailed(v) {
		t.Fatalf("a real GERMAN tag was dropped: %+v", v.Reasons)
	}
	if v := judge("Show.Name.S01E01.The.German.Job.1080p.WEB.h264-GRP"); langFailed(v) {
		t.Fatalf("with no episode title known the read stays: %+v", v.Reasons)
	}
}
