package metadata

import (
	"context"
	"net/url"
	"strings"
)

// Batch artist data for the scored Music rows. Requests carry artist ids only.

// MusicSimilarScore is one artist like a seed, S normalised by the seed's
// top score.
type MusicSimilarScore struct {
	MBID   string  `json:"mbid"`
	Name   string  `json:"name"`
	S      float64 `json:"s"`
	Source string  `json:"source"`
}

// MusicGenreVote is one genre on an artist and its vote count.
type MusicGenreVote struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// MusicTopAlbum is one release group of an artist's, by listeners.
type MusicTopAlbum struct {
	MBID        string   `json:"mbid"`
	Title       string   `json:"title"`
	Type        string   `json:"type"`
	Date        string   `json:"date"`
	Listeners   int64    `json:"listeners"`
	Artists     []string `json:"artists"`
	ArtistNames []string `json:"artist_names"`
}

// MusicArtistData is one artist's genre votes and top release groups.
type MusicArtistData struct {
	MBID   string           `json:"mbid"`
	Genres []MusicGenreVote `json:"genres"`
	Albums []MusicTopAlbum  `json:"albums"`
}

// MusicListeners reads ListenBrainz listener counts for up to 60 artists.
func (s *Service) MusicListeners(ctx context.Context, ids []string) (map[string]int64, error) {
	var out struct {
		Items []struct {
			MBID      string `json:"mbid"`
			Listeners int64  `json:"listeners"`
		} `json:"items"`
	}
	if err := s.musicGet(ctx, "/v1/music/listeners", url.Values{"artists": {strings.Join(ids, ",")}}, &out); err != nil {
		return nil, err
	}
	m := map[string]int64{}
	for _, x := range out.Items {
		m[strings.ToLower(x.MBID)] = x.Listeners
	}
	return m, nil
}

// MusicSimilarSet reads the similar artists of up to 12 seeds.
func (s *Service) MusicSimilarSet(ctx context.Context, seeds []string) (map[string][]MusicSimilarScore, error) {
	var out struct {
		Seeds []struct {
			MBID    string              `json:"mbid"`
			Artists []MusicSimilarScore `json:"artists"`
		} `json:"seeds"`
	}
	if err := s.musicGet(ctx, "/v1/music/similar-set", url.Values{"seeds": {strings.Join(seeds, ",")}}, &out); err != nil {
		return nil, err
	}
	m := map[string][]MusicSimilarScore{}
	for _, x := range out.Seeds {
		m[strings.ToLower(x.MBID)] = x.Artists
	}
	return m, nil
}

// MusicArtistData reads genre votes and top release groups of up to 30
// artists; artists the server could not answer in time are left out.
func (s *Service) MusicArtistData(ctx context.Context, ids []string) ([]MusicArtistData, error) {
	var out struct {
		Artists []MusicArtistData `json:"artists"`
	}
	if err := s.musicGet(ctx, "/v1/music/artist-data", url.Values{"artists": {strings.Join(ids, ",")}}, &out); err != nil {
		return nil, err
	}
	return out.Artists, nil
}
