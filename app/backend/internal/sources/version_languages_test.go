package sources

import (
	"context"
	"slices"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

func TestJudgeAll_VersionAudioLanguages(t *testing.T) {
	f := seed(t, "http://127.0.0.1:1/api")
	sr := &Searcher{Store: f.s}
	c, err := sr.loadSlot(context.Background(), f.slotID)
	if err != nil {
		t.Fatal(err)
	}
	c.ti.OriginalLanguage = ptr("en")
	names := []string{
		"Dune.Part.Two.2024.FRENCH.1080p.WEB.H264-GROUP",
		"Dune.Part.Two.2024.GERMAN.1080p.WEB.H264-GROUP",
		"Dune.Part.Two.2024.1080p.WEB.H264-GROUP",
		"Dune.Part.Two.2024.MULTI.1080p.WEB.H264-GROUP",
		"Dune.Part.Two.2024.VOSTFR.1080p.WEB.H264-GROUP",
	}
	all := make([]found, len(names))
	for i, n := range names {
		all[i] = found{src: domain.Source{ID: f.srcID}, item: Item{Title: n, Protocol: "torrent"}}
	}

	// The original Version keeps today's behaviour.
	shared := c.pr.Body.Language.Wanted
	_, vs, err := sr.judgeAll(context.Background(), c, all, true, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if vs[0].Passed || vs[1].Passed || !vs[2].Passed || !vs[3].Passed || !vs[4].Passed {
		t.Fatalf("original Version: %v %v %v %v %v", vs[0].Passed, vs[1].Passed, vs[2].Passed, vs[3].Passed, vs[4].Passed)
	}

	// A French Version takes FRENCH, refuses GERMAN and the untagged name.
	c.ver.AudioLanguages = []string{"fr"}
	_, vs, err = sr.judgeAll(context.Background(), c, all, true, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if !vs[0].Passed {
		t.Fatalf("FRENCH refused for a French Version: %+v", vs[0].Reasons)
	}
	for _, i := range []int{1, 2, 4} {
		if vs[i].Passed || !verdictHas(vs[i], "French audio") {
			t.Errorf("%s: passed=%v reasons %+v", names[i], vs[i].Passed, vs[i].Reasons)
		}
	}
	if !vs[3].Passed || !verdictHas(vs[3], "MULTI") {
		t.Errorf("MULTI is unclear and must only warn: %+v", vs[3])
	}
	if !slices.Equal(c.pr.Body.Language.Wanted, shared) || len(c.pr.Body.Language.Wanted) != 0 {
		t.Fatalf("shared Pick rules body changed: %v", c.pr.Body.Language.Wanted)
	}

	// Asking for the original language: the untagged name is the original.
	c.ver.AudioLanguages = []string{"en"}
	_, vs, err = sr.judgeAll(context.Background(), c, all[2:3], true, true, false)
	if err != nil || !vs[0].Passed {
		t.Fatalf("untagged name for an English Version: %+v %v", vs, err)
	}
}

func TestWithVersionAudio_LeavesSharedBody(t *testing.T) {
	backing := make([]string, 1, 4)
	backing[0] = "en"
	body := mediafiles.PickRulesBody{Language: mediafiles.LanguageRule{Wanted: backing}}
	got := withVersionAudio(body, []string{"fr", "en"})
	if !slices.Equal(got.Language.Wanted, []string{"en", "fr"}) {
		t.Fatalf("joined: %v", got.Language.Wanted)
	}
	if len(body.Language.Wanted) != 1 || backing[:2][1] != "" {
		t.Fatalf("shared slice written: %v %v", body.Language.Wanted, backing[:2])
	}
	if same := withVersionAudio(body, nil); !slices.Equal(same.Language.Wanted, []string{"en"}) {
		t.Fatalf("no audio languages changed the body: %v", same.Language.Wanted)
	}
}

func TestJudgeShow_VersionAudioLanguages(t *testing.T) {
	f := seedShow(t, "http://127.0.0.1:1/api", nil)
	sr := &Searcher{Store: f.s}
	ctx := context.Background()
	exec(t, f.fixture, `UPDATE versions SET audio_languages = '["fr"]' WHERE id = ?`, f.verID)
	set, err := sr.loadShow(ctx, mustTitle(t, f), mustVersion(t, f))
	if err != nil {
		t.Fatal(err)
	}
	set.target[f.slots["S01E01"]] = true
	names := []string{"Copper.Valley.S01E01.FRENCH.720p.HDTV.x264-OXBOW", "Copper.Valley.S01E01.GERMAN.720p.HDTV.x264-OXBOW", "Copper.Valley.S01E01.720p.HDTV.x264-OXBOW"}
	all := make([]found, len(names))
	for i, n := range names {
		all[i] = found{src: domain.Source{ID: f.srcID}, item: Item{Title: n, Size: 600 << 20, Protocol: "torrent", Published: time.Now().Add(-time.Hour)}}
	}
	_, vs, err := sr.judgeShow(ctx, set, all, true, true, sr.showRules(ctx))
	if err != nil {
		t.Fatal(err)
	}
	if !vs[0].Passed {
		t.Fatalf("FRENCH episode refused: %+v", vs[0].Reasons)
	}
	if vs[1].Passed || vs[2].Passed || !verdictHas(vs[1], "French audio") || !verdictHas(vs[2], "French audio") {
		t.Fatalf("German and untagged episodes: %+v / %+v", vs[1].Reasons, vs[2].Reasons)
	}
}

func TestAudioChoiceVerdict_LanguageNamesDoNotShow(t *testing.T) {
	// The name reader cannot see Romanian, so a ROMANIAN name only warns; the media inspected at import decides.
	ro := releasename.Parse("Quiet.Harbour.2019.ROMANIAN.1080p.WEB.H264-GROUP", releasename.Hints{})
	if v := audioChoiceVerdict([]string{"ro"}, "en", ro, mediafiles.Verdict{Passed: true}); !v.Passed || !verdictHas(v, "inspected media decides") {
		t.Fatalf("Romanian Version: %+v", v)
	}
	// A language the reader knows is still refused when clearly missing, whatever order the Version lists it in.
	de := releasename.Parse("Quiet.Harbour.2019.GERMAN.1080p.WEB.H264-GROUP", releasename.Hints{})
	if v := audioChoiceVerdict([]string{"ro", "fr"}, "en", de, mediafiles.Verdict{Passed: true}); v.Passed || !verdictHas(v, "French audio") {
		t.Fatalf("Romanian and French Version: %+v", v)
	}
	// An untagged dotted WEB.DL name is the original language, not a dual-language one.
	web := releasename.Parse("Quiet.Harbour.2019.1080p.WEB.DL.DD5.1.H264-GROUP", releasename.Hints{})
	if v := audioChoiceVerdict([]string{"fr"}, "en", web, mediafiles.Verdict{Passed: true}); v.Passed || !verdictHas(v, "French audio") {
		t.Fatalf("WEB.DL name for a French Version: %+v", v)
	}
}
