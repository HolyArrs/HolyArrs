package releasename

import "testing"

// A library name with "&" also appears in release names with the "&" dropped
// ("Law Order"), since indexers drop it. A name that never had an "&" does
// not lose its "and".
func TestSameNameAmpersandDropped(t *testing.T) {
	for _, c := range []struct {
		lib, rel string
		want     bool
	}{
		{"New PANTY & STOCKING with GARTERBELT", "New Panty Stocking with Garterbelt", true},
		{"New PANTY & STOCKING with GARTERBELT", "New PANTY & STOCKING with GARTERBELT", true},
		{"Law & Order", "Law and Order", true},
		{"Law & Order", "Law Order", true},
		{"Law and Order", "Law Order", false},
		{"Law & Order", "Law", false},
		// "Tom Jerry" matches on purpose, since indexers drop the "&".
		{"Tom & Jerry", "Tom Jerry", true},
		// A release's "amp" may stand for "&" or be a real word, so both kinds of library name match.
		{"Law & Order", "Law amp Order", true},
		{"Guitar Amp Hero", "Guitar amp Hero", true},
		{"Tube Amp Rock", "Tube amp Rock", true},
		{"Camp & Rock", "Camp amp Rock", true},
		{"Camp Rock", "Camp Rock amp", false},
		{"Amp", "Amp", true},
	} {
		if got := SameName(c.lib, CleanName(c.rel)); got != c.want {
			t.Errorf("SameName(%q, %q) = %v, want %v", c.lib, c.rel, got, c.want)
		}
	}
}
