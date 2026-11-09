package metadata

import (
	"context"
	"net/url"
	"regexp"
	"strings"
)

// PictureRecord is one picture the metadata server has stored: its version
// (the ?v= that makes its address answer immutable), its stored size and
// its main colour (#rrggbb), so a card draws its box and colour before the
// picture arrives.
type PictureRecord struct {
	V     string `json:"v"`
	W     int    `json:"w"`
	H     int    `json:"h"`
	Color string `json:"color"`
}

// ArtistPictures are an artist's stored pictures; each is nil until stored.
type ArtistPictures struct {
	Photo *PictureRecord `json:"photo,omitempty"`
	Wide  *PictureRecord `json:"wide,omitempty"`
	Logo  *PictureRecord `json:"logo,omitempty"`
}

// AlbumPictures are an album's stored pictures (its cover).
type AlbumPictures struct {
	Cover *PictureRecord `json:"cover,omitempty"`
}

// MusicPictures answers the batch picture call, by MusicBrainz id.
type MusicPictures struct {
	Artists map[string]ArtistPictures `json:"artists"`
	Albums  map[string]AlbumPictures  `json:"albums"`
}

// MusicPicturesMax is the most ids one batch call takes (artists and albums
// together).
const MusicPicturesMax = 100

var (
	pictureVersion = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	pictureColor   = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
)

// ValidPictureVersion says v can stand in a picture address (?v=).
func ValidPictureVersion(v string) bool { return pictureVersion.MatchString(v) }

// clean keeps a record only when every field is sane; the colour is kept
// in lower case.
func (p *PictureRecord) clean() *PictureRecord {
	if p == nil || !ValidPictureVersion(p.V) || p.W <= 0 || p.H <= 0 || p.W > 20000 || p.H > 20000 {
		return nil
	}
	out := *p
	if !pictureColor.MatchString(out.Color) {
		out.Color = ""
	}
	out.Color = strings.ToLower(out.Color)
	return &out
}

// MusicPictures reads the stored picture records of up to MusicPicturesMax
// artists and albums in one call (a whole row). Ids that are not MusicBrainz
// ids are left out. The server queues a few missing pictures per call, so
// asking again a minute later can find more.
func (s *Service) MusicPictures(ctx context.Context, artists, albums []string) (MusicPictures, error) {
	out := MusicPictures{Artists: map[string]ArtistPictures{}, Albums: map[string]AlbumPictures{}}
	keep := func(ids []string, room int) []string {
		var k []string
		seen := map[string]bool{}
		for _, id := range ids {
			id = strings.ToLower(id)
			if len(k) < room && ValidMBID(id) && !seen[id] {
				seen[id] = true
				k = append(k, id)
			}
		}
		return k
	}
	artists = keep(artists, MusicPicturesMax)
	albums = keep(albums, MusicPicturesMax-len(artists))
	if len(artists)+len(albums) == 0 {
		return out, nil
	}
	q := url.Values{}
	if len(artists) > 0 {
		q.Set("artists", strings.Join(artists, ","))
	}
	if len(albums) > 0 {
		q.Set("albums", strings.Join(albums, ","))
	}
	var raw MusicPictures
	if err := s.musicGet(ctx, "/v1/music/pictures", q, &raw); err != nil {
		return out, err
	}
	asked := map[string]bool{}
	for _, id := range append(append([]string{}, artists...), albums...) {
		asked[id] = true
	}
	for id, p := range raw.Artists {
		if id = strings.ToLower(id); asked[id] {
			out.Artists[id] = ArtistPictures{Photo: p.Photo.clean(), Wide: p.Wide.clean(), Logo: p.Logo.clean()}
		}
	}
	for id, p := range raw.Albums {
		if id = strings.ToLower(id); asked[id] {
			out.Albums[id] = AlbumPictures{Cover: p.Cover.clean()}
		}
	}
	return out, nil
}
