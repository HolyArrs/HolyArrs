package musicdata

import (
	"context"
	"net/url"
	"strconv"
	"strings"
)

// Song is one of an artist's top songs, in the shape the app's Deezer
// reader already has: length in milliseconds, the album's name and a
// cover of about 250 to 300 px.
type Song struct {
	Title    string `json:"title"`
	LengthMS int    `json:"length_ms"`
	Explicit bool   `json:"explicit"`
	Album    string `json:"album"`
	Cover    string `json:"cover,omitempty"`
}

// TopSongs is an artist's top songs and the source that gave them
// ("spotify", "applemusic" or "deezer").
type TopSongs struct {
	Source string `json:"source"`
	Songs  []Song `json:"songs"`
}

// topSongsMax is how many songs are kept.
const topSongsMax = 10

// TopSongs answers an artist's top songs: Spotify's (US market), then
// Apple Music's top-songs view, then Deezer's. ErrNone when none has any;
// ErrLater when one failed (or the artist's stored MusicBrainz answer is
// not there yet) and no later source had songs.
func (s *Service) TopSongs(ctx context.Context, mbid string) (TopSongs, error) {
	var t TopSongs
	if found, none := s.cached(ctx, "top:"+mbid, &t); found {
		if none {
			return TopSongs{}, ErrNone
		}
		return t, nil
	}
	failed := false
	for _, src := range []struct {
		name string
		get  func(context.Context, string) ([]Song, error)
	}{{"spotify", s.spotifyTop}, {"applemusic", s.appleTop}, {"deezer", s.deezerTop}} {
		songs, err := src.get(ctx, mbid)
		if err != nil {
			if ctx.Err() != nil {
				return TopSongs{}, ErrLater
			}
			failed = true
			continue
		}
		if len(songs) == 0 {
			continue
		}
		if len(songs) > topSongsMax {
			songs = songs[:topSongsMax]
		}
		t = TopSongs{Source: src.name, Songs: songs}
		keep := keepTopSongs
		if failed {
			keep = keepAfterFail
		}
		s.keep("top:"+mbid, t, keep)
		return t, nil
	}
	if failed {
		return TopSongs{}, ErrLater
	}
	s.keep("top:"+mbid, nil, keepNone)
	return TopSongs{}, ErrNone
}

type spotifyImage struct {
	URL   string `json:"url"`
	Width int    `json:"width"`
}

// spotifyCover is the smallest image at least 250 px wide (Spotify's 300),
// else the largest.
func spotifyCover(imgs []spotifyImage) string {
	best := spotifyImage{}
	for _, im := range imgs {
		switch {
		case im.Width >= 250 && (best.Width < 250 || im.Width < best.Width):
			best = im
		case best.Width < 250 && im.Width > best.Width:
			best = im
		}
	}
	return best.URL
}

func (s *Service) spotifyTop(ctx context.Context, mbid string) ([]Song, error) {
	id, err := s.o.Sources.SpotifyArtist(ctx, mbid)
	if err != nil || id == "" {
		return nil, err
	}
	var d struct {
		Tracks []struct {
			Name     string `json:"name"`
			Duration int    `json:"duration_ms"`
			Explicit bool   `json:"explicit"`
			Album    struct {
				Name   string         `json:"name"`
				Images []spotifyImage `json:"images"`
			} `json:"album"`
		} `json:"tracks"`
	}
	if _, err := s.o.Sources.SpotifyJSON(ctx, "/v1/artists/"+url.PathEscape(id)+"/top-tracks?market=US", &d); err != nil {
		return nil, err
	}
	var out []Song
	for _, t := range d.Tracks {
		if t.Name != "" {
			out = append(out, Song{Title: t.Name, LengthMS: t.Duration, Explicit: t.Explicit, Album: t.Album.Name, Cover: spotifyCover(t.Album.Images)})
		}
	}
	return out, nil
}

// appleArt fills an Apple artwork template at 250 px.
func appleArt(template string) string {
	if template == "" {
		return ""
	}
	return strings.NewReplacer("{w}", "250", "{h}", "250", "{f}", "jpg").Replace(template)
}

func (s *Service) appleTop(ctx context.Context, mbid string) ([]Song, error) {
	st, id, err := s.o.Sources.AppleArtist(ctx, mbid)
	if err != nil || id == "" {
		return nil, err
	}
	var d struct {
		Data []struct {
			Attributes struct {
				Name     string `json:"name"`
				Duration int    `json:"durationInMillis"`
				Rating   string `json:"contentRating"`
				Album    string `json:"albumName"`
				Artwork  struct {
					URL string `json:"url"`
				} `json:"artwork"`
			} `json:"attributes"`
		} `json:"data"`
	}
	path := "/v1/catalog/" + url.PathEscape(st) + "/artists/" + url.PathEscape(id) + "/view/top-songs"
	if _, err := s.o.Sources.AppleJSON(ctx, path, url.Values{"limit": {strconv.Itoa(topSongsMax)}}, &d); err != nil {
		return nil, err
	}
	var out []Song
	for _, x := range d.Data {
		a := x.Attributes
		if a.Name != "" {
			out = append(out, Song{Title: a.Name, LengthMS: a.Duration, Explicit: a.Rating == "explicit", Album: a.Album, Cover: appleArt(a.Artwork.URL)})
		}
	}
	return out, nil
}

func (s *Service) deezerTop(ctx context.Context, mbid string) ([]Song, error) {
	id, err := s.o.Sources.DeezerArtist(ctx, mbid)
	if err != nil || id == "" {
		return nil, err
	}
	var d struct {
		Data []struct {
			Title    string `json:"title"`
			Duration int    `json:"duration"`
			Explicit bool   `json:"explicit_lyrics"`
			Album    struct {
				Title string `json:"title"`
				Cover string `json:"cover_medium"`
			} `json:"album"`
		} `json:"data"`
	}
	if _, err := s.o.Sources.DeezerJSON(ctx, "/artist/"+url.PathEscape(id)+"/top", url.Values{"limit": {strconv.Itoa(topSongsMax)}}, &d); err != nil {
		return nil, err
	}
	var out []Song
	for _, t := range d.Data {
		if t.Title != "" {
			out = append(out, Song{Title: t.Title, LengthMS: t.Duration * 1000, Explicit: t.Explicit, Album: t.Album.Title, Cover: t.Album.Cover})
		}
	}
	return out, nil
}
