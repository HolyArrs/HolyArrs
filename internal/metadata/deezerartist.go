package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DeezerTrack is one popular song of an artist on Deezer.
type DeezerTrack struct {
	Title    string `json:"title"`
	LengthMS int    `json:"length_ms"`
	Explicit bool   `json:"explicit"`
	Album    string `json:"album"`
	Cover    string `json:"cover,omitempty"`
}

// DeezerAlbum is one album of an artist on Deezer (for the explicit mark,
// the label and a cover when MusicBrainz has none).
type DeezerAlbum struct {
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	Explicit bool   `json:"explicit"`
	Cover    string `json:"cover,omitempty"`
	Date     string `json:"release_date,omitempty"`
}

// DeezerProfile is what Deezer says about one artist by name.
type DeezerProfile struct {
	ID      int64         `json:"id"`
	Picture string        `json:"picture,omitempty"`
	Fans    int64         `json:"fans"`
	Top     []DeezerTrack `json:"top"`
	Albums  []DeezerAlbum `json:"albums"`
}

// DeezerAlbumInfo is one Deezer album's label and per-track explicit marks.
type DeezerAlbumInfo struct {
	Label    string          `json:"label,omitempty"`
	Explicit bool            `json:"explicit"`
	Tracks   map[string]bool `json:"-"` // normal track name: explicit
}

func (d *ArtistImages) json(ctx context.Context, u string, out any) error {
	b, err := d.getter().do(ctx, []string{d.base()}, func(ctx context.Context) (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	})
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

// Profile finds the artist by name on Deezer and reads its top songs and
// albums. A zero ID means Deezer does not know the name.
func (d *ArtistImages) Profile(ctx context.Context, name string) (DeezerProfile, error) {
	var p DeezerProfile
	name = strings.TrimSpace(name)
	if d == nil || name == "" {
		return p, nil
	}
	var found struct {
		Data []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
			XL   string `json:"picture_xl"`
			Fans int64  `json:"nb_fan"`
		} `json:"data"`
	}
	if err := d.json(ctx, d.base()+"/search/artist?"+url.Values{"q": {name}, "limit": {"10"}}.Encode(), &found); err != nil {
		return p, fmt.Errorf("Deezer artist search: %w", err)
	}
	for _, loose := range []bool{false, true} {
		for _, a := range found.Data {
			if p.ID == 0 && sameArtistName(a.Name, name, loose) {
				p.ID, p.Fans = a.ID, a.Fans
				if !strings.Contains(a.XL, "/artist//") {
					p.Picture = a.XL
				}
			}
		}
	}
	if p.ID == 0 {
		return p, nil
	}
	d.fillProfile(ctx, &p)
	return p, nil
}

// fillProfile reads the top songs and albums of the Deezer artist p.ID.
func (d *ArtistImages) fillProfile(ctx context.Context, p *DeezerProfile) {
	id := strconv.FormatInt(p.ID, 10)
	var top struct {
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
	if err := d.json(ctx, d.base()+"/artist/"+id+"/top?limit=10", &top); err == nil {
		for _, t := range top.Data {
			p.Top = append(p.Top, DeezerTrack{Title: t.Title, LengthMS: t.Duration * 1000, Explicit: t.Explicit, Album: t.Album.Title, Cover: t.Album.Cover})
		}
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
	if err := d.json(ctx, d.base()+"/artist/"+id+"/albums?limit=200", &albums); err == nil {
		for _, a := range albums.Data {
			p.Albums = append(p.Albums, DeezerAlbum{ID: a.ID, Title: a.Title, Explicit: a.Explicit, Cover: a.Cover, Date: a.Date})
		}
	}
}

// AlbumInfo reads one Deezer album's label and its tracks' explicit marks.
func (d *ArtistImages) AlbumInfo(ctx context.Context, id int64) (DeezerAlbumInfo, error) {
	out := DeezerAlbumInfo{Tracks: map[string]bool{}}
	if d == nil || id <= 0 {
		return out, nil
	}
	var a struct {
		Label    string `json:"label"`
		Explicit bool   `json:"explicit_lyrics"`
		Tracks   struct {
			Data []struct {
				Title    string `json:"title"`
				Explicit bool   `json:"explicit_lyrics"`
			} `json:"data"`
		} `json:"tracks"`
	}
	if err := d.json(ctx, d.base()+"/album/"+strconv.FormatInt(id, 10), &a); err != nil {
		return out, fmt.Errorf("Deezer album: %w", err)
	}
	out.Label, out.Explicit = a.Label, a.Explicit
	for _, t := range a.Tracks.Data {
		out.Tracks[SongKey(t.Title)] = t.Explicit
	}
	return out, nil
}

// SongKey folds a song or album name for loose matching across sources
// ("Get Lucky (Radio Edit)" and "get lucky" meet).
func SongKey(s string) string {
	s = strings.ToLower(s)
	if i := strings.IndexAny(s, "(["); i > 0 {
		s = s[:i]
	}
	return normalName(s)
}

// WikiBioBase is Wikipedia's REST address.
const WikiBioBase = "https://en.wikipedia.org/api/rest_v1"

var (
	wikiOnce sync.Once
	wikiGet  *getter
)

// ArtistBio reads the English Wikipedia summary for a music artist by name,
// trying "(band)" and "(musician)" when the bare name is a disambiguation
// page or not about music. nil when none fits.
func (d *ArtistImages) ArtistBio(ctx context.Context, name string) (*WikiSummary, error) {
	name = strings.TrimSpace(name)
	if d == nil || name == "" {
		return nil, nil
	}
	wikiOnce.Do(func() {
		wikiGet = &getter{name: "Wikipedia", http: d.HTTP, timeout: d.Timeout, gap: 200 * time.Millisecond, ttl: 7 * 24 * time.Hour, agent: d.UserAgent}
	})
	base := WikiBioBase
	if d.WikiBase != "" {
		base = strings.TrimRight(d.WikiBase, "/")
	}
	for _, t := range []string{name, name + " (band)", name + " (musician)", name + " (singer)"} {
		u := base + "/page/summary/" + url.PathEscape(strings.ReplaceAll(t, " ", "_"))
		b, err := wikiGet.do(ctx, []string{base}, func(ctx context.Context) (*http.Request, error) {
			return http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		})
		if err != nil {
			continue
		}
		var s struct {
			Type        string `json:"type"`
			Title       string `json:"title"`
			Description string `json:"description"`
			Extract     string `json:"extract"`
			URLs        struct {
				Desktop struct {
					Page string `json:"page"`
				} `json:"desktop"`
			} `json:"content_urls"`
		}
		if json.Unmarshal(b, &s) != nil || s.Type != "standard" || s.Extract == "" || !musicWords(s.Description+" "+s.Extract) {
			continue
		}
		if !wikipediaURL(s.URLs.Desktop.Page) {
			continue
		}
		return &WikiSummary{Title: s.Title, Extract: s.Extract, URL: s.URLs.Desktop.Page, License: "CC BY-SA 4.0", LicenseURL: ccBySA,
			Attribution: "From Wikipedia, the free encyclopedia"}, nil
	}
	return nil, nil
}

func musicWords(s string) bool {
	s = strings.ToLower(s)
	for _, w := range []string{"band", "singer", "musician", "rapper", "duo", "songwriter", "music", "dj", "producer", "group", "composer"} {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}
