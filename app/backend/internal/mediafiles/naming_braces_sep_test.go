package mediafiles

import "testing"

// An empty group between two of the same separator leaves one, so dotted
// and dashed patterns with Plex braces or brackets stay clean.
func TestEmptyGroupBetweenSeparators(t *testing.T) {
	m := NameFacts{Title: "The Matrix", Year: 1999, TMDB: "603", Ext: ".mkv"}
	for _, c := range []struct{ pat, want string }{
		{"{title}.{tvdb-{tvdb}}.{year}", "The Matrix.1999"},
		{"{title}-{tvdb-{tvdb}}-{year}", "The Matrix-1999"},
		{"{title}_[{tvdb}]_{year}", "The Matrix_1999"},
		{"{title}.{tmdb-{tmdb}}.{year}", "The Matrix.{tmdb-603}.1999"},
		{"{title} - {tvdb-{tvdb}} - {year}", "The Matrix - 1999"},
		{"{title}.{tvdb-{tvdb}}-{year}", "The Matrix.-1999"},
	} {
		s := NamingDefaults()
		s.FolderPattern = c.pat
		if err := s.Validate(); err != nil {
			t.Fatalf("%q: %v", c.pat, err)
		}
		if got := Folder(s, m); got != c.want {
			t.Errorf("%q gave %q, want %q", c.pat, got, c.want)
		}
	}
}
