package mediafiles

import (
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// At equal quality and points, the pack covering more wanted
// episodes wins, so a multi-season pack beats a one-season pack.
func TestRankPackCoversMoreWanted(t *testing.T) {
	rules := GoodEveryday()
	mk := func(name string, slots ...int64) (ReleaseFacts, Verdict) {
		p := releasename.Parse(name, releasename.Hints{})
		set := SlotSet{Match: MatchTitle(p, showLibrary), SlotIDs: slots, Out: true, RuntimeMinutes: 45 * len(slots)}
		rel := ReleaseFacts{Name: name, Parse: p, Size: int64(len(slots)) * 1200 * mb, Protocol: "torrent", PublishedAt: time.Now().Add(-time.Hour)}
		return rel, Judge(rel, set, rules)
	}
	r1, v1 := mk("Copper.Valley.S01.1080p.WEB-DL.H.264-QUILL", 1, 2, 3)
	r2, v2 := mk("Copper.Valley.S01-S03.1080p.WEB-DL.H.264-QUILL", 1, 2, 3, 4, 5, 6, 7, 8, 9)
	if !v1.Passed || !v2.Passed {
		t.Fatalf("both should pass:\n%s\n%s", dump(v1), dump(v2))
	}
	rels, vs := []ReleaseFacts{r1, r2}, []Verdict{v1, v2}
	Rank(rels, vs, rules)
	if vs[1].Rank != 1 {
		t.Fatalf("the multi-season pack should win:\n%s\n%s", dump(vs[0]), dump(vs[1]))
	}
	// Quality still comes first.
	r3, v3 := mk("Copper.Valley.S01.1080p.WEB-DL.H.265-QUILL", 1, 2, 3)
	_, v2 = mk("Copper.Valley.S01-S03.1080p.WEB-DL.H.264-QUILL", 1, 2, 3, 4, 5, 6, 7, 8, 9)
	rels, vs = []ReleaseFacts{r3, r2}, []Verdict{v3, v2}
	Rank(rels, vs, rules)
	if vs[0].Rank != 1 {
		t.Fatalf("quality first: %d %d", vs[0].Rank, vs[1].Rank)
	}
}
