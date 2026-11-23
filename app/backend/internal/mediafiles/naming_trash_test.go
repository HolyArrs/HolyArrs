package mediafiles

import "testing"

// The facts every TRaSH pattern is tried with: one release that says
// everything, and one that says almost nothing.
var (
	trashFull = NameFacts{Title: "The Matrix: Reloaded", Year: 2003, TMDB: "604", IMDB: "tt0234215", TVDB: "81189", Step: "2160p_bluray", Edition: "Directors Cut",
		Audio: "TrueHD", Channels: "7.1", HDR: "DolbyVisionHDR10", VideoCodec: "x265", BitDepth: "10", Group: "EVOLVE", Proper: "Proper",
		Languages: []string{"ja", "en"}, Release: "The.Matrix.Reloaded.2003.2160p.BluRay-EVOLVE", Ext: ".mkv"}
	trashBare = NameFacts{Title: "The Matrix: Reloaded", Year: 2003, TMDB: "604", Step: "1080p_web", Ext: ".mkv"}
)

func trashShowSettings(k string) NamingSettings {
	s := NamingDefaults()
	s.EpisodePattern, s.DailyPattern, s.AnimePattern, s.SeasonFolderPattern = trashEpisodes["standard/default"], trashEpisodes["daily/default"], trashEpisodes["anime/default"], trashSeasonFolder
	switch k[:5] {
	case "stand":
		s.EpisodePattern = trashEpisodes[k]
	case "daily":
		s.DailyPattern = trashEpisodes[k]
	case "anime":
		s.AnimePattern = trashEpisodes[k]
	}
	return s
}

func trashEpisode(k string, f NameFacts, eps []int) EpisodeFacts {
	e := EpisodeFacts{NameFacts: f, Order: OrderAired, Season: 1, Episodes: eps, Titles: []string{"The Mill", "Flood"}[:len(eps)]}
	switch k[:5] {
	case "daily":
		e.Order, e.AirDate, e.Season = OrderDaily, "2026-03-14", 2026
	case "anime":
		e.Order = OrderAbsolute
		for _, n := range eps {
			e.Absolute = append(e.Absolute, n+10)
		}
	}
	return e
}

// Every TRaSH naming pattern for Radarr and Sonarr (Plex, Jellyfin, Emby,
// anime, scene and original) saves and names files as Radarr and Sonarr do,
// with and without the facts; a word with no value leaves nothing behind.
func TestTrashPatterns(t *testing.T) {
	check := func(kind, k, pat string, set func(*NamingSettings), got func(NamingSettings) (string, string), full, bare string) {
		t.Helper()
		if pat == "" {
			t.Fatalf("%s %s: no such TRaSH pattern", kind, k)
		}
		s := NamingDefaults()
		set(&s)
		if err := s.Validate(); err != nil {
			t.Errorf("%s %s: %v", kind, k, err)
			return
		}
		gf, gb := got(s)
		if gf != full {
			t.Errorf("%s %s:\n got  %q\n want %q", kind, k, gf, full)
		}
		if gb != bare {
			t.Errorf("%s %s (bare):\n got  %q\n want %q", kind, k, gb, bare)
		}
	}
	for _, c := range trashMovieFileWant {
		pat := trashMovieFiles[c[0]]
		check("movie file", c[0], pat, func(s *NamingSettings) { s.FilePattern = pat },
			func(s NamingSettings) (string, string) { return FileName(s, trashFull), FileName(s, trashBare) }, c[1], c[2])
	}
	for _, c := range trashMovieFolderWant {
		pat := trashMovieFolders[c[0]]
		check("movie folder", c[0], pat, func(s *NamingSettings) { s.FolderPattern = pat },
			func(s NamingSettings) (string, string) { return Folder(s, trashFull), Folder(s, trashBare) }, c[1], c[2])
	}
	for _, c := range trashShowFolderWant {
		pat := trashShowFolders[c[0]]
		check("show folder", c[0], pat, func(s *NamingSettings) { s.ShowFolderPattern = pat },
			func(s NamingSettings) (string, string) { return ShowFolder(s, trashFull), ShowFolder(s, trashBare) }, c[1], c[2])
	}
	for _, c := range trashEpisodeWant {
		k := c[0]
		check("episode", k, trashEpisodes[k], func(s *NamingSettings) { *s = trashShowSettings(k) }, func(s NamingSettings) (string, string) {
			return EpisodePath(s, "X", trashEpisode(k, trashFull, []int{2, 3})), EpisodePath(s, "X", trashEpisode(k, trashBare, []int{2}))
		}, c[1], c[2])
	}
	if len(trashMovieFileWant) != len(trashMovieFiles) || len(trashMovieFolderWant) != len(trashMovieFolders) ||
		len(trashShowFolderWant) != len(trashShowFolders) || len(trashEpisodeWant) != len(trashEpisodes) {
		t.Fatal("a TRaSH pattern has no expected name")
	}
}
