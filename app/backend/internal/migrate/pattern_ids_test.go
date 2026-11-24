package migrate

import "testing"

// Sonarr's Plex-style id folders come across.
func TestTranslatePatternIDBraces(t *testing.T) {
	for in, want := range map[string]string{
		"{Series Title} ({Series Year}) {tvdb-{TvdbId}}": "{title} ({year}) {tvdb-{tvdb}}",
		"{Movie Title} {tmdb-{TmdbId}}":                  "{title} {tmdb-{tmdb}}",
	} {
		got, why, ok := translatePattern(in, ourWords)
		if !ok || got != want {
			t.Errorf("%q: got %q ok=%v (%s), want %q", in, got, ok, why, want)
		}
	}
}

// Text inside a word's braces, as TRaSH's names write it, and id words in
// any case come across; a word with no match is named without its brackets.
func TestTranslatePatternWrappedWords(t *testing.T) {
	for in, want := range map[string]string{
		"{Movie CleanTitle} {(Release Year)} {imdb-{ImdbId}} {edition-{Edition Tags}} {[Quality Full]}{-Release Group}": "{title} ({year}) {imdb-{imdb}} {edition-{edition}} [{quality}]-{group}",
		"{Movie Title} ({Release Year}) {tmdb-{tmdbid}}":                                                                "{title} ({year}) {tmdb-{tmdb}}",
		"{Series TitleYear} [tvdbid-{TVDBID}]":                                                                          "{title} ({year}) [tvdbid-{tvdb}]",
		"{Series TitleYear} - S{season:00}E{episode:00} - {Episode CleanTitle}":                                         "{title} ({year}) - {episode} - {episode_title}",
	} {
		got, why, ok := translatePattern(in, ourWords)
		if !ok || got != want {
			t.Errorf("%q: got %q ok=%v (%s), want %q", in, got, ok, why, want)
		}
	}
	if _, why, ok := translatePattern("{Movie Title} {[Custom Formats]}", ourWords); ok || why != "uses {Custom Formats}, which has no HolyArrs word" {
		t.Errorf("got ok=%v why=%q", ok, why)
	}
}
