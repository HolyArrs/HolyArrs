package downloads

import "testing"

func TestLikenessAliasBeatsShortName(t *testing.T) {
	g := guessOf("Marshals.A.Yellowstone.Story.S02E06.MULTI.1080p.WEB.H264-HiggsBoson", "")
	if likeness(g.title, "Marshals") >= sureScore {
		t.Skip("the short name already matches")
	}
	if s := likeness(g.title, "Marshals - A Yellowstone Story"); s < sureScore {
		t.Fatalf("alias likeness = %d, want at least %d (title %q)", s, sureScore, g.title)
	}
}
