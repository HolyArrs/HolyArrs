package musicdata

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"
)

// Album is one album's details. Each field comes from MusicBrainz first,
// then Apple Music, then Spotify, only when the sources before have none;
// Sources names where each came from. Barcode is the UPC or EAN, kept so
// an album can be matched to a MusicBrainz release by barcode later.
type Album struct {
	MBID       string            `json:"mbid"`
	Label      string            `json:"label,omitempty"`
	TrackCount int               `json:"track_count,omitempty"`
	Barcode    string            `json:"barcode,omitempty"`
	Explicit   *bool             `json:"explicit,omitempty"`
	Single     *bool             `json:"single,omitempty"`
	Complete   *bool             `json:"complete,omitempty"`
	Sources    map[string]string `json:"sources"`
}

func (a *Album) full() bool {
	return a.Label != "" && a.TrackCount > 0 && a.Barcode != "" && a.Explicit != nil && a.Single != nil
}

func (a *Album) setStr(field string, v *string, from, source string) {
	if *v == "" && from != "" {
		*v, a.Sources[field] = from, source
	}
}

func (a *Album) setInt(field string, v *int, from int, source string) {
	if *v == 0 && from > 0 {
		*v, a.Sources[field] = from, source
	}
}

func (a *Album) setBool(field string, v **bool, from bool, source string) {
	if *v == nil {
		b := from
		*v, a.Sources[field] = &b, source
	}
}

// spotifyAlbums is how many albums one Spotify call reads.
const spotifyAlbums = 20

// AlbumDetails answers each album's details (release-group ids), kept 30
// days (an hour when a source failed or MusicBrainz's answer was not
// stored yet). Albums no source knows anything about are left out.
func (s *Service) AlbumDetails(ctx context.Context, ids []string) map[string]Album {
	out := map[string]Album{}
	type work struct {
		a     Album
		short bool
	}
	var todo []*work
	for _, id := range ids {
		var a Album
		if found, none := s.cached(ctx, "album:"+id, &a); found {
			if !none {
				out[id] = a
			}
			continue
		}
		w := &work{a: Album{MBID: id, Sources: map[string]string{}}}
		w.short = !s.fromMusicBrainz(ctx, &w.a)
		if !w.a.full() {
			if err := s.fromApple(ctx, &w.a); err != nil {
				w.short = true
			}
		}
		todo = append(todo, w)
	}
	var spotify []*work
	for _, w := range todo {
		if !w.a.full() && ctx.Err() == nil {
			spotify = append(spotify, w)
		}
	}
	if len(spotify) > 0 {
		albums := make([]*Album, len(spotify))
		for i, w := range spotify {
			albums[i] = &w.a
		}
		for i, failed := range s.fromSpotify(ctx, albums) {
			spotify[i].short = spotify[i].short || failed
		}
	}
	for _, w := range todo {
		keep := keepAlbum
		if w.short {
			keep = keepAfterFail
		}
		if len(w.a.Sources) == 0 {
			if !w.short {
				s.keep("album:"+w.a.MBID, nil, keepNone)
			}
			continue
		}
		out[w.a.MBID] = w.a
		s.keep("album:"+w.a.MBID, w.a, keep)
	}
	return out
}

// fromMusicBrainz fills what the stored release-group answer has: the
// track count of its tracklist release, that release's barcode (else the
// first official one's), whether it is a single, and the label from the
// release's own stored answer when there is one. False when the release
// group's answer is not stored.
func (s *Service) fromMusicBrainz(ctx context.Context, a *Album) bool {
	if s.o.Stored == nil {
		return false
	}
	b, ok := s.o.Stored(ctx, "release-group", a.MBID)
	if !ok {
		return false
	}
	var d struct {
		Source struct {
			Type   string `json:"primary-type"`
			Tracks string `json:"tracklist_release"`
			Media  []struct {
				Count int `json:"track-count"`
			} `json:"media"`
			Releases []struct {
				ID      string `json:"id"`
				Status  string `json:"status"`
				Barcode string `json:"barcode"`
			} `json:"releases"`
		} `json:"source_data"`
	}
	if json.Unmarshal(b, &d) != nil {
		return false
	}
	src := d.Source
	n := 0
	for _, m := range src.Media {
		n += m.Count
	}
	a.setInt("track_count", &a.TrackCount, n, "musicbrainz")
	for _, r := range src.Releases {
		if r.ID == src.Tracks {
			a.setStr("barcode", &a.Barcode, strings.TrimSpace(r.Barcode), "musicbrainz")
		}
	}
	for _, r := range src.Releases {
		if r.Status == "Official" {
			a.setStr("barcode", &a.Barcode, strings.TrimSpace(r.Barcode), "musicbrainz")
		}
	}
	if src.Type != "" {
		a.setBool("single", &a.Single, src.Type == "Single", "musicbrainz")
	}
	if src.Tracks != "" {
		if rb, ok := s.o.Stored(ctx, "release", src.Tracks); ok {
			var rel struct {
				Source struct {
					Labels []struct {
						Label struct {
							Name string `json:"name"`
						} `json:"label"`
					} `json:"label-info"`
				} `json:"source_data"`
			}
			if json.Unmarshal(rb, &rel) == nil {
				for _, l := range rel.Source.Labels {
					a.setStr("label", &a.Label, strings.TrimSpace(l.Label.Name), "musicbrainz")
				}
			}
		}
	}
	return true
}

// fromApple fills the gaps from the Apple Music album the cover chain
// matched (or the same search, kept for next time).
func (s *Service) fromApple(ctx context.Context, a *Album) error {
	st, id, err := s.o.Sources.AppleAlbumID(ctx, a.MBID)
	if err != nil || id == "" {
		return err
	}
	var d struct {
		Data []struct {
			Attributes struct {
				Label    string `json:"recordLabel"`
				Tracks   int    `json:"trackCount"`
				UPC      string `json:"upc"`
				Rating   string `json:"contentRating"`
				Single   bool   `json:"isSingle"`
				Complete bool   `json:"isComplete"`
			} `json:"attributes"`
		} `json:"data"`
	}
	ok, err := s.o.Sources.AppleJSON(ctx, "/v1/catalog/"+url.PathEscape(st)+"/albums/"+url.PathEscape(id), nil, &d)
	if err != nil || !ok || len(d.Data) == 0 {
		return err
	}
	at := d.Data[0].Attributes
	a.setStr("label", &a.Label, strings.TrimSpace(at.Label), "applemusic")
	a.setInt("track_count", &a.TrackCount, at.Tracks, "applemusic")
	a.setStr("barcode", &a.Barcode, strings.TrimSpace(at.UPC), "applemusic")
	// Apple leaves contentRating out for an album that is not explicit.
	a.setBool("explicit", &a.Explicit, at.Rating == "explicit", "applemusic")
	a.setBool("single", &a.Single, at.Single, "applemusic")
	a.setBool("complete", &a.Complete, at.Complete, "applemusic")
	return nil
}

// fromSpotify fills the gaps from Spotify: each album found by one search
// (the same title, the artist among its artists, the year within one when
// both are known), then read 20 a call. It answers which albums had a
// failed call.
func (s *Service) fromSpotify(ctx context.Context, albums []*Album) []bool {
	failed := make([]bool, len(albums))
	byID := map[string]int{}
	var ask []string
	for i, a := range albums {
		id, err := s.spotifyAlbumID(ctx, a.MBID)
		if err != nil {
			failed[i] = true
			continue
		}
		if id != "" {
			byID[id] = i
			ask = append(ask, id)
		}
	}
	for len(ask) > 0 {
		n := min(spotifyAlbums, len(ask))
		var d struct {
			Albums []*struct {
				ID       string `json:"id"`
				Label    string `json:"label"`
				Total    int    `json:"total_tracks"`
				Type     string `json:"album_type"`
				External struct {
					UPC string `json:"upc"`
				} `json:"external_ids"`
				Tracks struct {
					Items []struct {
						Explicit bool `json:"explicit"`
					} `json:"items"`
				} `json:"tracks"`
			} `json:"albums"`
		}
		if _, err := s.o.Sources.SpotifyJSON(ctx, "/v1/albums?ids="+strings.Join(ask[:n], ",")+"&market=US", &d); err != nil {
			for _, id := range ask[:n] {
				failed[byID[id]] = true
			}
			ask = ask[n:]
			continue
		}
		for _, x := range d.Albums {
			if x == nil {
				continue
			}
			i, ok := byID[x.ID]
			if !ok {
				continue
			}
			a := albums[i]
			a.setStr("label", &a.Label, strings.TrimSpace(x.Label), "spotify")
			a.setInt("track_count", &a.TrackCount, x.Total, "spotify")
			a.setStr("barcode", &a.Barcode, strings.TrimSpace(x.External.UPC), "spotify")
			explicit := false
			for _, t := range x.Tracks.Items {
				explicit = explicit || t.Explicit
			}
			if len(x.Tracks.Items) > 0 {
				a.setBool("explicit", &a.Explicit, explicit, "spotify")
			}
			if x.Type != "" {
				a.setBool("single", &a.Single, x.Type == "single", "spotify")
			}
		}
		ask = ask[n:]
	}
	return failed
}

// spotifyAlbumID finds the album on Spotify by one search on its stored
// names ("" when none matches).
func (s *Service) spotifyAlbumID(ctx context.Context, mbid string) (string, error) {
	sub := s.o.Sources.Subject(ctx, imgstore.KindAlbum, mbid)
	if !sub.Known {
		return "", imgstore.ErrNotKnown
	}
	var d struct {
		Albums struct {
			Items []struct {
				ID      string `json:"id"`
				Name    string `json:"name"`
				Date    string `json:"release_date"`
				Artists []struct {
					Name string `json:"name"`
				} `json:"artists"`
			} `json:"items"`
		} `json:"albums"`
	}
	q := url.Values{"type": {"album"}, "q": {`album:"` + sub.Title + `" artist:"` + sub.Name + `"`}, "limit": {"10"}, "market": {"US"}}
	if _, err := s.o.Sources.SpotifyJSON(ctx, "/v1/search?"+q.Encode(), &d); err != nil {
		return "", err
	}
	for _, x := range d.Albums.Items {
		if !imgstore.SameTitle(x.Name, sub.Title) {
			continue
		}
		if y, _ := strconv.Atoi(firstN(x.Date, 4)); sub.Year > 0 && y > 0 && (y-sub.Year > 1 || sub.Year-y > 1) {
			continue
		}
		for _, ar := range x.Artists {
			if imgstore.SameName(ar.Name, sub.Name) {
				return x.ID, nil
			}
		}
	}
	return "", nil
}

func firstN(s string, n int) string {
	if len(s) < n {
		return s
	}
	return s[:n]
}
