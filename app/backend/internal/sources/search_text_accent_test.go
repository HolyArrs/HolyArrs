package sources

import "testing"

// An id answer named only by an accented other name is kept.
func TestNamesAllFoldsAccents(t *testing.T) {
	cases := []struct{ release, name string }{
		{"De.Battre.Mon.Coeur.S.Est.Arrete.2005.FRENCH.1080p.BluRay.x264-GRP", "De battre mon cœur s'est arrêté"},
		{"Pokemon.S01E01.1080p.WEB-DL", "Pokémon"},
		{"Pokémon.S01E01.1080p.WEB-DL", "Pokemon"},
	}
	for _, c := range cases {
		if !namesAll(c.release, nameWords(c.name)) {
			t.Errorf("%q should name %q", c.release, c.name)
		}
	}
	if namesAll("Some.Other.Movie.2005.1080p", nameWords("De battre mon cœur s'est arrêté")) {
		t.Error("an unrelated release should not name the title")
	}
}
