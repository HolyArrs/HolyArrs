// Package music holds HolyArrs's Music module: what it needs from the rest
// of the program (Deps) and, in subpackages, its pure parts (score).
package music

import (
	"context"

	"github.com/CoderLuii/holyarrs/internal/metadata"
)

// Deps are the metadata server calls and flags the Music module reads. A
// nil call leaves its rows or pictures out.
type Deps struct {
	// MusicTrending reads a music trending list from the metadata server;
	// nil hides the rows.
	MusicTrending func(ctx context.Context, kind, region string) (metadata.MusicTrend, error)
	// MusicNew reads a new music window (tracks, week, recent, upcoming)
	// from the metadata server; nil leaves New & Popular → Music to
	// MusicBrainz alone.
	MusicNew func(ctx context.Context, window string) (metadata.MusicNew, error)
	// MusicSimilar reads similar artists from the metadata server; nil has
	// no Because you have or Fans of rows.
	MusicSimilar func(ctx context.Context, mbid string) ([]metadata.SimilarArtist, error)
	// MusicCharts and MusicGenres read the metadata server's region charts
	// (with P and F) and genre tiles for the music_rows_v2 rows; nil leaves
	// those rows out.
	MusicCharts func(ctx context.Context, region, kind, genre string) (metadata.MusicChart, error)
	MusicGenres func(ctx context.Context, region string) (metadata.MusicGenres, error)
	// MusicGenreTile reads a genre tile's grey picture (Phase 2); nil
	// keeps every tile a flat hue.
	MusicGenreTile func(ctx context.Context, slug, version string) ([]byte, error)
	// MusicRowsV2 is flag music_rows_v2: Explore → Music from the
	// 2026-10-22 plan's rows; off keeps the old row builder.
	MusicRowsV2 bool
	// MusicRowsV3 is flag music_rows_v3 (Phase 3): the v2 rows with their
	// scores, plus the similarity, genre and guest rows; off keeps the
	// Phase 1 rows. It needs MusicRowsV2.
	MusicRowsV3 bool
	// MusicRowsV6 is flag music_rows_v6 (Phase 6): Fresh & Trending → Music
	// from the region charts (Spotlight, Fresh songs, Out this week, Top
	// albums, Climbing, Coming soon) and the Rising row on Explore → Music
	// (with MusicRowsV3). Off keeps both as they were.
	MusicRowsV6 bool
	// MusicListeners, MusicSimilarSet and MusicArtistData read the metadata
	// server's batch artist data for the scored rows; nil leaves P at the
	// charts and the similarity rows out.
	MusicListeners  func(ctx context.Context, ids []string) (map[string]int64, error)
	MusicSimilarSet func(ctx context.Context, seeds []string) (map[string][]metadata.MusicSimilarScore, error)
	MusicArtistData func(ctx context.Context, ids []string) ([]metadata.MusicArtistData, error)
	// MusicSuggest reads the metadata server's suggest index (charted
	// artists, albums and songs); nil leaves the dropdown to the Library
	// and TMDB.
	MusicSuggest func(ctx context.Context, q string) ([]metadata.MusicSuggestion, error)
	// MusicCover reads an album cover URL from the metadata server; nil uses
	// only the Cover Art Archive.
	MusicCover func(ctx context.Context, mbid, artist, title string) (string, error)
}
