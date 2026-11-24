package mediafiles

import "testing"

func TestLetterFolders(t *testing.T) {
	s := NamingDefaults()
	f := NameFacts{Title: "The Matrix", Year: 1999, Genre: "Action"}
	if got := MovieFolder(s, f); got != "The Matrix (1999)" {
		t.Fatalf("off: %q", got)
	}
	s.GroupByGenre = true
	if got := MovieFolder(s, f); got != "Action/The Matrix (1999)" {
		t.Fatalf("genre: %q", got)
	}
	s.GroupByLetter = true
	if got := MovieFolder(s, f); got != "M/The Matrix (1999)" {
		t.Fatalf("letter wins: %q", got)
	}
	for title, want := range map[string]string{"Old School": "O", "Élite": "E", "30 Rock": "0-9", "¡Mucha Lucha!": "#", "An Education": "E"} {
		if got := LetterFolder(title); got != want {
			t.Errorf("LetterFolder(%q) = %q, want %q", title, got, want)
		}
	}
	sh := NameFacts{Title: "The Office", Year: 2005}
	if got := ShowFolderIn(s, sh); got != ShowFolder(s, sh) {
		t.Fatalf("show off: %q", got)
	}
	s.ShowGroupByLetter = true
	if got := ShowFolderIn(s, sh); got != "O/"+ShowFolder(s, sh) {
		t.Fatalf("show letter: %q", got)
	}
}
