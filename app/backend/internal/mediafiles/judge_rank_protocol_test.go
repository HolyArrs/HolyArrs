package mediafiles

import "testing"

// Two Releases on protocols that are not the preferred one tie on protocol,
// so a later step (here Source priority) decides between them, while the
// preferred protocol still wins.
func TestRankNonPreferredProtocolsTie(t *testing.T) {
	b := GoodEveryday()
	b.Protocol.Prefer = "usenet"
	step := &StepRef{Index: 0}
	verdicts := func(n int) []Verdict {
		vs := make([]Verdict, n)
		for i := range vs {
			vs[i] = Verdict{Passed: true, Step: step}
		}
		return vs
	}
	for _, rels := range [][]ReleaseFacts{
		{{Protocol: "torrent", SourcePriority: 25}, {Protocol: "soulseek", SourcePriority: 10}},
		{{Protocol: "soulseek", SourcePriority: 10}, {Protocol: "torrent", SourcePriority: 25}},
	} {
		vs := verdicts(2)
		Rank(rels, vs, b)
		for i, r := range rels {
			if r.Protocol == "soulseek" && vs[i].Rank != 1 {
				t.Errorf("non-preferred protocols did not tie: %+v", vs)
			}
		}
	}
	rels := []ReleaseFacts{{Protocol: "soulseek", SourcePriority: 1}, {Protocol: "usenet", SourcePriority: 50}, {Protocol: "torrent", SourcePriority: 1}}
	vs := verdicts(3)
	Rank(rels, vs, b)
	if vs[1].Rank != 1 {
		t.Errorf("preferred protocol lost: %+v", vs)
	}
}
