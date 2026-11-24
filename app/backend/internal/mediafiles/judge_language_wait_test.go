package mediafiles

import (
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

func dubRules(days int) PickRulesBody {
	r := GoodEveryday()
	r.Language = LanguageRule{Wanted: []string{"de"}, OriginalOK: true, WaitDays: days}
	return r
}

func judgeDub(name string, rules PickRulesBody, out time.Time, hand bool, cur *Current) Verdict {
	title := library[0] // The Lantern Keepers, original English
	p := releasename.Parse(name, releasename.Hints{})
	return Judge(ReleaseFacts{Name: name, Parse: p, Size: 4 << 30, PublishedAt: out},
		SlotSet{Match: Match{Title: &title, Conf: releasename.Sure, Detail: "test"}, SlotIDs: []int64{1}, RuntimeMinutes: 118, Out: true,
			OutSince: &out, HandSearch: hand, Current: cur}, rules)
}

func TestLanguageWait(t *testing.T) {
	en := "The.Lantern.Keepers.2021.1080p.WEB-DL.DDP5.1.H.265-QUILL"
	de := "The.Lantern.Keepers.2021.German.DL.1080p.WEB-DL.DDP5.1.H.265-QUILL"
	bare := "The.Lantern.Keepers.2021.1080p.WEBRip.x264.AAC-TINCAN"
	now := time.Now()
	for _, c := range []struct {
		name, rel string
		rules     PickRulesBody
		out       time.Time
		hand      bool
		outcome   string
	}{
		{"original held while waiting", en, dubRules(7), now.Add(-24 * time.Hour), false, "held"},
		{"no language word held too", bare, dubRules(7), now.Add(-24 * time.Hour), false, "held"},
		{"original taken after the wait", en, dubRules(7), now.Add(-8 * 24 * time.Hour), false, "chosen"},
		{"wanted language not held", de, dubRules(7), now, false, "chosen"},
		{"person's search skips the wait", en, dubRules(7), now, true, "chosen"},
		{"no wait set", en, dubRules(0), now, false, "chosen"},
	} {
		t.Run(c.name, func(t *testing.T) {
			v := judgeDub(c.rel, c.rules, c.out, c.hand, nil)
			if v.Outcome != c.outcome {
				t.Fatalf("outcome %s, want %s: %+v", v.Outcome, c.outcome, v.Reasons)
			}
			if c.outcome == "held" && (v.HoldUntil == nil || !v.HoldUntil.Equal(c.out.Add(7*24*time.Hour))) {
				t.Fatalf("hold until %v", v.HoldUntil)
			}
		})
	}
}

func TestLanguageWaitUpgrade(t *testing.T) {
	en := "The.Lantern.Keepers.2021.1080p.WEB-DL.DDP5.1.H.265-QUILL"
	de := "The.Lantern.Keepers.2021.German.DL.1080p.WEB-DL.DDP5.1.H.265-QUILL"
	de720 := "The.Lantern.Keepers.2021.German.DL.720p.WEB-DL.DDP5.1.H.265-QUILL"
	old := time.Now().Add(-30 * 24 * time.Hour)
	curEN := CurrentOf(en, 4<<30, old, dubRules(7))
	if v := judgeDub(de, dubRules(7), old, false, &curEN); v.Outcome != "chosen" || !hasCode(v, "better.language") {
		t.Fatalf("German copy should replace the English one: %s %+v", v.Outcome, v.Reasons)
	}
	if v := judgeDub(de720, dubRules(7), old, false, &curEN); v.Outcome != "refused" {
		t.Fatalf("a lower step is not a language upgrade: %s", v.Outcome)
	}
	curDE := CurrentOf(de, 4<<30, old, dubRules(7))
	if v := judgeDub(de, dubRules(7), old, false, &curDE); v.Outcome != "refused" {
		t.Fatalf("already German: %s", v.Outcome)
	}
	if v := judgeDub(de, dubRules(0), old, false, &curEN); v.Outcome != "refused" {
		t.Fatalf("without a language wait nothing changes: %s", v.Outcome)
	}
	// Audio tracks known to hold German count as German.
	curAudio := CurrentOfFile(en, 4<<30, old, []string{"en", "de"}, dubRules(7))
	if v := judgeDub(de, dubRules(7), old, false, &curAudio); v.Outcome != "refused" {
		t.Fatalf("file already has German audio: %s", v.Outcome)
	}
}

func TestLanguageWaitCheck(t *testing.T) {
	b := dubRules(91)
	if err := CheckPickRules("x", []string{"movie"}, b); err == nil {
		t.Fatal("91 days should be refused")
	}
	b.Language.WaitDays = 90
	if err := CheckPickRules("x", []string{"movie"}, b); err != nil {
		t.Fatal(err)
	}
}

func hasCode(v Verdict, code string) bool {
	for _, r := range v.Reasons {
		if r.Code() == code {
			return true
		}
	}
	return false
}

func TestLanguageWaitRanksWantedFirst(t *testing.T) {
	en := "The.Lantern.Keepers.2021.2160p.WEB-DL.DDP5.1.H.265-QUILL"
	de := "The.Lantern.Keepers.2021.German.DL.1080p.WEB-DL.DDP5.1.H.265-QUILL"
	r := dubRules(7)
	r.Steps[stepIndex(r, "2160p_web")].Allowed = true
	out := time.Now().Add(-30 * 24 * time.Hour)
	var rels []ReleaseFacts
	var vs []Verdict
	for _, n := range []string{en, de} {
		rels = append(rels, ReleaseFacts{Name: n, Parse: releasename.Parse(n, releasename.Hints{}), Size: 4 << 30, PublishedAt: out})
		vs = append(vs, judgeDub(n, r, out, false, nil))
	}
	Rank(rels, vs, r)
	if vs[1].Rank != 1 {
		t.Fatalf("German should rank first: %+v / %+v", vs[0], vs[1])
	}
}

// A wait left behind after "the original language is fine" is turned off
// (the box is then hidden) does nothing.
func TestLanguageWaitNeedsOriginalOK(t *testing.T) {
	bare := "The.Lantern.Keepers.2021.1080p.WEBRip.x264.AAC-TINCAN"
	de := "The.Lantern.Keepers.2021.German.DL.1080p.WEB-DL.DDP5.1.H.265-QUILL"
	r := dubRules(7)
	r.Language.OriginalOK = false
	now := time.Now()
	if v := judgeDub(bare, r, now, false, nil); v.Outcome == "held" {
		t.Fatalf("held with the original language not allowed: %+v", v.Reasons)
	}
	old := now.Add(-30 * 24 * time.Hour)
	cur := CurrentOf(bare, 4<<30, old, r)
	if v := judgeDub(de, r, old, false, &cur); hasCode(v, "better.language") {
		t.Fatalf("language upgrade without the original language allowed: %+v", v.Reasons)
	}
}
