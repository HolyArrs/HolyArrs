package musictrend

import (
	"context"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Apple Music API charts: the most-played albums and songs per storefront
// (read daily, 100 of each in two pages) and, for a genre page, the same
// charts for one Apple genre (read when asked, kept a day). The API is
// read through the picture chains' resolver (its signed token, pace and
// limits).

// AppleAPI reads one Apple Music API answer into out; ok is false when the
// source is off or has no such thing.
type AppleAPI func(ctx context.Context, path string, q url.Values, out any) (ok bool, err error)

// appleChartPage is how many entries one chart page holds (the API's most).
const appleChartPage = 50

type appleChartItem struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Attributes struct {
		Name      string   `json:"name"`
		Artist    string   `json:"artistName"`
		ArtistURL string   `json:"artistUrl"`
		UPC       string   `json:"upc"`
		Date      string   `json:"releaseDate"`
		Rating    string   `json:"contentRating"`
		Names     []string `json:"genreNames"`
		Artwork   struct {
			URL string `json:"url"`
		} `json:"artwork"`
	} `json:"attributes"`
}

type appleChartAnswer struct {
	Results struct {
		Albums []struct {
			Data []appleChartItem `json:"data"`
			Next string           `json:"next"`
		} `json:"albums"`
		Songs []struct {
			Data []appleChartItem `json:"data"`
			Next string           `json:"next"`
		} `json:"songs"`
	} `json:"results"`
}

// appleEntries turns chart items into entries in chart order: the artwork
// at 500 px, explicit from the content rating, Apple's genres without the
// catch-all "Music"; an album or song twice is kept once.
func appleEntries(items []appleChartItem, seen map[string]bool) []Entry {
	var out []Entry
	for _, x := range items {
		a := x.Attributes
		k := norm(a.Artist) + "|" + norm(baseTitle(a.Name))
		if a.Name == "" || a.Artist == "" || seen[k] {
			continue
		}
		seen[k] = true
		e := Entry{Name: a.Name, Artist: a.Artist, Date: a.Date, Explicit: a.Rating == "explicit"}
		if digits(x.ID) && (x.Type == "albums" || x.Type == "songs") {
			e.AppleID, e.AppleKind = x.ID, x.Type
		}
		if m := appleArtistURL.FindStringSubmatch(a.ArtistURL); m != nil {
			e.AppleArtist = m[1]
		}
		if barcodeRe.MatchString(a.UPC) {
			e.UPC = a.UPC
		}
		e.Name, e.Type = SplitReleaseTag(a.Name)
		if a.Artwork.URL != "" {
			e.Cover = strings.NewReplacer("{w}", "500", "{h}", "500", "{f}", "jpg").Replace(a.Artwork.URL)
			if e.UPC == "" {
				e.UPC = coverBarcode(a.Artwork.URL)
			}
		}
		for _, g := range a.Names {
			if g != "" && g != "Music" {
				e.Genres = append(e.Genres, g)
			}
		}
		out = append(out, e)
	}
	return out
}

// readAppleChart reads a storefront's albums and songs charts (genre "" for
// all genres, else an Apple genre id), pages pages of each. ok is false
// when Apple has no chart for it.
func readAppleChart(ctx context.Context, apple AppleAPI, st, genre string, pages int) (albums, songs []Entry, ok bool, err error) {
	seenA, seenS := map[string]bool{}, map[string]bool{}
	for page := 0; page < pages; page++ {
		q := url.Values{"types": {"albums,songs"}, "chart": {"most-played"}, "limit": {strconv.Itoa(appleChartPage)}}
		if page > 0 {
			q.Set("offset", strconv.Itoa(page*appleChartPage))
		}
		if genre != "" {
			q.Set("genre", genre)
		}
		var d appleChartAnswer
		found, err := apple(ctx, "/v1/catalog/"+url.PathEscape(st)+"/charts", q, &d)
		if err != nil {
			return nil, nil, false, err
		}
		if !found {
			break
		}
		ok = true
		more := false
		for _, c := range d.Results.Albums {
			albums = append(albums, appleEntries(c.Data, seenA)...)
			more = more || c.Next != ""
		}
		for _, c := range d.Results.Songs {
			songs = append(songs, appleEntries(c.Data, seenS)...)
			more = more || c.Next != ""
		}
		if !more {
			break
		}
	}
	return albums, songs, ok && (len(albums) > 0 || len(songs) > 0), nil
}

// genreAliases are the Apple genre names each genre page stands for, the
// closest first (Apple's top-level genres differ per storefront).
var genreAliases = map[string][]string{
	"pop": {"Pop"}, "latin": {"Latin", "Música Mexicana", "Urbano latino", "Pop Latino"}, "rock": {"Rock", "Hard Rock"},
	"country": {"Country"}, "dance": {"Dance", "Electronic"}, "rnb": {"R&B/Soul"}, "indie": {"Alternative", "Indie Pop", "Indie Rock"},
	"kpop": {"K-Pop"}, "metal": {"Metal", "Hard Rock"}, "jazz": {"Jazz"}, "alternative": {"Alternative"},
	"folk": {"Singer/Songwriter", "Folk"}, "soul": {"R&B/Soul", "Soul"}, "classical": {"Classical"}, "punk": {"Punk", "Alternative"},
	"blues": {"Blues"}, "funk": {"Funk", "R&B/Soul"}, "ambient": {"Ambient", "Electronic"}, "afro": {"Afrobeats", "African", "Afro-Pop"},
	"brazil": {"Brazilian", "MPB", "Sertanejo"},
}

// inGenre reports whether an entry's Apple genres stand for a genre page.
func inGenre(e Entry, slug string) bool {
	for _, g := range e.Genres {
		if GenreSlug(g) == slug {
			return true
		}
		for _, a := range genreAliases[slug] {
			if g == a {
				return true
			}
		}
	}
	return false
}

// genreCharts keeps the per-genre charts read from the API.
type genreCharts struct {
	mu     sync.Mutex
	ids    map[string]appleGenres // by storefront
	charts map[string]genreChart  // by storefront|slug
}

type appleGenres struct {
	byName map[string]string
	at     time.Time
}

type genreChart struct {
	albums, songs []Entry
	at            time.Time
}

// Kept lifetimes of a storefront's genre list and of a genre chart.
const (
	genreListFor  = 7 * 24 * time.Hour
	genreChartFor = 24 * time.Hour
)

// appleGenreID answers the Apple genre id a genre page stands for in a
// storefront ("" when the storefront has none of its names).
func (s *Service) appleGenreID(ctx context.Context, st, slug string) (string, error) {
	g := &s.genres
	g.mu.Lock()
	list, ok := g.ids[st]
	g.mu.Unlock()
	if !ok || time.Since(list.at) >= genreListFor {
		var d struct {
			Data []struct {
				ID         string `json:"id"`
				Attributes struct {
					Name string `json:"name"`
				} `json:"attributes"`
			} `json:"data"`
		}
		if _, err := s.opts.Apple(ctx, "/v1/catalog/"+url.PathEscape(st)+"/genres", url.Values{"limit": {"100"}}, &d); err != nil {
			return "", err
		}
		list = appleGenres{byName: map[string]string{}, at: time.Now()}
		for _, x := range d.Data {
			list.byName[x.Attributes.Name] = x.ID
		}
		g.mu.Lock()
		if g.ids == nil {
			g.ids = map[string]appleGenres{}
		}
		g.ids[st] = list
		g.mu.Unlock()
	}
	for _, name := range genreAliases[slug] {
		if id := list.byName[name]; id != "" {
			return id, nil
		}
	}
	return "", nil
}

// appleGenreChart answers a storefront's chart for one genre page from the
// API (kept a day); false when Apple is off, has no such genre or failed.
func (s *Service) appleGenreChart(ctx context.Context, st, slug string) (albums, songs []Entry, ok bool) {
	if s.opts.Apple == nil {
		return nil, nil, false
	}
	key := st + "|" + slug
	g := &s.genres
	g.mu.Lock()
	c, have := g.charts[key]
	g.mu.Unlock()
	if have && time.Since(c.at) < genreChartFor {
		return c.albums, c.songs, len(c.albums)+len(c.songs) > 0
	}
	id, err := s.appleGenreID(ctx, st, slug)
	if err != nil || id == "" {
		return nil, nil, false
	}
	albums, songs, found, err := readAppleChart(ctx, s.opts.Apple, st, id, 1)
	if err != nil {
		return nil, nil, false
	}
	g.mu.Lock()
	if g.charts == nil || len(g.charts) > 2000 {
		g.charts = map[string]genreChart{}
	}
	g.charts[key] = genreChart{albums: albums, songs: songs, at: time.Now()}
	g.mu.Unlock()
	return albums, songs, found
}

var (
	// appleArtistURL is an Apple Music artist link; its last part the id.
	appleArtistURL = regexp.MustCompile(`^https://music\.apple\.com/[a-z]{2}/artist/(?:[^/?#]+/)?([0-9]{1,20})$`)
	barcodeRe      = regexp.MustCompile(`^[0-9]{8,14}$`)
	// coverFile is an Apple artwork file named by the album's barcode.
	coverFile = regexp.MustCompile(`/([0-9]{8,14})\.(?:jpg|jpeg|png|tif)(?:/|$)`)
)

// coverBarcode reads a barcode from an Apple artwork address (its file is
// usually named by the album's UPC); "" when it is not.
func coverBarcode(u string) string {
	if m := coverFile.FindStringSubmatch(u); m != nil {
		return m[1]
	}
	return ""
}

// EntryBarcode answers the album barcode a chart row carries: Apple's upc,
// else the cover's file name.
func EntryBarcode(e Entry) string {
	if barcodeRe.MatchString(e.UPC) {
		return e.UPC
	}
	return coverBarcode(e.Cover)
}
