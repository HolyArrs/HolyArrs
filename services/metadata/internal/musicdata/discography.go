package musicdata

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
)

// DiscAlbum is one album of an artist's discography from Apple Music or
// Spotify, for an artist MusicBrainz has no release groups for. These
// albums have no MusicBrainz id; Barcode (when the source gives one) lets
// them be matched to MusicBrainz later. Explicit is left out when the
// source does not say.
type DiscAlbum struct {
	Title       string `json:"title"`
	ReleaseDate string `json:"release_date,omitempty"`
	Type        string `json:"type"` // album, single, ep or compilation
	TrackCount  int    `json:"track_count,omitempty"`
	Explicit    *bool  `json:"explicit,omitempty"`
	Barcode     string `json:"barcode,omitempty"`
	Cover       string `json:"cover,omitempty"`
}

// Discography is an artist's albums and where they came from:
// "musicbrainz" (with no albums: the app shows MusicBrainz's own release
// groups), "applemusic" or "spotify".
type Discography struct {
	Source string      `json:"source"`
	Albums []DiscAlbum `json:"albums"`
}

// Apple pages: 25 albums each, at most 4 read.
const (
	applePage  = 25
	applePages = 4
)

// Discography answers an artist's albums: MusicBrainz's when its stored
// answer has release groups, else Apple Music's, else Spotify's (kept a
// week; an hour after a failure). ErrNone when none has any; ErrLater when
// the stored MusicBrainz answer is not there yet or a source failed and no
// later one had albums.
func (s *Service) Discography(ctx context.Context, mbid string) (Discography, error) {
	var d Discography
	if found, none := s.cached(ctx, "disc:"+mbid, &d); found {
		if none {
			return Discography{}, ErrNone
		}
		return d, nil
	}
	if s.o.Stored == nil {
		return Discography{}, ErrLater
	}
	b, ok := s.o.Stored(ctx, "artist", mbid)
	if !ok {
		return Discography{}, ErrLater
	}
	var mb struct {
		Source struct {
			Groups []json.RawMessage `json:"release-groups"`
		} `json:"source_data"`
	}
	if json.Unmarshal(b, &mb) == nil && len(mb.Source.Groups) > 0 {
		return Discography{Source: "musicbrainz", Albums: []DiscAlbum{}}, nil
	}
	failed := false
	for _, src := range []struct {
		name string
		get  func(context.Context, string) ([]DiscAlbum, error)
	}{{"applemusic", s.appleAlbums}, {"spotify", s.spotifyAlbums}} {
		albums, err := src.get(ctx, mbid)
		if err != nil {
			if ctx.Err() != nil {
				return Discography{}, ErrLater
			}
			failed = true
			continue
		}
		if len(albums) == 0 {
			continue
		}
		d = Discography{Source: src.name, Albums: albums}
		keep := keepDiscography
		if failed {
			keep = keepAfterFail
		}
		s.keep("disc:"+mbid, d, keep)
		return d, nil
	}
	if failed {
		return Discography{}, ErrLater
	}
	s.keep("disc:"+mbid, nil, keepNone)
	return Discography{}, ErrNone
}

func (s *Service) appleAlbums(ctx context.Context, mbid string) ([]DiscAlbum, error) {
	st, id, err := s.o.Sources.AppleArtist(ctx, mbid)
	if err != nil || id == "" {
		return nil, err
	}
	var out []DiscAlbum
	for page := 0; page < applePages; page++ {
		var d struct {
			Data []struct {
				Attributes struct {
					Name     string `json:"name"`
					Date     string `json:"releaseDate"`
					Tracks   int    `json:"trackCount"`
					Rating   string `json:"contentRating"`
					UPC      string `json:"upc"`
					Single   bool   `json:"isSingle"`
					Compiled bool   `json:"isCompilation"`
					Artwork  struct {
						URL string `json:"url"`
					} `json:"artwork"`
				} `json:"attributes"`
			} `json:"data"`
			Next string `json:"next"`
		}
		q := url.Values{"limit": {strconv.Itoa(applePage)}}
		if page > 0 {
			q.Set("offset", strconv.Itoa(page*applePage))
		}
		if _, err := s.o.Sources.AppleJSON(ctx, "/v1/catalog/"+url.PathEscape(st)+"/artists/"+url.PathEscape(id)+"/albums", q, &d); err != nil {
			return nil, err
		}
		for _, x := range d.Data {
			a := x.Attributes
			if a.Name == "" {
				continue
			}
			explicit := a.Rating == "explicit"
			al := DiscAlbum{Title: a.Name, ReleaseDate: a.Date, Type: "album", TrackCount: a.Tracks, Explicit: &explicit, Barcode: strings.TrimSpace(a.UPC), Cover: appleArt(a.Artwork.URL)}
			switch {
			case a.Compiled:
				al.Type = "compilation"
			case strings.HasSuffix(a.Name, " - EP"):
				al.Type, al.Title = "ep", strings.TrimSuffix(a.Name, " - EP")
			case a.Single:
				al.Type, al.Title = "single", strings.TrimSuffix(a.Name, " - Single")
			}
			out = append(out, al)
		}
		if d.Next == "" {
			break
		}
	}
	return out, nil
}

func (s *Service) spotifyAlbums(ctx context.Context, mbid string) ([]DiscAlbum, error) {
	id, err := s.o.Sources.SpotifyArtist(ctx, mbid)
	if err != nil || id == "" {
		return nil, err
	}
	var d struct {
		Items []struct {
			Name   string         `json:"name"`
			Date   string         `json:"release_date"`
			Type   string         `json:"album_type"`
			Tracks int            `json:"total_tracks"`
			Images []spotifyImage `json:"images"`
		} `json:"items"`
	}
	q := url.Values{"include_groups": {"album,single,compilation"}, "market": {"US"}, "limit": {"50"}}
	if _, err := s.o.Sources.SpotifyJSON(ctx, "/v1/artists/"+url.PathEscape(id)+"/albums?"+q.Encode(), &d); err != nil {
		return nil, err
	}
	var out []DiscAlbum
	for _, x := range d.Items {
		if x.Name == "" {
			continue
		}
		t := x.Type
		if t != "single" && t != "compilation" {
			t = "album"
		}
		out = append(out, DiscAlbum{Title: x.Name, ReleaseDate: x.Date, Type: t, TrackCount: x.Tracks, Cover: spotifyCover(x.Images)})
	}
	return out, nil
}
