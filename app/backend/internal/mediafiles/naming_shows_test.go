package mediafiles

import (
	"fmt"
	"path"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestEpisodeNames(t *testing.T) {
	s := NamingDefaults()
	show := NameFacts{Title: "Copper Valley", Year: 2025, Step: "1080p_web", Ext: ".MKV"}
	cases := []struct {
		name string
		f    EpisodeFacts
		want string
	}{
		{"one episode", EpisodeFacts{NameFacts: show, Order: OrderAired, Season: 1, Episodes: []int{2}, Titles: []string{"The Mill"}},
			"Copper Valley/Season 1/Copper Valley - S01E02 - The Mill 1080p WEB.mkv"},
		{"multi-episode", EpisodeFacts{NameFacts: show, Order: OrderAired, Season: 1, Episodes: []int{2, 3}, Titles: []string{"The Mill", "The Mill", "Flood"}},
			"Copper Valley/Season 1/Copper Valley - S01E02-E03 - The Mill + Flood 1080p WEB.mkv"},
		{"no episode name", EpisodeFacts{NameFacts: show, Order: OrderAired, Season: 2, Episodes: []int{10}, Titles: []string{"TBA"}},
			"Copper Valley/Season 2/Copper Valley - S02E10 - 1080p WEB.mkv"},
		{"daily", EpisodeFacts{NameFacts: show, Order: OrderDaily, Season: 2026, Episodes: []int{31}, AirDate: "2026-03-14", Titles: []string{"Guests"}},
			"Copper Valley/Season 2026/Copper Valley - 2026-03-14 - Guests 1080p WEB.mkv"},
		{"anime", EpisodeFacts{NameFacts: show, Order: OrderAbsolute, Season: 1, Episodes: []int{12, 13}, Absolute: []int{12, 13}, Titles: []string{"Rain"}},
			"Copper Valley/Season 1/Copper Valley - S01E12-E13 - 012-013 - Rain 1080p WEB.mkv"},
		{"anime without running number", EpisodeFacts{NameFacts: show, Order: OrderAbsolute, Season: 1, Episodes: []int{1}},
			"Copper Valley/Season 1/Copper Valley - S01E01 - 1080p WEB.mkv"},
		{"special", EpisodeFacts{NameFacts: show, Order: OrderAired, Season: 0, Episodes: []int{1}, Titles: []string{"Behind: the scenes"}},
			"Copper Valley/Specials/Copper Valley - S00E01 - Behind - the scenes 1080p WEB.mkv"},
	}
	for _, c := range cases {
		if got := EpisodePath(s, "Copper Valley", c.f); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
	s.SeasonFolders = false
	if got := EpisodePath(s, "Copper Valley", cases[0].f); got != "Copper Valley/Copper Valley - S01E02 - The Mill 1080p WEB.mkv" {
		t.Errorf("no season folders: %q", got)
	}
	long := cases[0].f
	long.Titles = []string{strings.Repeat("x", 400)}
	if got := EpisodeFile(NamingDefaults(), long); len(got) > MaxComponent || !strings.HasSuffix(got, ".mkv") {
		t.Errorf("long name %d bytes: %q", len(got), got)
	}
	if got := ShowFolder(NamingDefaults(), NameFacts{Title: "Copper Valley", Year: 2025}); got != "Copper Valley (2025)" {
		t.Errorf("show folder %q", got)
	}
}

func TestShowPatternsValidate(t *testing.T) {
	bad := []func(*NamingSettings){
		func(s *NamingSettings) { s.EpisodePattern = "{title} {quality}" },
		func(s *NamingSettings) { s.EpisodePattern = "{episode} {quality}" },
		func(s *NamingSettings) { s.SeasonFolderPattern = "Season" },
		func(s *NamingSettings) { s.SpecialsFolder = "{season}" },
		func(s *NamingSettings) { s.FilePattern = "{title} {episode}" },
		func(s *NamingSettings) { s.ShowFolderPattern = "{title}/{season}" },
		func(s *NamingSettings) { s.DailyPattern = "{title} {quality}" },
	}
	for i, f := range bad {
		s := NamingDefaults()
		f(&s)
		if s.Validate() == nil {
			t.Errorf("case %d passed: %+v", i, s)
		}
	}
	s := NamingDefaults()
	s.AnimePattern = "{title} - {episode}"
	if err := s.Validate(); err != nil {
		t.Errorf("anime by episode: %v", err)
	}
}

func TestEpisodeNameStyles(t *testing.T) {
	show := NameFacts{Title: "Copper Valley", Year: 2025, Step: "1080p_web", Ext: ".mkv"}
	multi := EpisodeFacts{NameFacts: show, Order: OrderAired, Season: 1, Episodes: []int{2, 3, 4}, Titles: []string{"A", "B", "C"}}
	for style, want := range map[string]string{
		MultiRange:      "Copper Valley - S01E02-E04 - A + B + C 1080p WEB.mkv",
		MultiShortRange: "Copper Valley - S01E02-04 - A + B + C 1080p WEB.mkv",
		MultiEach:       "Copper Valley - S01E02E03E04 - A + B + C 1080p WEB.mkv",
		MultiEachDash:   "Copper Valley - S01E02-E03-E04 - A + B + C 1080p WEB.mkv",
	} {
		s := NamingDefaults()
		s.MultiEpisodeStyle = style
		if got := EpisodeFile(s, multi); got != want {
			t.Errorf("%s: got %q, want %q", style, got, want)
		}
	}
	daily := EpisodeFacts{NameFacts: NameFacts{Title: "Night Talk", Step: "720p_hdtv", Ext: ".mp4"}, Order: OrderDaily, Season: 2026, Episodes: []int{31}, AirDate: "2026-03-14", Titles: []string{"Guests"}}
	for style, want := range map[string]string{
		AirDateDash:  "Night Talk - 2026-03-14 - Guests 720p HDTV.mp4",
		AirDateDot:   "Night Talk - 2026.03.14 - Guests 720p HDTV.mp4",
		AirDateSpace: "Night Talk - 2026 03 14 - Guests 720p HDTV.mp4",
	} {
		s := NamingDefaults()
		s.AirDateStyle = style
		if got := EpisodeFile(s, daily); got != want {
			t.Errorf("%s: got %q, want %q", style, got, want)
		}
	}
	anime := EpisodeFacts{NameFacts: NameFacts{Title: "Blade Song", Step: "1080p_web", Group: "SubsX", Ext: ".mkv"}, Order: OrderAbsolute, Season: 5, Episodes: []int{12}, Absolute: []int{112}, Titles: []string{"Rain"}}
	for digits, want := range map[int]string{
		2: "Blade Song - S05E12 - 112 - Rain 1080p WEB.mkv",
		3: "Blade Song - S05E12 - 112 - Rain 1080p WEB.mkv",
		4: "Blade Song - S05E12 - 0112 - Rain 1080p WEB.mkv",
	} {
		s := NamingDefaults()
		s.AbsoluteDigits = digits
		if got := EpisodeFile(s, anime); got != want {
			t.Errorf("%d digits: got %q, want %q", digits, got, want)
		}
	}
	s := NamingDefaults()
	s.AnimePattern = "[{group}] {title} - {absolute} [{quality}]"
	early := anime
	early.Absolute = []int{7}
	if got := EpisodeFile(s, early); got != "[SubsX] Blade Song - 007 [1080p WEB].mkv" {
		t.Errorf("anime pattern: %q", got)
	}
	// A daily show's special has no air date season: the episode pattern.
	sp := daily
	sp.Season, sp.Episodes = 0, []int{1}
	if got := EpisodePath(NamingDefaults(), "Night Talk", sp); got != "Night Talk/Specials/Night Talk - S00E01 - Guests 720p HDTV.mp4" {
		t.Errorf("daily special: %q", got)
	}
	// Season 00 as the specials folder, padded season folders.
	s = NamingDefaults()
	s.SpecialsFolder = "Season 00"
	s.SeasonFolderPattern = "Season {season_padded}"
	if got := SeasonFolder(s, 0); got != "Season 00" {
		t.Errorf("specials: %q", got)
	}
	if got := SeasonFolder(s, 3); got != "Season 03" {
		t.Errorf("padded: %q", got)
	}
	if err := s.Validate(); err != nil {
		t.Errorf("padded season folder: %v", err)
	}
	// Colon rules reach show and episode names.
	for colon, want := range map[string]string{
		ColonSmart:          "Star - Trail - S01E01 - Part - One 1080p WEB.mkv",
		ColonDash:           "Star- Trail - S01E01 - Part- One 1080p WEB.mkv",
		ColonSpaceDashSpace: "Star - Trail - S01E01 - Part - One 1080p WEB.mkv",
		ColonRemove:         "Star Trail - S01E01 - Part One 1080p WEB.mkv",
	} {
		s := NamingDefaults()
		s.Colon = colon
		f := EpisodeFacts{NameFacts: NameFacts{Title: "Star: Trail", Step: "1080p_web", Ext: ".mkv"}, Order: OrderAired, Season: 1, Episodes: []int{1}, Titles: []string{"Part: One"}}
		if got := EpisodeFile(s, f); got != want {
			t.Errorf("colon %s: got %q, want %q", colon, got, want)
		}
	}
	for _, f := range []func(*NamingSettings){
		func(s *NamingSettings) { s.MultiEpisodeStyle = "dup" },
		func(s *NamingSettings) { s.AirDateStyle = "slash" },
		func(s *NamingSettings) { s.AbsoluteDigits = 5 },
		func(s *NamingSettings) { s.FilePattern = "{title} {season_padded}" },
	} {
		s := NamingDefaults()
		f(&s)
		if s.Validate() == nil {
			t.Errorf("passed: %+v", s)
		}
	}
}

// 5.2 review: a file of 40 episodes in every multi-episode style stays
// within MaxComponent bytes, keeps its extension and whole UTF-8 letters,
// and with a short show name keeps its whole episode code.
func TestLongMultiEpisodeNamesAreCapped(t *testing.T) {
	eps := make([]int, 40)
	titles := make([]string, 40)
	for i := range eps {
		eps[i] = i + 1
		titles[i] = fmt.Sprintf("Ünïcödé Chapter Number %d", i+1)
	}
	for _, show := range []string{"Copper Valley", strings.Repeat("Ålpha Bravo ", 20)} {
		f := EpisodeFacts{NameFacts: NameFacts{Title: show, Year: 2025, Step: "1080p_web", Ext: ".MKV"}, Order: OrderAired, Season: 1, Episodes: eps, Titles: titles}
		for _, style := range []string{MultiRange, MultiShortRange, MultiEach, MultiEachDash} {
			s := NamingDefaults()
			s.MultiEpisodeStyle = style
			got := EpisodeFile(s, f)
			if len(got) > MaxComponent || !strings.HasSuffix(got, ".mkv") || !utf8.ValidString(got) {
				t.Errorf("%s/%s: %d bytes %q", show[:5], style, len(got), got)
			}
			if show == "Copper Valley" && !strings.Contains(got, episodeCode(1, eps, style)) {
				t.Errorf("%s: the episode code was cut: %q", style, got)
			}
			if p := EpisodePath(s, ShowFolder(s, f.NameFacts), f); len(path.Base(path.Dir(p))) > MaxComponent || len(path.Dir(path.Dir(p))) > MaxComponent {
				t.Errorf("%s: a folder is too long in %q", style, p)
			}
		}
	}
}
