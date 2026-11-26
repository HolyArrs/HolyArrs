package mediafiles

import (
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// A release only loses to one that holds one of its episodes: in a season
// search an E08 release never "lost to" an E02 one, and the best E08 release
// stays chosen for E08.
func TestRankLosesOnlyToSameEpisode(t *testing.T) {
	rules := GoodEveryday()
	mk := func(name string, slots ...int64) (ReleaseFacts, Verdict) {
		p := releasename.Parse(name, releasename.Hints{})
		set := SlotSet{Match: MatchTitle(p, showLibrary), SlotIDs: slots, Out: true, RuntimeMinutes: 45}
		rel := ReleaseFacts{Name: name, Parse: p, Size: 1200 * mb, Protocol: "torrent", PublishedAt: time.Now().Add(-time.Hour)}
		return rel, Judge(rel, set, rules)
	}
	r2, v2 := mk("Copper.Valley.S01E02.1080p.WEB-DL.H.265-QUILL", 2)
	r8a, v8a := mk("Copper.Valley.S01E08.1080p.WEB-DL.H.264-QUILL", 8)
	r8b, v8b := mk("Copper.Valley.S01E08.720p.WEB-DL.H.264-QUILL", 8)
	for _, v := range []Verdict{v2, v8a, v8b} {
		if !v.Passed {
			t.Fatalf("all should pass:\n%s", dump(v))
		}
	}
	rels, vs := []ReleaseFacts{r2, r8a, r8b}, []Verdict{v2, v8a, v8b}
	Rank(rels, vs, rules)
	for i, v := range vs {
		for _, r := range v.Reasons {
			if r.Step == "rank" && strings.Contains(r.Detail, "S01E02") && i != 0 {
				t.Fatalf("an E08 release lost to the E02 one: %s", r.Detail)
			}
		}
	}
	if vs[1].Outcome == "passed_over" {
		t.Fatalf("the best E08 release should not be passed over:\n%s", dump(vs[1]))
	}
	if vs[2].Outcome != "passed_over" || !strings.Contains(lastRank(vs[2]), r8a.Name) {
		t.Fatalf("the second E08 release should lose to the first E08 one: %q", lastRank(vs[2]))
	}
}

func lastRank(v Verdict) string {
	s := ""
	for _, r := range v.Reasons {
		if r.Step == "rank" {
			s = r.Detail
		}
	}
	return s
}
