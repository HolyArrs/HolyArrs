package mediafiles

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

var dune = NameFacts{Title: "Dune: Part Two", Year: 2024, Step: "1080p_bluray", Group: "HOLYARRS", TMDB: "693134", IMDB: "tt15239678",
	Release: "Dune.Part.Two.2024.1080p.BluRay.x264-HOLYARRS", Ext: ".MKV"}

func TestFilePatterns(t *testing.T) {
	for _, c := range []struct {
		name    string
		pattern string
		colon   string
		illegal string
		f       NameFacts
		want    string
	}{
		{"default", DefaultFilePattern, "", "", dune, "Dune - Part Two (2024) 1080p BluRay.mkv"},
		{"web step", DefaultFilePattern, "", "", NameFacts{Title: "Dune: Part Two", Year: 2024, Step: "1080p_web", Ext: ".mkv"}, "Dune - Part Two (2024) 1080p WEB.mkv"},
		{"remux", DefaultFilePattern, "", "", NameFacts{Title: "Heat", Year: 1995, Step: "2160p_remux", Ext: ".mkv"}, "Heat (1995) 2160p Remux.mkv"},
		{"no step drops the word", DefaultFilePattern, "", "", NameFacts{Title: "Heat", Year: 1995, Ext: ".mkv"}, "Heat (1995).mkv"},
		{"no year drops the brackets", DefaultFilePattern, "", "", NameFacts{Title: "Heat", Step: "720p_web", Ext: ".mkv"}, "Heat 720p WEB.mkv"},
		{"ids and group", "{title} [{tmdb}] {imdb} - {group}", "", "", dune, "Dune - Part Two [693134] tt15239678 - HOLYARRS.mkv"},
		{"empty group leaves no dash", "{title} ({year}) - {group}", "", "", NameFacts{Title: "Heat", Year: 1995, Ext: ".mkv"}, "Heat (1995).mkv"},
		{"resolution and source", "{title}.{resolution}.{source}", "", "", dune, "Dune - Part Two.1080p.BluRay.mkv"},
		{"release name", "{release}", "", "", dune, "Dune.Part.Two.2024.1080p.BluRay.x264-HOLYARRS.mkv"},
		{"sort title", "{sort_title} ({year})", "", "", NameFacts{Title: "The Matrix", Year: 1999, Ext: ".mkv"}, "Matrix, The (1999).mkv"},
		{"edition", "{title} ({year}) {edition}", "", "", NameFacts{Title: "Heat", Year: 1995, Edition: "Director's Cut", Ext: ".mkv"}, "Heat (1995) Director's Cut.mkv"},
		{"colon dash", "{title}", ColonDash, "", NameFacts{Title: "Dune: Part Two", Ext: ".mkv"}, "Dune- Part Two.mkv"},
		{"colon space dash", "{title}", ColonSpaceDash, "", NameFacts{Title: "Dune: Part Two", Ext: ".mkv"}, "Dune - Part Two.mkv"},
		{"colon space dash space", "{title}", ColonSpaceDashSpace, "", NameFacts{Title: "Dune:Part Two", Ext: ".mkv"}, "Dune - Part Two.mkv"},
		{"colon remove", "{title}", ColonRemove, "", NameFacts{Title: "Dune: Part Two", Ext: ".mkv"}, "Dune Part Two.mkv"},
		{"colon smart inside a word", "{title}", ColonSmart, "", NameFacts{Title: "12:01", Ext: ".mkv"}, "12-01.mkv"},
		{"illegal replaced", "{title}", "", IllegalReplace, NameFacts{Title: `AC/DC? Live* "Now" <x>|y\z`, Ext: ".mkv"}, "AC-DC Live- 'Now' xy-z.mkv"},
		{"illegal removed", "{title}", "", IllegalRemove, NameFacts{Title: `AC/DC? Live* "Now" <x>|y\z`, Ext: ".mkv"}, "ACDC Live Now xyz.mkv"},
		{"windows device name", "{title}", "", "", NameFacts{Title: "Con", Ext: ".mkv"}, "Con_.mkv"},
		{"trailing dots", "{title}", "", "", NameFacts{Title: "Movie...", Ext: ".mkv"}, "Movie.mkv"},
		{"control characters", "{title}", "", "", NameFacts{Title: "A\x01B\tC", Ext: ".mkv"}, "AB C.mkv"},
	} {
		s := NamingDefaults()
		s.FilePattern = c.pattern
		if c.colon != "" {
			s.Colon = c.colon
		}
		if c.illegal != "" {
			s.Illegal = c.illegal
		}
		if err := s.Validate(); err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if got := FileName(s, c.f); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestFolderPattern(t *testing.T) {
	s := NamingDefaults()
	if got := Folder(s, dune); got != "Dune - Part Two (2024)" {
		t.Fatalf("folder %q", got)
	}
	s.FolderPattern = "{title} ({year}) {tmdb}"
	if got := Folder(s, dune); got != "Dune - Part Two (2024) 693134" {
		t.Fatalf("folder %q", got)
	}
	if got := Folder(NamingDefaults(), NameFacts{}); got != "Unnamed" {
		t.Fatalf("empty folder %q", got)
	}
}

func TestValidateRefusesBadPatterns(t *testing.T) {
	for _, c := range []struct {
		file, folder, colon, illegal, want string
	}{
		{"", DefaultFolderPattern, ColonSmart, IllegalReplace, "file pattern"},
		{"{nope}", DefaultFolderPattern, ColonSmart, IllegalReplace, "unknown word"},
		{"{title", DefaultFolderPattern, ColonSmart, IllegalReplace, "unclosed"},
		{"title}", DefaultFolderPattern, ColonSmart, IllegalReplace, "unopened"},
		{"{title}/{year}", DefaultFolderPattern, ColonSmart, IllegalReplace, "folder separator"},
		{"{year}", DefaultFolderPattern, ColonSmart, IllegalReplace, "{title}"},
		{DefaultFilePattern, "{year}", ColonSmart, IllegalReplace, "{title}"},
		{DefaultFilePattern, DefaultFolderPattern, "colon?", IllegalReplace, "colon"},
		{DefaultFilePattern, DefaultFolderPattern, ColonSmart, "drop", "illegal"},
		{strings.Repeat("{title}", 40), DefaultFolderPattern, ColonSmart, IllegalReplace, "too long"},
	} {
		s := NamingSettings{FilePattern: c.file, FolderPattern: c.folder, Colon: c.colon, Illegal: c.illegal, RenameOnImport: true}
		err := s.Validate()
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%+v: err %v, want %q", c, err, c.want)
		}
	}
	if err := NamingDefaults().Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestSettingsReadAndSave(t *testing.T) {
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	got, err := ReadNaming(ctx, s.Read())
	if err != nil || got != NamingDefaults() {
		t.Fatalf("defaults %+v %v", got, err)
	}
	want := NamingSettings{FilePattern: "{title} {year}", FolderPattern: "{title}", Colon: ColonRemove, Illegal: IllegalRemove, RenameOnImport: false, AdoptScanLimit: 300, MusicAdoptScanLimit: 50}
	want.fillShows() // show patterns left out keep their default
	if err := PutNamingSettings(ctx, s, want, database.System); err != nil {
		t.Fatal(err)
	}
	if got, _ = ReadNaming(ctx, s.Read()); got != want {
		t.Fatalf("saved %+v", got)
	}
	if err := PutNamingSettings(ctx, s, NamingSettings{FilePattern: "{title}", FolderPattern: "{title}", Colon: ColonRemove, Illegal: IllegalRemove, AdoptScanLimit: 5}, database.System); err == nil {
		t.Fatal("a scan limit under 10 saved")
	}
	if err := PutNamingSettings(ctx, s, NamingSettings{FilePattern: "{title}", FolderPattern: "{title}", Colon: ColonRemove, Illegal: IllegalRemove, MusicAdoptScanLimit: 5}, database.System); err == nil {
		t.Fatal("a music scan limit under 10 saved")
	}
	if err := PutNamingSettings(ctx, s, NamingSettings{FilePattern: "{x}"}, database.System); err == nil {
		t.Fatal("bad settings saved")
	}
}

// 5.1.7 important 4: each rendered name is capped at MaxComponent bytes on
// a UTF-8 boundary, keeping the extension.
func TestNamesAreCappedKeepingTheExtension(t *testing.T) {
	long := strings.Repeat("Ünïcödé Tïtlé ", 40)
	f := FileName(NamingDefaults(), NameFacts{Title: long, Year: 2024, Ext: ".MKV"})
	if len(f) > MaxComponent || !strings.HasSuffix(f, ".mkv") || !utf8.ValidString(f) {
		t.Fatalf("file %d bytes %q", len(f), f)
	}
	d := Folder(NamingDefaults(), NameFacts{Title: long, Year: 2024})
	if len(d) > MaxComponent || !utf8.ValidString(d) || d == "" {
		t.Fatalf("folder %d bytes %q", len(d), d)
	}
	if got := FileName(NamingDefaults(), NameFacts{Title: "Dune", Year: 2024, Ext: ".mkv"}); got != "Dune (2024).mkv" {
		t.Fatalf("short name changed: %q", got)
	}
}
