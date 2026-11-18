package importer

import "testing"

// A movie brought in from inside a genre or letter folder keeps getting its
// files in that folder; each part is cleaned on its own.
func TestMovieFolderKeepsItsPlace(t *testing.T) {
	for in, want := range map[string]string{
		"Action/Heat (1995)":     "Action/Heat (1995)",
		"A-Z/D/Dune: Part Two":   "A-Z/D/Dune - Part Two",
		"Heat (1995)":            "Heat (1995)",
		"../Heat (1995)":         "Heat (1995)",
		"Action//./Heat (1995)/": "Action/Heat (1995)",
		"":                       "",
	} {
		if got := safeFolder(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}
