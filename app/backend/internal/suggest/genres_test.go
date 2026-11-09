package suggest

import (
	"slices"
	"testing"
)

func TestShowGenreFor_KnownAndUnknown(t *testing.T) {
	if got := ShowGenreFor("Action"); got != showCombined["Action"] || got == "" {
		t.Fatalf("Action -> %q", got)
	}
	if ShowGenreFor("Nope") != "" {
		t.Fatal("unknown genre answered")
	}
	for k, v := range showCombined {
		if v == "" || !slices.Contains(movieSplit[v], k) {
			t.Errorf("showCombined[%q]=%q not mirrored in movieSplit", k, v)
		}
	}
	for v, ks := range movieSplit {
		for _, k := range ks {
			if showCombined[k] != v {
				t.Errorf("movieSplit[%q] has %q but showCombined says %q", v, k, showCombined[k])
			}
		}
	}
}
