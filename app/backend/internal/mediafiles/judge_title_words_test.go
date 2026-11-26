package mediafiles

import (
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// TestDualMultiNoLanguage: a DUAL or MULTI name with no language word may
// hold any language, so it warns that the file's audio is checked at import
// instead of assuming the original language (and never fails on it).
func TestDualMultiNoLanguage(t *testing.T) {
	b := GoodEveryday()
	b.Language = LanguageRule{Wanted: []string{"fr"}}
	title := &TitleRef{Name: "Show Name", Kind: "show", OriginalLanguage: "ja"}
	judge := func(name string) Verdict {
		p := releasename.Parse(name, releasename.Hints{Show: true})
		return Judge(ReleaseFacts{Name: name, Parse: p, Size: 2 << 30, Protocol: "torrent"},
			SlotSet{Match: Match{Title: title, Conf: releasename.Sure}, SlotIDs: []int64{1}, RuntimeMinutes: 45, Out: true}, b)
	}
	lang := func(v Verdict) (string, string) {
		for _, r := range v.Reasons {
			if r.Rule == "language" {
				return r.Effect, r.Detail
			}
		}
		return "", ""
	}
	const dualWords = "DUAL/MULTI, no language named; the file's audio is checked at import"
	// DUAL with no language word: warns with the new words, does not fail on
	// the assumed original even though only French is wanted.
	if e, d := lang(judge("Show.Name.S01E01.1080p.WEB-DL.DUAL.DDP2.0.H.264-GRP")); e != "warn" || d != dualWords {
		t.Fatalf("DUAL: effect %q detail %q", e, d)
	}
	// MULTI the same.
	if e, d := lang(judge("Show.Name.S01E01.1080p.WEB-DL.MULTI.DDP2.0.H.264-GRP")); e != "warn" || d != dualWords {
		t.Fatalf("MULTI: effect %q detail %q", e, d)
	}
	// Without DUAL or MULTI the old words stay, and the assumed original
	// still fails when only French is wanted.
	if e, d := lang(judge("Show.Name.S01E01.1080p.WEB-DL.DDP2.0.H.264-GRP")); e != "fail" || d != "no language in the name; assumed the original language, Japanese, not wanted" {
		t.Fatalf("plain: effect %q detail %q", e, d)
	}
}

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
