package mediafiles

import "testing"

// A search may end in the year of what it looks for: that year is a year,
// not a word of the name.
func TestSplitYear(t *testing.T) {
	for _, c := range []struct {
		term string
		name string
		year int
		ok   bool
	}{
		{"Strike 2017", "Strike", 2017, true},
		{"The Strike 2017", "The Strike", 2017, true},
		{"Blade Runner 2049", "Blade Runner", 2049, true},
		{"Dune (2024)", "Dune", 2024, true},
		{"Dune [2024]", "Dune", 2024, true},
		{"Dune 2024 ", "Dune", 2024, true},
		{"Dune", "Dune", 0, false},           // one word is a name
		{"2012", "2012", 0, false},           // a year alone is a name
		{"Dune 3", "Dune 3", 0, false},       // a number that is no year stays a word
		{"Dune 1799", "Dune 1799", 0, false}, // an early figure stays a word
		{"Dune 2124", "Dune 2124", 0, false}, // a late figure stays a word
		{"Dune Part", "Dune Part", 0, false},
		{"", "", 0, false},
	} {
		name, year, ok := SplitYear(c.term)
		if name != c.name || year != c.year || ok != c.ok {
			t.Errorf("%q: got %q %d %v, want %q %d %v", c.term, name, year, ok, c.name, c.year, c.ok)
		}
	}
}
