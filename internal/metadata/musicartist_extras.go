package metadata

import (
	"context"
	"net/url"
	"strconv"
	"strings"
)

// Per-artist music answers from the metadata server: an artist's top songs,
// the day's rising artists, and the discography a store knows when
// MusicBrainz has none. 404 is ErrNotFound and 503 ErrServiceUnavailable,
// as for every music read; the pages hide the section on either.

// MusicTopSong is one of an artist's most played songs.
type MusicTopSong struct {
	Title    string `json:"title"`
	LengthMS int    `json:"length_ms"`
	Explicit bool   `json:"explicit"`
	Album    string `json:"album"`
	Cover    string `json:"cover,omitempty"`
}

// MusicTopSongs is GET /v1/music/top-songs/{mbid}.
type MusicTopSongs struct {
	MBID   string         `json:"mbid"`
	Source string         `json:"source"`
	Songs  []MusicTopSong `json:"songs"`
}

// MusicTopSongs reads an artist's top songs.
func (s *Service) MusicTopSongs(ctx context.Context, mbid string) (MusicTopSongs, error) {
	if !ValidMBID(mbid) {
		return MusicTopSongs{}, ErrNotFound
	}
	var out MusicTopSongs
	if err := s.musicGet(ctx, "/v1/music/top-songs/"+strings.ToLower(mbid), url.Values{}, &out); err != nil {
		return MusicTopSongs{}, err
	}
	return out, nil
}

// MusicRisingArtist is one artist gaining the most listeners today.
type MusicRisingArtist struct {
	MBID      string `json:"mbid"`
	Name      string `json:"name"`
	Listeners int64  `json:"listeners"`
	Gain      int64  `json:"gain"`
	Rank      int    `json:"rank"`
}

// MusicRising is GET /v1/music/charts/rising: up to 20, biggest gain first.
type MusicRising struct {
	AsOf  string              `json:"as_of"`
	Items []MusicRisingArtist `json:"items"`
}

// MusicRisingToday reads the day's rising artists; rows without a valid
// MusicBrainz id are left out.
func (s *Service) MusicRisingToday(ctx context.Context) (MusicRising, error) {
	var out MusicRising
	if err := s.musicGet(ctx, "/v1/music/charts/rising", url.Values{}, &out); err != nil {
		return MusicRising{}, err
	}
	kept := out.Items[:0]
	for _, x := range out.Items {
		if ValidMBID(x.MBID) && strings.TrimSpace(x.Name) != "" {
			x.MBID = strings.ToLower(x.MBID)
			kept = append(kept, x)
		}
	}
	out.Items = kept
	return out, nil
}

// MusicStoreAlbum is one release a store knows for an artist MusicBrainz
// has nothing for. It has no MusicBrainz id, so it cannot be asked for.
type MusicStoreAlbum struct {
	Title       string `json:"title"`
	ReleaseDate string `json:"release_date,omitempty"`
	Type        string `json:"type"` // album, single, ep, compilation
	TrackCount  int    `json:"track_count,omitempty"`
	Explicit    bool   `json:"explicit,omitempty"`
	Barcode     string `json:"barcode,omitempty"`
	Cover       string `json:"cover,omitempty"`
}

// MusicDiscography is GET /v1/music/discography/{mbid}. Source
// "musicbrainz" with no albums means MusicBrainz's own list stands.
type MusicDiscography struct {
	MBID   string            `json:"mbid"`
	Source string            `json:"source"`
	Albums []MusicStoreAlbum `json:"albums"`
}

// MusicDiscography reads an artist's discography fallback.
func (s *Service) MusicDiscography(ctx context.Context, mbid string) (MusicDiscography, error) {
	if !ValidMBID(mbid) {
		return MusicDiscography{}, ErrNotFound
	}
	var out MusicDiscography
	if err := s.musicGet(ctx, "/v1/music/discography/"+strings.ToLower(mbid), url.Values{}, &out); err != nil {
		return MusicDiscography{}, err
	}
	if out.Source == "musicbrainz" {
		out.Albums = nil
	}
	return out, nil
}

// ArtistPopularity rides on the metadata server's artist answer. The most
// played ~2,500 artists have monthly listeners, a world rank and today's
// change; the others only total plays.
type ArtistPopularity struct {
	MonthlyListeners     int64  `json:"monthly_listeners,omitempty"`
	WorldRank            int    `json:"world_rank,omitempty"`
	ListenersChangeToday int64  `json:"listeners_change_today,omitempty"`
	TotalPlays           int64  `json:"total_plays,omitempty"`
	AsOf                 string `json:"as_of,omitempty"`
}

// orNil drops an answer with no figure in it, and listener figures without
// listeners.
func (p *ArtistPopularity) orNil() *ArtistPopularity {
	if p == nil {
		return nil
	}
	x := *p
	if x.MonthlyListeners <= 0 {
		x.MonthlyListeners, x.WorldRank, x.ListenersChangeToday = 0, 0, 0
	}
	if x.WorldRank < 0 {
		x.WorldRank = 0
	}
	if x.TotalPlays < 0 {
		x.TotalPlays = 0
	}
	if x.MonthlyListeners == 0 && x.TotalPlays == 0 {
		return nil
	}
	return &x
}

// MusicAlbumTrack is one track of the chosen edition.
type MusicAlbumTrack struct {
	Disc      int    `json:"disc"`
	Position  int    `json:"position"`
	Number    string `json:"number"`
	Title     string `json:"title"`
	LengthMS  int    `json:"length_ms"`
	Recording string `json:"recording"`
}

// MusicAlbumEdition is one edition (a MusicBrainz release).
type MusicAlbumEdition struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	Status         string   `json:"status"`
	Date           string   `json:"date"`
	Country        string   `json:"country"`
	Formats        []string `json:"formats"`
	TrackCount     int      `json:"track_count"`
	Disambiguation string   `json:"disambiguation"`
}

// MusicAlbum is GET /v1/music/album/{release group}: the album with its
// chosen edition's tracks, in one call.
type MusicAlbum struct {
	MBID             string              `json:"mbid"`
	Title            string              `json:"title"`
	Edition          *MusicAlbumEdition  `json:"edition"`
	Tracks           []MusicAlbumTrack   `json:"tracks"`
	Editions         []MusicAlbumEdition `json:"editions"`
	EditionCount     int                 `json:"edition_count"`
	EditionsComplete bool                `json:"editions_complete"`
}

// MusicAlbum reads an album with its chosen edition's tracks. A 202 (not
// stored yet) is ErrServicePending.
func (s *Service) MusicAlbum(ctx context.Context, mbid string) (MusicAlbum, error) {
	if !ValidMBID(mbid) {
		return MusicAlbum{}, ErrNotFound
	}
	var out MusicAlbum
	if err := s.musicGet(ctx, "/v1/music/album/"+strings.ToLower(mbid), url.Values{}, &out); err != nil {
		return MusicAlbum{}, err
	}
	return out, nil
}

// pressings turns the answer into the chosen edition with its tracks (the
// page asks the rest when picked); false when it has none.
func (a MusicAlbum) pressings() ([]Pressing, bool) {
	if a.Edition == nil || !ValidMBID(a.Edition.ID) || len(a.Tracks) == 0 {
		return nil, false
	}
	one := func(e MusicAlbumEdition) Pressing {
		return Pressing{MBID: strings.ToLower(e.ID), Name: e.Title, Status: strings.ToLower(e.Status), Country: e.Country, Date: e.Date,
			Disambiguation: e.Disambiguation, Formats: append([]string{}, e.Formats...), Tracks: []Track{}}
	}
	first := one(*a.Edition)
	for _, t := range a.Tracks {
		if strings.TrimSpace(t.Title) == "" {
			return nil, false
		}
		n, _ := strconv.Atoi(t.Number)
		if n == 0 {
			n = t.Position
		}
		disc := t.Disc
		if disc == 0 {
			disc = 1
		}
		first.Tracks = append(first.Tracks, Track{RecordingID: strings.ToLower(t.Recording), Disc: disc, Number: n, Position: len(first.Tracks) + 1, Name: t.Title, LengthMS: t.LengthMS})
	}
	return []Pressing{first}, true
}
