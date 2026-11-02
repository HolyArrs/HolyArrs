package musicdata

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Deezer answers, kept keepDeezer (an install never asks Deezer itself):
// an artist's explicit marks per album, its fans and top songs, and one
// album's label and explicit marks per track.
const keepDeezer = 14 * 24 * time.Hour

// DeezerAlbum is one album of an artist on Deezer.
type DeezerAlbum struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	Explicit bool   `json:"explicit"`
	Cover    string `json:"cover,omitempty"`
	Date     string `json:"release_date,omitempty"`
}

// DeezerArtist is the Deezer artist matched to a MusicBrainz artist.
type DeezerArtist struct {
	ID      int64         `json:"id"`
	Picture string        `json:"picture,omitempty"`
	Fans    int64         `json:"fans"`
	Top     []Song        `json:"top"`
	Albums  []DeezerAlbum `json:"albums"`
}

// DeezerTrack is one track's explicit mark on a Deezer album.
type DeezerTrack struct {
	Title    string `json:"title"`
	Explicit bool   `json:"explicit"`
}

// DeezerAlbumInfo is one Deezer album's label and explicit marks.
type DeezerAlbumInfo struct {
	ID       int64         `json:"id"`
	Label    string        `json:"label,omitempty"`
	Explicit bool          `json:"explicit"`
	Tracks   []DeezerTrack `json:"tracks"`
}

// stale reads a kept answer even past its time (a failed refresh serves
// it rather than nothing).
func (s *Service) stale(ctx context.Context, k string, out any) bool {
	var body []byte
	if s.db.QueryRowContext(ctx, `SELECT body FROM answers WHERE k = ?`, k).Scan(&body) != nil || body == nil {
		return false
	}
	return json.Unmarshal(body, out) == nil
}

// DeezerArtist answers the Deezer artist matched to mbid (the picture
// chains' kept match), with its albums and top songs. ErrNone when Deezer
// has no match; ErrLater when Deezer failed or the artist's MusicBrainz
// answer is not stored yet (a kept older answer is served then).
func (s *Service) DeezerArtist(ctx context.Context, mbid string) (DeezerArtist, error) {
	k := "dz-artist:" + mbid
	var a DeezerArtist
	if found, none := s.cached(ctx, k, &a); found {
		if none {
			return DeezerArtist{}, ErrNone
		}
		return a, nil
	}
	a, err := s.deezerArtist(ctx, mbid)
	switch {
	case errors.Is(err, ErrNone):
		s.keep(k, nil, keepNone)
		return DeezerArtist{}, ErrNone
	case err != nil:
		var old DeezerArtist
		if s.stale(ctx, k, &old) {
			return old, nil
		}
		return DeezerArtist{}, ErrLater
	}
	s.keep(k, a, keepDeezer)
	return a, nil
}

func (s *Service) deezerArtist(ctx context.Context, mbid string) (DeezerArtist, error) {
	id, err := s.o.Sources.DeezerArtist(ctx, mbid)
	if err != nil {
		return DeezerArtist{}, err
	}
	n, _ := strconv.ParseInt(id, 10, 64)
	if n <= 0 {
		return DeezerArtist{}, ErrNone
	}
	var head struct {
		ID   int64  `json:"id"`
		XL   string `json:"picture_xl"`
		Big  string `json:"picture_big"`
		Fans int64  `json:"nb_fan"`
	}
	ok, err := s.o.Sources.DeezerJSON(ctx, "/artist/"+id, nil, &head)
	if err != nil {
		return DeezerArtist{}, err
	}
	if !ok || head.ID != n {
		return DeezerArtist{}, ErrNone
	}
	a := DeezerArtist{ID: n, Fans: head.Fans, Top: []Song{}, Albums: []DeezerAlbum{}}
	for _, p := range []string{head.XL, head.Big} {
		if p != "" && !strings.Contains(p, "/artist//") {
			a.Picture = p
			break
		}
	}
	if a.Top, err = s.deezerTop(ctx, mbid); err != nil {
		return DeezerArtist{}, err
	}
	if a.Top == nil {
		a.Top = []Song{}
	}
	var albums struct {
		Data []struct {
			ID       int64  `json:"id"`
			Title    string `json:"title"`
			Explicit bool   `json:"explicit_lyrics"`
			Cover    string `json:"cover_xl"`
			Date     string `json:"release_date"`
		} `json:"data"`
	}
	if _, err := s.o.Sources.DeezerJSON(ctx, "/artist/"+id+"/albums", url.Values{"limit": {"200"}}, &albums); err != nil {
		return DeezerArtist{}, err
	}
	for _, al := range albums.Data {
		if al.ID > 0 && al.Title != "" {
			a.Albums = append(a.Albums, DeezerAlbum{ID: al.ID, Title: al.Title, Explicit: al.Explicit, Cover: al.Cover, Date: al.Date})
		}
	}
	return a, nil
}

// DeezerAlbum answers one Deezer album's label and per-track explicit
// marks (by Deezer album id, as DeezerArtist lists them). ErrNone when
// Deezer has no such album; ErrLater when it failed and nothing is kept.
func (s *Service) DeezerAlbum(ctx context.Context, id int64) (DeezerAlbumInfo, error) {
	k := "dz-album:" + strconv.FormatInt(id, 10)
	var a DeezerAlbumInfo
	if found, none := s.cached(ctx, k, &a); found {
		if none {
			return DeezerAlbumInfo{}, ErrNone
		}
		return a, nil
	}
	var raw struct {
		ID       int64  `json:"id"`
		Label    string `json:"label"`
		Explicit bool   `json:"explicit_lyrics"`
		Tracks   struct {
			Data []struct {
				Title    string `json:"title"`
				Explicit bool   `json:"explicit_lyrics"`
			} `json:"data"`
		} `json:"tracks"`
	}
	ok, err := s.o.Sources.DeezerJSON(ctx, "/album/"+strconv.FormatInt(id, 10), nil, &raw)
	if err != nil {
		var old DeezerAlbumInfo
		if s.stale(ctx, k, &old) {
			return old, nil
		}
		return DeezerAlbumInfo{}, ErrLater
	}
	if !ok || raw.ID != id {
		s.keep(k, nil, keepNone)
		return DeezerAlbumInfo{}, ErrNone
	}
	a = DeezerAlbumInfo{ID: id, Label: raw.Label, Explicit: raw.Explicit, Tracks: []DeezerTrack{}}
	for _, t := range raw.Tracks.Data {
		if t.Title != "" {
			a.Tracks = append(a.Tracks, DeezerTrack{Title: t.Title, Explicit: t.Explicit})
		}
	}
	s.keep(k, a, keepDeezer)
	return a, nil
}
