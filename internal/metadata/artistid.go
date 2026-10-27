package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// ArtistIDs are an artist's ids at other sites, read from its MusicBrainz
// links. They tie a picture to the artist itself: two artists can share a
// name, never an id.
type ArtistIDs struct {
	Deezer   int64  `json:"deezer,omitempty"`
	Wikidata string `json:"wikidata,omitempty"` // Q and digits
}

// ArtistIDSource reads an artist's ids at other sites by MusicBrainz id.
type ArtistIDSource interface {
	ArtistIDs(ctx context.Context, mbid string) (ArtistIDs, error)
}

var (
	deezerArtistURL = regexp.MustCompile(`^https?://(?:www\.)?deezer\.com/(?:[a-z]{2}/)?artist/(\d+)/?$`)
	wikidataURL     = regexp.MustCompile(`^https?://(?:www\.)?wikidata\.org/wiki/(Q\d+)$`)
)

// parseArtistURLs reads the Deezer and Wikidata ids from an artist's links.
func parseArtistURLs(links []string) ArtistIDs {
	var ids ArtistIDs
	for _, l := range links {
		l = strings.TrimSpace(l)
		if m := deezerArtistURL.FindStringSubmatch(l); m != nil && ids.Deezer == 0 {
			ids.Deezer, _ = strconv.ParseInt(m[1], 10, 64)
		}
		if m := wikidataURL.FindStringSubmatch(l); m != nil && ids.Wikidata == "" {
			ids.Wikidata = m[1]
		}
	}
	return ids
}

// ArtistIDs reads an artist's Deezer and Wikidata ids from its MusicBrainz
// links.
func (m *MusicBrainz) ArtistIDs(ctx context.Context, mbid string) (ArtistIDs, error) {
	if !ValidMBID(mbid) {
		return ArtistIDs{}, ErrArtistNotFound
	}
	b, err := m.get(ctx, "/ws/2/artist/"+strings.ToLower(mbid), url.Values{"inc": {"url-rels"}})
	if err != nil {
		return ArtistIDs{}, err
	}
	var a struct {
		Relations []struct {
			URL struct {
				Resource string `json:"resource"`
			} `json:"url"`
		} `json:"relations"`
	}
	if err := json.Unmarshal(b, &a); err != nil {
		return ArtistIDs{}, fmt.Errorf("MusicBrainz: odd answer")
	}
	links := make([]string, 0, len(a.Relations))
	for _, r := range a.Relations {
		links = append(links, r.URL.Resource)
	}
	return parseArtistURLs(links), nil
}

// deezerPicture is a usable picture link of a Deezer artist ("" for the
// empty picture Deezer gives an artist without one).
func deezerPicture(list ...string) string {
	for _, p := range list {
		if p != "" && !strings.Contains(p, "/artist//") {
			return p
		}
	}
	return ""
}

// ProfileFor reads the Deezer profile of the artist: by the Deezer id from
// its MusicBrainz links when known, else by a name search that needs the
// exact name and at least two album titles shared with MusicBrainz (one
// when MusicBrainz lists fewer than three). A zero ID means no sure match.
func (d *ArtistImages) ProfileFor(ctx context.Context, name string, deezerID int64, albums []string) (DeezerProfile, error) {
	if d == nil {
		return DeezerProfile{}, nil
	}
	if deezerID > 0 {
		return d.profileByID(ctx, deezerID)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return DeezerProfile{}, nil
	}
	var found struct {
		Data []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := d.json(ctx, d.base()+"/search/artist?"+url.Values{"q": {name}, "limit": {"10"}}.Encode(), &found); err != nil {
		return DeezerProfile{}, fmt.Errorf("Deezer artist search: %w", err)
	}
	mine := map[string]bool{}
	for _, a := range albums {
		if k := SongKey(a); k != "" {
			mine[k] = true
		}
	}
	need := 2
	if len(mine) < 3 {
		need = 1
	}
	if len(mine) == 0 {
		return DeezerProfile{}, nil
	}
	tried := 0
	for _, a := range found.Data {
		if tried >= 3 {
			break
		}
		if !sameArtistName(a.Name, name, false) {
			continue
		}
		tried++
		p, err := d.profileByID(ctx, a.ID)
		if err != nil {
			continue
		}
		shared := map[string]bool{}
		for _, al := range p.Albums {
			if k := SongKey(al.Title); mine[k] {
				shared[k] = true
			}
		}
		if len(shared) >= need {
			return p, nil
		}
	}
	return DeezerProfile{}, nil
}

// profileByID reads one Deezer artist's picture, fans, top songs and albums.
func (d *ArtistImages) profileByID(ctx context.Context, id int64) (DeezerProfile, error) {
	var a struct {
		ID   int64  `json:"id"`
		XL   string `json:"picture_xl"`
		Big  string `json:"picture_big"`
		Fans int64  `json:"nb_fan"`
	}
	if err := d.json(ctx, d.base()+"/artist/"+strconv.FormatInt(id, 10), &a); err != nil {
		return DeezerProfile{}, fmt.Errorf("Deezer artist: %w", err)
	}
	if a.ID != id {
		return DeezerProfile{}, nil
	}
	p := DeezerProfile{ID: id, Fans: a.Fans, Picture: deezerPicture(a.XL, a.Big)}
	d.fillProfile(ctx, &p)
	return p, nil
}
