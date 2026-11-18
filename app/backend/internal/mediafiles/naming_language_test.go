package mediafiles

import "testing"

// A naming language puts the title's translated name in new folder and
// file names; without one (or without a kept translation) the stored name stays.
func TestNamingLanguage(t *testing.T) {
	old := TitleNamer
	t.Cleanup(func() { TitleNamer = old })
	TitleNamer = func(kind, tmdbID, lang string) string {
		if lang == "de" && tmdbID == "1" {
			return map[string]string{"movie": "Das Boot", "show": "Dunkel"}[kind]
		}
		return ""
	}
	f := NameFacts{Title: "The Boat", Year: 1981, TMDB: "1", Step: "1080p_bluray", Ext: ".mkv"}
	s := NamingDefaults()
	if got := Folder(s, f); got != "The Boat (1981)" {
		t.Fatalf("default folder = %q", got)
	}
	s.TitleLanguage = "de"
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := Folder(s, f); got != "Das Boot (1981)" {
		t.Fatalf("folder = %q", got)
	}
	if got := FileName(s, f); got != "Das Boot (1981) 1080p BluRay.mkv" {
		t.Fatalf("file = %q", got)
	}
	if got := ShowFolder(s, NameFacts{Title: "Dark", Year: 2017, TMDB: "1"}); got != "Dunkel (2017)" {
		t.Fatalf("show folder = %q", got)
	}
	if got := Folder(s, NameFacts{Title: "Other", Year: 2000, TMDB: "2"}); got != "Other (2000)" {
		t.Fatalf("no translation = %q", got)
	}
	s.TitleLanguage = "xx"
	if s.Validate() == nil {
		t.Fatal("odd language accepted")
	}
}
