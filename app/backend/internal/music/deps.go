// Package music holds the Music module. Deps lists what the module needs
// from the rest of the program, and the subpackages hold its parts: score
// for ranking and row plans, rowtpl for the row template engine.
package music

import (
	"context"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Deps holds the metadata server calls and feature flags the Music module
// reads. A nil call means that data is left out of the page.
type Deps struct {
	// MusicTrending reads a trending list from the metadata server. Nil hides those rows.
	MusicTrending func(ctx context.Context, kind, region string) (metadata.MusicTrend, error)
	// MusicNew reads a new-music window (tracks, week, recent, upcoming). Nil leaves New & Popular to MusicBrainz alone.
	MusicNew func(ctx context.Context, window string) (metadata.MusicNew, error)
	// MusicSimilar reads similar artists. Nil removes the rows built on them.
	MusicSimilar func(ctx context.Context, mbid string) ([]metadata.SimilarArtist, error)
	// MusicCharts reads region charts for the Explore rows. Nil leaves those rows out.
	MusicCharts func(ctx context.Context, region, kind, genre string) (metadata.MusicChart, error)
	// MusicGenres reads the genre tiles for the same rows. Nil leaves them out.
	MusicGenres func(ctx context.Context, region string) (metadata.MusicGenres, error)
	// MusicGenreTile reads the grey picture of a genre tile. Nil keeps every tile a flat color.
	MusicGenreTile func(ctx context.Context, slug, version string) ([]byte, error)
	// MusicShelves reads the world-list shelves for a region. Nil leaves the list rows out.
	MusicShelves func(ctx context.Context, region string) (metadata.MusicShelves, error)
	// MusicPools reads the music catalog bundle that feeds the row templates, conditional on its etag. Nil keeps the older list rows.
	MusicPools func(ctx context.Context, etag string) ([]byte, string, error)
	// MusicRowsTplOff is a kill switch that keeps the older list pages instead of the row template engine.
	MusicRowsTplOff bool
	// MusicListeners reads listener counts per artist for scoring. Nil leaves the chart numbers as they are.
	MusicListeners func(ctx context.Context, ids []string) (map[string]int64, error)
	// MusicSimilarSet reads similar artists for many seeds at once. Nil drops the similarity rows.
	MusicSimilarSet func(ctx context.Context, seeds []string) (map[string][]metadata.MusicSimilarScore, error)
	// MusicArtistData reads batch artist data for the scored rows. Nil drops those rows.
	MusicArtistData func(ctx context.Context, ids []string) ([]metadata.MusicArtistData, error)
	// MusicSuggest reads the suggest index of charted artists, albums and songs. Nil leaves the dropdown to the Library and TMDB.
	MusicSuggest func(ctx context.Context, q string) ([]metadata.MusicSuggestion, error)
	// MusicTopSongs reads the top songs of an artist. Nil keeps the default top songs.
	MusicTopSongs func(ctx context.Context, mbid string) (metadata.MusicTopSongs, error)
	// MusicRising reads the artists rising today. Nil hides the row.
	MusicRising func(ctx context.Context) (metadata.MusicRising, error)
	// MusicDiscography reads a discography from a store when MusicBrainz has none. Nil hides it.
	MusicDiscography func(ctx context.Context, mbid string) (metadata.MusicDiscography, error)
	// MusicDeezer reads kept Deezer marks (explicit, label, top songs) from the metadata server, so installs never call Deezer. Nil falls back to Library.ArtistImages, a developer stand-in that is off by default.
	MusicDeezer func(ctx context.Context, mbid string) (metadata.DeezerProfile, error)
	// MusicDeezerAlbum reads kept Deezer info for one album. Nil leaves it out.
	MusicDeezerAlbum func(ctx context.Context, id int64) (metadata.DeezerAlbumInfo, error)
	// MusicArtistBio reads an artist bio from the metadata server, so installs never call Wikipedia. Nil leaves the bio out.
	MusicArtistBio func(ctx context.Context, mbid string) (*metadata.WikiSummary, error)
	// MusicPictures reads stored picture records (version, size, main color) for a row's artists and albums in one call. Nil answers none.
	MusicPictures func(ctx context.Context, artists, albums []string) (metadata.MusicPictures, error)
}
