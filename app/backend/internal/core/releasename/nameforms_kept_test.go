package releasename

import (
	"fmt"
	"slices"
	"sync"
	"testing"
)

// Kept forms must be the same as freshly made ones, for any name, so keeping them is only for speed.
func TestNameFormsKeptSameAsFresh(t *testing.T) {
	names := []string{"", " ", "Streghe", "Charmed", "La Mummia: Il Ritorno", "The Mummy Returns", "Pokémon",
		"Ávila", "S.W.A.T.", "M*A*S*H", "I.T.", "E.R.", "Grey's Anatomy", "Schindler’s List", "Rambo III",
		"Rocky 2", "Die Mumie kehrt zurück", "ÜBER", "Café Society", "Ştefan cel Mare", "Città", "Ωmega",
		"Тьма", "寄生虫", "Señor Ávila", "Ocean's Eleven", "X", "2001: A Space Odyssey", "9-1-1", "Mr. & Mrs. Smith",
		"WALL·E", "Amélie", "Æon Flux", "Œdipus", "Straße", "Star Wars: Episode IV", "Spider-Man", "Dark",
		"FBI", "CSI: NY", "Love, Death & Robots", "Brooklyn Nine-Nine", "Rocky II", "Part V", "  spaced  name  "}
	for _, n := range names {
		want := nameForms(n)
		for i := 0; i < 3; i++ {
			if got := NameForms(n); !slices.Equal(got, want) {
				t.Fatalf("NameForms(%q) round %d = %q, want %q", n, i, got, want)
			}
		}
	}
	if len(names) < 40 {
		t.Fatal("keep at least 40 names")
	}
}

func TestNameFormsKeptConcurrentAndCapped(t *testing.T) {
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 30_000; i++ {
				n := fmt.Sprintf("Title %d of %d", i, g)
				if f := NameForms(n); len(f) == 0 || f[0] != CleanName(n) {
					t.Errorf("NameForms(%q) = %q", n, f)
					return
				}
			}
		}(g)
	}
	wg.Wait()
	formsMu.RLock()
	n := len(formsKept)
	formsMu.RUnlock()
	if n > 50_000 {
		t.Fatalf("kept %d names, cap is 50000", n)
	}
}
