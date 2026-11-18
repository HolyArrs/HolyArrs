package mediafiles

import "testing"

func TestTitleFirstWord(t *testing.T) {
	for _, c := range []struct{ title, want string }{
		{"Dune: Part Two", "D"},
		{"the Matrix", "T"},
		{"The Matrix", "M"},
		{"A Quiet Place", "Q"},
		{"An Officer", "O"},
		{"Élite", "E"},
		{"ødegaard", "Ø"},
		{"über", "U"},
		{"12 Monkeys", "0-9"},
		{"(500) Days of Summer", "#"},
		{"", ""},
	} {
		if got := titleFirst(c.title); got != c.want {
			t.Errorf("titleFirst(%q) = %q, want %q", c.title, got, c.want)
		}
	}
	got := Folder(NamingSettings{FolderPattern: "{title_first} - {title} ({year})"}, NameFacts{Title: "The Matrix", Year: 1999})
	if got != "M - The Matrix (1999)" {
		t.Errorf("folder = %q", got)
	}
}
