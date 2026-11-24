package mediafiles

import "testing"

// show folders take {tmdb} and {tvdb}, the Sonarr-style names
// {TmdbId} and {TvdbId}, and literal braces around an id.
func TestShowFolderIDWords(t *testing.T) {
	allen := NameFacts{Title: "Allen", Year: 2026, TMDB: "1518204", TVDB: "445566"}
	for _, c := range []struct{ pat, want string }{
		{"{title} ({year}) {tmdb}", "Allen (2026) 1518204"},
		{"{title} ({year}) [tvdb-{tvdb}]", "Allen (2026) [tvdb-445566]"},
		{"{title} ({year}) {tmdb-{tmdb}}", "Allen (2026) {tmdb-1518204}"},
		{"{title} ({year}) {tmdb-{TmdbId}}", "Allen (2026) {tmdb-1518204}"},
		{"{title} ({year}) {tvdb-{tvdbid}}", "Allen (2026) {tvdb-445566}"},
	} {
		s := NamingDefaults()
		s.ShowFolderPattern = c.pat
		if err := s.Validate(); err != nil {
			t.Fatalf("%q: %v", c.pat, err)
		}
		if got := ShowFolder(s, allen); got != c.want {
			t.Fatalf("%q gave %q, want %q", c.pat, got, c.want)
		}
	}
	s := NamingDefaults()
	s.ShowFolderPattern = "{title} ({year}) {tvdb-{tvdb}}"
	if got := ShowFolder(s, NameFacts{Title: "Allen", Year: 2026}); got != "Allen (2026)" {
		t.Fatalf("no id gave %q", got)
	}
	s.FolderPattern = "{title} ({year}) {tmdb-{TmdbId}}"
	if got := Folder(s, dune); got != "Dune - Part Two (2024) {tmdb-693134}" {
		t.Fatalf("movie folder %q", got)
	}
	for _, bad := range []string{"{title} {tmdb-{tmdb}", "{title} {tmdb-{nope}}"} {
		s := NamingDefaults()
		s.ShowFolderPattern = bad
		if s.Validate() == nil {
			t.Fatalf("%q passed", bad)
		}
	}
}
