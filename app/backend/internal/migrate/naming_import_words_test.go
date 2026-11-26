package migrate

import (
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// Which pattern a test fills, and the episode order that names by it.
const (
	patMovie = iota
	patEpisode
	patDaily
	patAnime
)

// namesAlike checks that a translated pattern names a file as their pattern
// does under HolyArrs's own naming, which reads both: once with every fact
// known and once with most left out, so empty words leave the same name.
func namesAlike(t *testing.T, theirs, ours string, part int) {
	t.Helper()
	full := mediafiles.NameFacts{Title: "The Matrix Reloaded", Year: 2003, TMDB: "604", TVDB: "81189", IMDB: "tt0234215", Step: "1080p_bluray", Audio: "TrueHD",
		Channels: "7.1", HDR: "DolbyVisionHDR10", VideoCodec: "x265", BitDepth: "10", Group: "EVOLVE", Proper: "Proper", ThreeD: "3D",
		Languages: []string{"ja", "en"}, Ext: ".mkv"}
	sparse := mediafiles.NameFacts{Title: "The Matrix Reloaded", Year: 2003, TMDB: "604", TVDB: "81189", Step: "720p_web", Ext: ".mkv"}
	name := func(p string, f mediafiles.NameFacts) string {
		s := mediafiles.NamingDefaults()
		ep := mediafiles.EpisodeFacts{NameFacts: f, Season: 1, Episodes: []int{1}, Absolute: []int{1}, AirDate: "2026-03-14", Titles: []string{"The Mill"}}
		switch part {
		case patMovie:
			s.FilePattern = p
		case patEpisode:
			s.EpisodePattern, ep.Order = p, mediafiles.OrderAired
		case patDaily:
			s.DailyPattern, ep.Order = p, mediafiles.OrderDaily
		case patAnime:
			s.AnimePattern, ep.Order = p, mediafiles.OrderAbsolute
		}
		if err := s.Validate(); err != nil {
			t.Fatalf("%q does not save: %v", p, err)
		}
		if part == patMovie {
			return mediafiles.FileName(s, f)
		}
		return mediafiles.EpisodeFile(s, ep)
	}
	for _, f := range []mediafiles.NameFacts{full, sparse} {
		if a, b := name(theirs, f), name(ours, f); a != b {
			t.Errorf("translated pattern names files differently:\n theirs %q -> %q\n ours   %q -> %q", theirs, a, ours, b)
		}
	}
}

// {Quality Full} is the quality with Proper or Repack, as Radarr writes it,
// and {Quality Title} the same without; both have their own word.
func TestTranslatePatternQualityFull(t *testing.T) {
	for _, c := range []struct {
		in, want string
		part     int
	}{
		{"{Series Title} - S{season:00}E{episode:00} - {Episode Title} {Quality Full}", "{title} - {episode} - {episode_title} {quality_full}", patEpisode},
		{"{Movie Title} ({Release Year}) {Quality Title} {Quality Proper}", "{title} ({year}) {quality_title} {proper}", patMovie},
	} {
		got, why, ok := translatePattern(c.in, ourWords)
		if !ok || got != c.want {
			t.Errorf("%q: got %q ok=%v (%s), want %q", c.in, got, ok, why, c.want)
			continue
		}
		namesAlike(t, c.in, got, c.part)
	}
}

// Common Sonarr and Radarr patterns come across and name files the same.
func TestTranslatePatternCommonPatterns(t *testing.T) {
	for _, c := range []struct {
		in, want string
		part     int
	}{
		{"{Series Title} - S{season:00}E{episode:00} - {Episode Title} {Quality Full}", "{title} - {episode} - {episode_title} {quality_full}", patEpisode},
		{"{Series Title} - {Air-Date} - {Episode Title} {Quality Full}", "{title} - {air_date} - {episode_title} {quality_full}", patDaily},
		{"{Series Title} - S{season:00}E{episode:00} - {Episode Title} {Quality Full}", "{title} - {episode} - {episode_title} {quality_full}", patAnime},
		{"{Movie Title} ({Release Year}) {Quality Full}", "{title} ({year}) {quality_full}", patMovie},
	} {
		got, why, ok := translatePattern(c.in, ourWords)
		if !ok || got != c.want {
			t.Errorf("%q: got %q ok=%v (%s), want %q", c.in, got, ok, why, c.want)
			continue
		}
		namesAlike(t, c.in, got, c.part)
	}
}

// TRaSH-style patterns (Custom Formats left out) come across with their
// MediaInfo words, written as MediaInfo or Mediainfo.
func TestTranslatePatternTrashMediaInfo(t *testing.T) {
	for _, c := range []struct {
		in   string
		part int
	}{
		{"{Series TitleYear} - S{season:00}E{episode:00} - {Episode CleanTitle} {[Quality Full]}{[Mediainfo AudioCodec}{ Mediainfo AudioChannels]}{[MediaInfo VideoDynamicRangeType]}{[Mediainfo VideoCodec]}{-Release Group}", patEpisode},
		{"{Movie CleanTitle} {(Release Year)} {tmdb-{TmdbId}} {[Quality Full]}{[Mediainfo AudioCodec}{ Mediainfo AudioChannels]}{[MediaInfo VideoDynamicRangeType]}{[Mediainfo VideoCodec]}{-Release Group}", patMovie},
		{"{Series TitleYear} - S{season:00}E{episode:00} - {absolute:000} - {Episode CleanTitle} {[Quality Full]}{[MediaInfo VideoDynamicRangeType]}[{MediaInfo VideoBitDepth}bit]{[Mediainfo VideoCodec]}[{Mediainfo AudioCodec} { Mediainfo AudioChannels}]{MediaInfo AudioLanguages}{-Release Group}", patAnime},
	} {
		got, why, ok := translatePattern(c.in, ourWords)
		if !ok {
			t.Errorf("%q refused: %s", c.in, why)
			continue
		}
		namesAlike(t, c.in, got, c.part)
	}
}

// MediaInfo words whose value differs from every HolyArrs word stay
// unmatched, and so does 3D, whose empty value would drop a space.
func TestTranslatePatternMediaInfoWithoutMatch(t *testing.T) {
	for in, word := range map[string]string{
		"{Movie Title} {MediaInfo VideoDynamicRange}":                          "MediaInfo VideoDynamicRange",
		"{Movie Title} {MediaInfo AudioLanguagesAll}":                          "MediaInfo AudioLanguagesAll",
		"{Movie Title} {MediaInfo SubtitleLanguages}":                          "MediaInfo SubtitleLanguages",
		"{Movie Title} {MediaInfo Simple}":                                     "MediaInfo Simple",
		"{Movie Title} {MediaInfo Full}":                                       "MediaInfo Full",
		"{Movie CleanTitle} {(Release Year)} {[MediaInfo 3D]}{[Quality Full]}": "MediaInfo 3D",
	} {
		if _, why, ok := translatePattern(in, ourWords); ok || why != "uses {"+word+"}, which has no HolyArrs word" {
			t.Errorf("%q: ok=%v why=%q", in, ok, why)
		}
	}
}

// A pattern too long for HolyArrs is refused, not cut short.
func TestTranslatePatternTooLong(t *testing.T) {
	in := "{Movie Title}" + strings.Repeat(" x", mediafiles.MaxPattern/2)
	if _, why, ok := translatePattern(in, ourWords); ok || why != "it is longer than a HolyArrs pattern can be" {
		t.Errorf("ok=%v why=%q", ok, why)
	}
}
