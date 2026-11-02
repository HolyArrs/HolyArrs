package lists

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/musictrend"
)

// The lists as the music job's charts (musictrend.ListCharts): a region's
// song, album or artist chart and the Browse genre pages, with MusicBrainz
// ids and our own words only.

// genreNames are the genre tiles' names in our own words; the other tiles
// (moods, decades, fresh faces, the season) are no genre.
var genreNames = map[string]string{
	"pop": "Pop", "latin": "Latin", "rock": "Rock", "country": "Country", "dance": "Dance & electronic", "rnb": "R&B",
	"indie": "Indie", "kpop": "K-pop", "metal": "Metal", "jazz": "Jazz", "alternative": "Alternative", "folk": "Folk & acoustic",
	"soul": "Soul", "classical": "Classical", "punk": "Punk", "blues": "Blues", "funk": "Funk & disco", "ambient": "Ambient",
	"afro": "Afro", "brazil": "Brazil",
}

// ChartList answers a region's kept chart ("songs", "albums", "artists"
// or "new", the new releases) with the stored matches, and the same chart seven days ago.
// chartMaxAge is how old a stored list can be before Charts uses Apple
// instead: the weekly refresh plus slack.
const chartMaxAge = 10 * 24 * time.Hour

func (s *Service) ChartList(ctx context.Context, region, kind string) (musictrend.ListChart, bool) {
	var k Kind
	switch kind {
	case "songs":
		k = KindSongs
	case "albums":
		k = KindAlbums
	case "artists":
		k = KindArtists
	case "new":
		k = KindNew
	default:
		return musictrend.ListChart{}, false
	}
	key := string(k) + ":" + region
	es, day := s.latest(key)
	if len(es) == 0 {
		return musictrend.ListChart{}, false
	}
	// A list not refreshed in over chartMaxAge (the quota ran out) gives way
	// to the fresh Apple chart. A day that won't parse counts as fresh.
	if d, err := time.Parse("2006-01-02", day); err == nil && s.opts.Now().Sub(d) > chartMaxAge {
		return musictrend.ListChart{}, false
	}
	start := time.Now()
	look0 := s.lookup(ctx)
	var tLook, tExact, tShared time.Duration
	look := func(kind, a, b string) (string, string, bool) {
		t := time.Now()
		defer func() { tLook += time.Since(t) }()
		return look0(kind, a, b)
	}
	entry := func(e Entry) musictrend.Entry {
		out := musictrend.Entry{Name: cleanTitle(e.Title), Artist: e.Artist, Date: e.ReleaseDate}
		switch k {
		case KindAlbums, KindNew:
			out.MBID, out.ArtistMBID, _ = look("album", e.Title, e.Artist)
			if out.ArtistMBID == "" {
				out.ArtistMBID, _, _ = look("artist", e.Artist, "")
			}
		case KindArtists:
			out.Name = e.Artist
			out.MBID, _, _ = look("artist", e.Artist, "")
		default:
			out.ArtistMBID, _, _ = look("artist", e.Artist, "")
		}
		// The row's own artist id, resolved exactly, beats the name match.
		x := ""
		t := time.Now()
		if s.opts.ExactArtist != nil && e.ExtArtist != "" {
			x = s.opts.ExactArtist(ctx, "spotify", e.ExtArtist, e.Artist, nil)
		}
		tExact += time.Since(t)
		t = time.Now()
		shared := x == "" && s.opts.SharedName != nil && e.Artist != "" && s.opts.SharedName(ctx, e.Artist)
		tShared += time.Since(t)
		if shared {
			// Never the wrong artist: a shared name without exact evidence
			// links to none.
			if k == KindArtists {
				out.MBID, out.Unlinked = "", true
			} else {
				out.ArtistMBID, out.Unlinked = "", true
			}
		}
		{
			if x != "" {
				switch k {
				case KindArtists:
					out.MBID = x
				default:
					if out.ArtistMBID != x && (k == KindAlbums || k == KindNew) {
						out.MBID = "" // found again under the right artist
					}
					out.ArtistMBID = x
				}
			}
		}
		return out
	}
	c := musictrend.ListChart{}
	for _, e := range es {
		c.Entries = append(c.Entries, entry(e))
	}
	if d := time.Since(start); d > 2*time.Second {
		log.Printf("lists: chart %s slow: %s (%d rows; lookup %s, exact artist %s, shared name %s)", key, d.Round(time.Millisecond), len(es), tLook.Round(time.Millisecond), tExact.Round(time.Millisecond), tShared.Round(time.Millisecond))
	}
	if ago := s.at(key, s.opts.Now().UTC().AddDate(0, 0, -7).Format("2006-01-02")); len(ago) > 0 {
		c.HaveAgo = true
		for _, e := range ago {
			out := musictrend.Entry{Name: cleanTitle(e.Title), Artist: e.Artist}
			if k == KindArtists {
				out.Name = e.Artist
			}
			c.Ago = append(c.Ago, out)
		}
	}
	return c, true
}

// GenreTiles answers the genre tiles whose block holds items, in plan
// order, with the block's artists.
func (s *Service) GenreTiles() []musictrend.GenreTile {
	s.mu.RLock()
	b := s.shelves["global"].body
	s.mu.RUnlock()
	var sh Shelves
	if json.Unmarshal(b, &sh) != nil {
		return nil
	}
	var out []musictrend.GenreTile
	for _, t := range Tiles {
		name, ok := genreNames[t.Key]
		items := sh.Blocks["tile:"+t.Key]
		if !ok || len(items) == 0 {
			continue
		}
		g := musictrend.GenreTile{Slug: t.Key, Name: name, Count: len(items)}
		seen := map[string]bool{}
		for _, it := range items {
			id := it.ArtistMBID
			if it.Kind == "artist" {
				id = it.MBID
			}
			if id != "" && !seen[id] {
				seen[id] = true
				g.Artists = append(g.Artists, musictrend.NamedArtist{ID: id, Name: it.Artist})
			}
		}
		out = append(out, g)
	}
	return out
}

// Built answers when the shelves were last built.
func (s *Service) Built() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.last
}
