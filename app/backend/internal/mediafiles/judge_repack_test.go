package mediafiles

import (
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

func TestRankRepackBeatsPlain(t *testing.T) {
	for _, sfx := range []string{"-FLUX-xpost", "-FLUX"} {
		rules := GoodEveryday()
		mk := func(name string) (ReleaseFacts, Verdict) {
			p := releasename.Parse(name, releasename.Hints{})
			set := SlotSet{Match: MatchTitle(p, showLibrary), SlotIDs: []int64{1}, Out: true, RuntimeMinutes: 45}
			rel := ReleaseFacts{Name: name, Parse: p, Size: 1200 * mb, Protocol: "torrent", PublishedAt: time.Now().Add(-time.Hour)}
			return rel, Judge(rel, set, rules)
		}
		r1, v1 := mk("Copper.Valley.S01E01.1080p.WEB.H264" + sfx)
		r2, v2 := mk("Copper.Valley.S01E01.REPACK.1080p.WEB.H264" + sfx)
		t.Logf("%s: plain fix=%d group=%q step=%d pts=%d | repack fix=%d group=%q step=%d pts=%d", sfx,
			r1.Parse.Fix, r1.Parse.Group.Value, v1.Step.Index, v1.Points, r2.Parse.Fix, r2.Parse.Group.Value, v2.Step.Index, v2.Points)
		vs := []Verdict{v1, v2}
		Rank([]ReleaseFacts{r1, r2}, vs, rules)
		if vs[1].Rank != 1 {
			t.Errorf("%s: REPACK should rank first\n%s\n%s", sfx, dump(vs[0]), dump(vs[1]))
		}
	}
}

// A third release from another group must not flip the fix-number order.
func TestRankRepackBeatsPlainWithOtherGroup(t *testing.T) {
	rules := GoodEveryday()
	mk := func(name string, seeders int) (ReleaseFacts, Verdict) {
		p := releasename.Parse(name, releasename.Hints{})
		set := SlotSet{Match: MatchTitle(p, showLibrary), SlotIDs: []int64{1}, Out: true, RuntimeMinutes: 45}
		rel := ReleaseFacts{Name: name, Parse: p, Size: 1200 * mb, Protocol: "torrent", Seeders: ptr(seeders), PublishedAt: time.Now().Add(-time.Hour)}
		return rel, Judge(rel, set, rules)
	}
	rp, vp := mk("Copper.Valley.S01E01.1080p.WEB.H264-FLUX", 300)
	ro, vo := mk("Copper.Valley.S01E01.1080p.WEB.H264-QUILL", 60)
	rr, vr := mk("Copper.Valley.S01E01.REPACK.1080p.WEB.H264-FLUX", 20)
	for _, order := range [][3]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}} {
		all := [3]ReleaseFacts{rp, ro, rr}
		allv := [3]Verdict{vp, vo, vr}
		rels := []ReleaseFacts{all[order[0]], all[order[1]], all[order[2]]}
		vs := []Verdict{allv[order[0]], allv[order[1]], allv[order[2]]}
		Rank(rels, vs, rules)
		rank := map[string]int{}
		for i := range vs {
			rank[rels[i].Name] = vs[i].Rank
		}
		if rank[rr.Name] > rank[rp.Name] {
			t.Errorf("order %v: REPACK rank %d below plain rank %d", order, rank[rr.Name], rank[rp.Name])
		}
	}
}
