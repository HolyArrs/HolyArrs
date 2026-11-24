package mediafiles

import (
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// Words written the old apps' way: any case and spacing, text kept only
// with a value, :00 padding, :N cutting, lower and upper case, dots for spaces.
func TestOldAppWords(t *testing.T) {
	f := NameFacts{Title: "Dune: Part Two", Year: 2024, TMDB: "693134", IMDB: "tt15239678", Step: "1080p_web", Group: "FLUX", Audio: "EAC3", Channels: "5.1",
		VideoCodec: "h264", WebRip: true, Edition: "IMAX", Ext: ".mkv"}
	for _, c := range []struct{ pat, want string }{
		{"{Title} ({Year})", "Dune - Part Two (2024).mkv"},
		{"{TITLE} {YEAR}", "DUNE - PART TWO 2024.mkv"},
		{"{movie title} {release year}", "dune - part two 2024.mkv"},
		{"{Movie CleanTitle} {(Release Year)} {[Quality Title]}{-Release Group}", "Dune Part Two (2024) [WEBRip-1080p]-FLUX.mkv"},
		{"{Movie.CleanTitle}{.Release.Year}{.Edition.Tags}{.Mediainfo.AudioCodec}{.Mediainfo.AudioChannels}", "Dune.Part.Two.2024.IMAX.EAC3.5.1.mkv"},
		{"{Movie CleanTitleYear} {Movie TitleYear}", "Dune Part Two 2024 Dune - Part Two (2024).mkv"},
		{"{Movie Title:4} {year}", "Dune 2024.mkv"},
		{"{title} {[Quality Proper]}{(MediaInfo 3D)}{ Mediainfo VideoBitDepth}", "Dune - Part Two.mkv"},
		{"{title} {tmdbid-{TmdbId}} [imdbid-{ImdbId}] {Release_Group}", "Dune - Part Two {tmdbid-693134} [imdbid-tt15239678] FLUX.mkv"},
		{"{Movie TitleThe} {Movie TitleFirstCharacter} {MediaInfo VideoCodec}", "Dune - Part Two D h264.mkv"},
	} {
		s := NamingDefaults()
		s.FilePattern = c.pat
		if err := s.Validate(); err != nil {
			t.Errorf("%s: %v", c.pat, err)
			continue
		}
		if got := FileName(s, f); got != c.want {
			t.Errorf("%s:\n got  %q\n want %q", c.pat, got, c.want)
		}
	}
}

// Unknown words and formats are refused with the word as written.
func TestOldAppWordsRefused(t *testing.T) {
	for _, c := range []struct{ pat, want string }{
		{"{title} {Movie Collection}", "unknown word {Movie Collection}"},
		{"{title} {Movie Certification}", "unknown word {Movie Certification}"},
		{"{title} {Release Year:abc}", "use :00 to pad"},
		{"{title} {[Nope]}", "unknown word {[Nope]}"},
	} {
		s := NamingDefaults()
		s.FilePattern = c.pat
		err := s.Validate()
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err %v, want %q", c.pat, err, c.want)
		}
	}
}

// Sonarr's episode words: padding, the episode code with several episodes,
// air dates with the separator the word is written with, cut titles.
func TestOldAppEpisodeWords(t *testing.T) {
	f := EpisodeFacts{NameFacts: NameFacts{Title: "Copper Valley", Year: 2025, Step: "1080p_web", Ext: ".mkv"}, Order: OrderAired, Season: 1, Episodes: []int{2, 3}, Titles: []string{"A Very Long Episode Name", "Flood"}}
	for _, c := range []struct {
		pat  string
		f    EpisodeFacts
		want string
	}{
		{"{Series Title} - S{season:00}E{episode:00} - {Episode Title}", f, "Copper Valley - S01E02-E03 - A Very Long Episode Name + Flood.mkv"},
		{"{Series Title} - {season:0}x{episode:000} - {Episode Title:6}", f, "Copper Valley - 1x002-003 - A Very.mkv"},
		{"{series title} s{season:00}e{episode:00}", EpisodeFacts{NameFacts: f.NameFacts, Season: 3, Episodes: []int{7}}, "copper valley s03e07.mkv"},
		{"{Series Title} - {Air.Date} {Air Date} {Air-Date}", EpisodeFacts{NameFacts: f.NameFacts, Order: OrderDaily, Season: 2026, Episodes: []int{1}, AirDate: "2026-03-14"}, "Copper Valley - 2026.03.14 2026 03 14 2026-03-14.mkv"},
		{"{Series Title} - {absolute:0000}", EpisodeFacts{NameFacts: f.NameFacts, Order: OrderAbsolute, Season: 1, Episodes: []int{2}, Absolute: []int{12}}, "Copper Valley - 0012.mkv"},
	} {
		s := NamingDefaults()
		s.EpisodePattern, s.DailyPattern, s.AnimePattern = c.pat, c.pat, c.pat
		if c.f.Order == OrderDaily {
			s.EpisodePattern = DefaultEpisodePattern
		}
		if err := s.Validate(); err != nil && !strings.Contains(c.pat, "Air") && !strings.Contains(c.pat, "absolute") {
			t.Errorf("%s: %v", c.pat, err)
			continue
		}
		if got := EpisodeFile(s, c.f); got != c.want {
			t.Errorf("%s:\n got  %q\n want %q", c.pat, got, c.want)
		}
	}
	s := NamingDefaults()
	s.SeasonFolderPattern = "Season {season:00}"
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := SeasonFolder(s, 4); got != "Season 04" {
		t.Errorf("season folder = %q", got)
	}
}

// Lidarr's words in music patterns.
func TestOldAppMusicWords(t *testing.T) {
	s := NamingDefaults()
	s.ArtistFolderPattern = "{Artist Name}"
	s.AlbumFolderPattern = "{Album Title} ({Release Year}) {[Album Disambiguation]}"
	s.TrackPattern = "{track:00} - {Track Title} {[Quality Title]}"
	s.MultiDiscTrackPattern = "{medium:00}-{track:00} - {Track Title}"
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	f := TrackFacts{Artist: "The Band", Album: "Songs", Year: 2001, Disc: 2, Discs: 2, Track: 5, Title: "Five", Ext: ".flac"}
	if got := TrackPath(s, ArtistFolder(s, f), f); got != "The Band/Songs (2001)/02-05 - Five.flac" {
		t.Errorf("multi-disc = %q", got)
	}
	f.Discs, f.Edition = 1, "Deluxe"
	if got := TrackPath(s, ArtistFolder(s, f), f); got != "The Band/Songs (2001) [Deluxe]/05 - Five [FLAC].flac" {
		t.Errorf("track = %q", got)
	}
}

// An import reads the old apps' release facts from the release name.
func TestReleaseFacts(t *testing.T) {
	rel := "Some.Movie.2020.REPACK.2160p.UHD.BluRay.x265.10bit.HDR.TrueHD.7.1.Atmos-GROUP"
	f := FactsFrom("Some Movie", 2020, nil, releasename.Parse(rel, releasename.Hints{}), rel, ".mkv")
	if f.Proper != "Repack" || f.VideoCodec != "x265" || f.Channels != "7.1" || f.BitDepth != "10" {
		t.Fatalf("facts = %+v", f)
	}
	rel = "Some.Movie.2020.PROPER.1080p.WEBRip.H.264-GROUP"
	f = FactsFrom("Some Movie", 2020, nil, releasename.Parse(rel, releasename.Hints{}), rel, ".mkv")
	s := NamingDefaults()
	s.FilePattern = "{Movie Title} {Quality Full} {MediaInfo VideoCodec}"
	if got := FileName(s, f); got != "Some Movie WEBRip-1080p Proper h264.mkv" {
		t.Fatalf("name = %q", got)
	}
}

// The naming page's media server styles (Plex, Jellyfin, Emby) save, and
// give the TRaSH guides' names in our own words.
func TestServerStyles(t *testing.T) {
	tail := " [{quality_full}][{audio} {audio_channels}][{hdr_type}][{video_codec}]-{group}"
	for _, c := range []struct{ folder, file, show, want, wantShow string }{
		{"{title} ({year}) {tmdb-{tmdb}}", "{title} ({year}) {tmdb-{tmdb}} - {edition-{edition}}" + tail, "{title} ({year}) {tvdb-{tvdb}}",
			"The Matrix Reloaded (2003) {tmdb-604}/The Matrix Reloaded (2003) {tmdb-604} - {edition-Directors Cut} [Bluray-2160p Proper][TrueHD 7.1][DV HDR10][x265]-EVOLVE.mkv",
			"The Matrix Reloaded (2003) {tvdb-81189}"},
		{"{title} ({year}) [tmdbid-{tmdb}]", "{title} ({year}) [tmdbid-{tmdb}] - {{edition}}" + tail, "{title} ({year}) [tvdbid-{tvdb}]",
			"The Matrix Reloaded (2003) [tmdbid-604]/The Matrix Reloaded (2003) [tmdbid-604] - {Directors Cut} [Bluray-2160p Proper][TrueHD 7.1][DV HDR10][x265]-EVOLVE.mkv",
			"The Matrix Reloaded (2003) [tvdbid-81189]"},
	} {
		s := NamingDefaults()
		s.FolderPattern, s.FilePattern, s.ShowFolderPattern = c.folder, c.file, c.show
		s.EpisodePattern = "{title} ({year}) - {episode} - {episode_title}" + tail
		if err := s.Validate(); err != nil {
			t.Fatal(err)
		}
		f := trashFull
		f.Title = "The Matrix Reloaded"
		if got := Folder(s, f) + "/" + FileName(s, f); got != c.want {
			t.Errorf("got  %q\nwant %q", got, c.want)
		}
		if got := ShowFolder(s, f); got != c.wantShow {
			t.Errorf("show got %q", got)
		}
		b := trashBare
		b.Title = "The Matrix Reloaded"
		if got := FileName(s, b); !strings.HasSuffix(got, "- [WEBDL-1080p].mkv") {
			t.Errorf("bare got %q", got)
		}
	}
}
