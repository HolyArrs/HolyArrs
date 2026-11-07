// Package music holds HolyArrs's Music module: what it needs from the rest
// of the program (Deps) and, in subpackages, its pure parts (score).
package music

import (
	"context"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
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
	// MusicRowsV7 is flag music_rows_v7 (plan 2026-10-23): Explore → Music
	// as a daily row plan of up to 30 rows, served 6 at a time with a cursor,
	// plus mixes, Because you have, Deep cuts, Turning N and Together with
	// (with MusicRowsV3). Off keeps the v3/v6 page.
	MusicRowsV7 bool
	// MusicListsV1 is flag music_lists_v1 (plan 2026-10-24): world-list
	// rows (charts turned into albums and artists) on Explore → Music and
	// New & Popular → Music, from MusicShelves. Off keeps today's pages.
	MusicListsV1 bool
	// MusicShelves reads the metadata server's world-list shelves for a
	// region; nil leaves the list rows out.
	MusicShelves func(ctx context.Context, region string) (metadata.MusicShelves, error)
	// MusicPools reads the metadata server's music catalog bundle (the row
	// templates' data; conditional on etag); nil keeps today's list rows.
	MusicPools func(ctx context.Context, etag string) ([]byte, string, error)
	// MusicRowsTplOff is flag music_rows_tpl_off: the kill switch that keeps
	// today's music_lists_v1 pages instead of the row template engine.
	MusicRowsTplOff bool
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
	// MusicTopSongs, MusicRising and MusicDiscography read an artist's top
	// songs, the day's rising artists and the discography a store knows when
	// MusicBrainz has none; nil leaves today's top songs and hides the rest.
	MusicTopSongs    func(ctx context.Context, mbid string) (metadata.MusicTopSongs, error)
	MusicRising      func(ctx context.Context) (metadata.MusicRising, error)
	MusicDiscography func(ctx context.Context, mbid string) (metadata.MusicDiscography, error)
	// MusicDeezer, MusicDeezerAlbum and MusicArtistBio read the metadata
	// server's kept Deezer marks (explicit, label, top songs) and an
	// artist's Wikipedia bio; installs never ask Deezer or Wikipedia
	// themselves. nil falls back to Library.ArtistImages, a developer
	// stand-in only (off by default).
	MusicDeezer      func(ctx context.Context, mbid string) (metadata.DeezerProfile, error)
	MusicDeezerAlbum func(ctx context.Context, id int64) (metadata.DeezerAlbumInfo, error)
	MusicArtistBio   func(ctx context.Context, mbid string) (*metadata.WikiSummary, error)
	// MusicPictures reads the stored picture records (version, size, main
	// colour) of a row's artists and albums in one call; nil answers none.
	MusicPictures func(ctx context.Context, artists, albums []string) (metadata.MusicPictures, error)
}
