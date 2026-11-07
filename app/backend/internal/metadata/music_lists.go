package metadata

import (
	"context"
	"net/url"
)

// MusicShelfItem is one album or artist on a world-list shelf, with its
// chart signals (MusicBrainz ids only).
type MusicShelfItem struct {
	Kind       string  `json:"kind"` // album or artist
	MBID       string  `json:"mbid"`
	ArtistMBID string  `json:"artist_mbid"`
	Title      string  `json:"title"`
	Artist     string  `json:"artist_name"`
	Rank       int     `json:"rank"`
	Prev       int     `json:"prev"`
	Peak       int     `json:"peak"`
	Days       int     `json:"days"`
	Streams    int64   `json:"streams"`
	Date       string  `json:"release_date"`
	Song       string  `json:"song"`
	P          float64 `json:"p"`
	M          float64 `json:"m"`
}

// MusicShelves is GET /v1/music/lists/shelves: one region's blocks
// (top_now, top_world, albums_week, albums_world, artists_week, climbing,
// new_entries, new_friday, big_new_albums, singles_artists, long_runners,
// number_ones, comebacks, neighbor).
type MusicShelves struct {
	Region   string `json:"region"`
	Chart    string `json:"chart_region"`
	Neighbor string `json:"neighbor_region"`
	// Season is the paid season the "tile:season" block holds.
	Season string                      `json:"season"`
	AsOf   string                      `json:"as_of"`
	Blocks map[string][]MusicShelfItem `json:"blocks"`
}

// MusicShelves reads one region's world-list shelves ("global" or an ISO
// country code).
func (s *Service) MusicShelves(ctx context.Context, region string) (MusicShelves, error) {
	var out MusicShelves
	if err := s.musicGet(ctx, "/v1/music/lists/shelves", url.Values{"region": {region}}, &out); err != nil {
		return MusicShelves{}, err
	}
	return out, nil
}
