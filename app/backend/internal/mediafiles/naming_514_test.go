package mediafiles

import "testing"

// Plex-style literal braces around an id and an edition; an empty
// edition leaves nothing behind.
func TestPlexBraces(t *testing.T) {
	s := NamingDefaults()
	s.FolderPattern = "{title} ({year}) {tmdb-{tmdb}}"
	s.FilePattern = "{title} ({year}) {tmdb-{tmdb}} {edition-{edition}} [{quality}]"
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	m := NameFacts{Title: "The Matrix", Year: 1999, TMDB: "603", Step: "1080p_bluray", Edition: "Directors Cut", Ext: ".mkv"}
	for _, c := range []struct{ got, want string }{
		{Folder(s, m), "The Matrix (1999) {tmdb-603}"},
		{FileName(s, m), "The Matrix (1999) {tmdb-603} {edition-Directors Cut} [1080p BluRay].mkv"},
		{FileName(s, NameFacts{Title: "The Matrix", Year: 1999, TMDB: "603", Step: "1080p_bluray", Ext: ".mkv"}), "The Matrix (1999) {tmdb-603} [1080p BluRay].mkv"},
	} {
		if c.got != c.want {
			t.Fatalf("got %q, want %q", c.got, c.want)
		}
	}
}
