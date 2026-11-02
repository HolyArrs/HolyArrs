package musictrend

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/CoderLuii/holyarrs-metadata/internal/srctag"
	"io"
	"strings"
	"sync"
	"time"
)

// One source order for every chart, new releases included (music data
// plan S5, AC 11): the paid list source (spotify81) first, Apple Music
// when it has nothing, ListenBrainz as the backup. Genre pages are
// ListenBrainz's sitewide lists sorted into genres by the slim
// MusicBrainz copy's tags (AC 12): the list source has no ranked genre
// chart. Every item carries the source it came from.

// Source tags (srctag names).
const (
	SrcLists        = "spotify81"
	SrcApple        = "applemusic"
	SrcListenBrainz = "listenbrainz"
	SrcDump         = "mb-dump"
)

// SourceOrder is the order every chart is read in.
var SourceOrder = []string{SrcLists, SrcApple, SrcListenBrainz}

// listsSchema is the stored list body version; bodies stored before the
// source tags (older versions) are not served again.
const listsSchema = 2

// lbPages is how many 100-item pages of each ListenBrainz sitewide list
// are read per refresh (free and keyless; one refresh every Every).
const lbPages = 5

// lbCharts are ListenBrainz's sitewide lists of the week, kept as the
// backup chart and the genre pages' pool.
type lbCharts struct {
	mu      sync.RWMutex
	artists []Entry
	albums  []Entry
	songs   []Entry
	at      time.Time
	// genres are the albums and songs per genre page slug (slim tags).
	genres map[string]genrePool
}

type genrePool struct{ albums, songs []Entry }

// ParseLBRecordings reads the ListenBrainz sitewide top recordings answer
// (songs: Name is the track, ArtistMBID the first credited artist, MBID
// the album's release group when given).
func ParseLBRecordings(r io.Reader) ([]Entry, error) {
	var body struct {
		Payload struct {
			Recordings []struct {
				Track       string   `json:"track_name"`
				Artist      string   `json:"artist_name"`
				ArtistMBIDs []string `json:"artist_mbids"`
				Recording   string   `json:"recording_mbid"`
				CAARelease  string   `json:"caa_release_mbid"`
				Listens     int64    `json:"listen_count"`
			} `json:"recordings"`
		} `json:"payload"`
	}
	if err := json.NewDecoder(r).Decode(&body); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []Entry
	for _, g := range body.Payload.Recordings {
		k := norm(g.Artist) + "|" + norm(g.Track)
		if g.Track == "" || g.Artist == "" || seen[k] {
			continue
		}
		seen[k] = true
		e := Entry{Name: g.Track, Artist: g.Artist, Listens: g.Listens}
		if len(g.ArtistMBIDs) > 0 && validMBID(strings.ToLower(g.ArtistMBIDs[0])) {
			e.ArtistMBID = strings.ToLower(g.ArtistMBIDs[0])
		}
		if validMBID(strings.ToLower(g.CAARelease)) {
			e.CAARelease = strings.ToLower(g.CAARelease)
		}
		out = append(out, e)
	}
	if len(out) == 0 {
		return nil, errors.New("musictrend: no ListenBrainz recordings")
	}
	return out, nil
}

// refreshLB reads the week's sitewide artists, release groups and
// recordings (lbPages pages each) and sorts them into genre pages. A list
// that fails keeps its last copy.
func (s *Service) refreshLB(ctx context.Context) error {
	if s.opts.ListenBrainzURL == "" {
		return nil
	}
	s.lb.mu.RLock()
	fresh := !s.lb.at.IsZero() && time.Since(s.lb.at) < s.opts.Every
	sorted := s.lb.genres != nil || s.opts.Tags == nil
	keptAlbums, keptSongs := s.lb.albums, s.lb.songs
	s.lb.mu.RUnlock()
	if fresh {
		if !sorted {
			g := s.sortGenres(ctx, keptAlbums, keptSongs)
			s.lb.mu.Lock()
			s.lb.genres = g
			s.lb.mu.Unlock()
		}
		return nil
	}
	var errs error
	read := func(path string, parse func(io.Reader) ([]Entry, error)) []Entry {
		var out []Entry
		for p := 0; p < lbPages; p++ {
			var page []Entry
			err := s.get(ctx, s.lbRoot()+path+"&count=100&offset="+itoa(p*100), func(r io.Reader) (err error) {
				page, err = parse(r)
				return err
			})
			if err != nil {
				if p == 0 {
					errs = errors.Join(errs, err)
				}
				break
			}
			out = append(out, page...)
		}
		return out
	}
	artists := read("/stats/sitewide/artists?range=week", func(r io.Reader) ([]Entry, error) { e, _, err := ParseLBArtists(r); return e, err })
	albums := read("/stats/sitewide/release-groups?range=week", func(r io.Reader) ([]Entry, error) { e, _, err := ParseLBAlbums(r); return e, err })
	songs := read("/stats/sitewide/recordings?range=week", ParseLBRecordings)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	genres := s.sortGenres(ctx, albums, songs)
	s.lb.mu.Lock()
	if len(artists) > 0 {
		s.lb.artists = artists
	}
	if len(albums) > 0 {
		s.lb.albums = albums
	}
	if len(songs) > 0 {
		s.lb.songs = songs
	}
	if len(genres) > 0 {
		s.lb.genres = genres
	}
	if errs == nil {
		s.lb.at = time.Now()
	}
	s.lb.mu.Unlock()
	s.saveLB()
	if errs != nil {
		s.opts.Report(SrcListenBrainz, "fetch_failed", 1)
	}
	return errs
}

// saveLB keeps the ListenBrainz lists across restarts.
func (s *Service) saveLB() {
	s.lb.mu.RLock()
	b, err := json.Marshal(map[string]any{"artists": s.lb.artists, "albums": s.lb.albums, "songs": s.lb.songs, "at": s.lb.at.Unix()})
	s.lb.mu.RUnlock()
	if err == nil {
		_, _ = s.db.Exec(`INSERT OR REPLACE INTO lb_charts (id, body) VALUES (1, ?)`, b)
	}
}

// loadLB reads the kept ListenBrainz lists (genres are sorted again).
func (s *Service) loadLB() {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS lb_charts (id INTEGER PRIMARY KEY, body BLOB NOT NULL)`); err != nil {
		return
	}
	_ = srctag.Stamp(s.db, srctag.Spec{Table: "lb_charts", Key: []string{"id"}, Body: "body", Src: "'listenbrainz'",
		Known: "COALESCE(json_extract(CAST(@.body AS TEXT),'$.at'),0)"})
	// A row stamped before with no fetch time takes the one its body
	// carries.
	_, _ = s.db.Exec(`UPDATE lb_charts SET fetched_at=COALESCE(json_extract(CAST(body AS TEXT),'$.at'),0) WHERE fetched_at=0`)
	var b []byte
	if s.db.QueryRow(`SELECT body FROM lb_charts WHERE id = 1`).Scan(&b) != nil {
		return
	}
	var kept struct {
		Artists, Albums, Songs []Entry
		At                     int64
	}
	if json.Unmarshal(b, &kept) != nil {
		return
	}
	s.lb.artists, s.lb.albums, s.lb.songs = kept.Artists, kept.Albums, kept.Songs
	s.lb.at = time.Unix(kept.At, 0)
}

// genreWords maps a genre page slug to words found in MusicBrainz genre
// and tag names (lower case, matched as a part of the name).
var genreWords = map[string][]string{
	"pop": {"pop"}, "latin": {"latin", "reggaeton", "bachata", "salsa", "cumbia", "regional mexican", "corrido", "urbano"},
	"rock": {"rock"}, "country": {"country"}, "dance": {"dance", "electronic", "house", "techno", "edm", "trance", "dubstep", "drum and bass"},
	"rnb": {"r&b", "rnb", "rhythm and blues"}, "indie": {"indie"}, "kpop": {"k-pop", "kpop"}, "metal": {"metal"},
	"jazz": {"jazz"}, "alternative": {"alternative"}, "folk": {"folk", "acoustic", "singer-songwriter"}, "soul": {"soul"},
	"classical": {"classical", "baroque", "opera", "orchestral"}, "punk": {"punk"}, "blues": {"blues"},
	"funk": {"funk", "disco"}, "ambient": {"ambient"}, "afro": {"afrobeat", "afropop", "afro-pop", "amapiano", "afro"},
	"brazil":  {"mpb", "sertanejo", "samba", "bossa nova", "pagode", "brazilian", "funk carioca"},
	"hip-hop": {"hip hop", "hip-hop", "rap", "trap"},
}

// GenreSlugsOf answers the genre pages a set of MusicBrainz genre or tag
// names stands for.
func GenreSlugsOf(tags []string) []string {
	var out []string
	for slug, words := range genreWords {
		hit := false
		for _, t := range tags {
			t = strings.ToLower(t)
			for _, w := range words {
				if strings.Contains(t, w) {
					hit = true
				}
			}
		}
		if hit {
			out = append(out, slug)
		}
	}
	return out
}

// sortGenres puts each album and song on its genre pages by the slim
// copy's tags: the release group's own, else its artist's.
func (s *Service) sortGenres(ctx context.Context, albums, songs []Entry) map[string]genrePool {
	if s.opts.Tags == nil {
		return nil
	}
	artistTags := map[string][]string{}
	tagsOf := func(kind, id string) []string {
		if id == "" {
			return nil
		}
		if kind == "artist" {
			if t, ok := artistTags[id]; ok {
				return t
			}
		}
		t := s.opts.Tags(ctx, kind, id)
		if kind == "artist" {
			artistTags[id] = t
		}
		return t
	}
	out := map[string]genrePool{}
	for _, e := range albums {
		t := tagsOf("release-group", e.MBID)
		if len(t) == 0 {
			t = tagsOf("artist", e.ArtistMBID)
		}
		for _, g := range GenreSlugsOf(t) {
			p := out[g]
			p.albums = append(p.albums, e)
			out[g] = p
		}
	}
	for _, e := range songs {
		for _, g := range GenreSlugsOf(tagsOf("artist", e.ArtistMBID)) {
			p := out[g]
			p.songs = append(p.songs, e)
			out[g] = p
		}
	}
	return out
}

// lbChart answers ListenBrainz's sitewide albums or songs chart (the
// same for every region), or one genre page of it.
func (s *Service) lbChart(ctx context.Context, kind, genre string, now time.Time) ([]ChartItem, bool) {
	s.lb.mu.RLock()
	list := s.lb.albums
	if kind == "songs" {
		list = s.lb.songs
	}
	if genre != "" {
		p := s.lb.genres[genre]
		list = p.albums
		if kind == "songs" {
			list = p.songs
		}
	}
	s.lb.mu.RUnlock()
	if len(list) == 0 {
		return nil, false
	}
	if len(list) > 100 {
		list = list[:100]
	}
	L := len(list)
	out := make([]ChartItem, 0, L)
	for i, e := range list {
		it := ChartItem{Rank: i + 1, ChartRank: i + 1, MBID: e.MBID, ArtistMBID: e.ArtistMBID, Title: e.Name, Artist: e.Artist, Genres: []string{}, Source: SrcListenBrainz}
		if kind == "songs" {
			it.MBID = ""
		}
		if genre != "" {
			it.GenreSource = SrcDump
		}
		it.P = Popularity(it.Rank, L)
		it.F7, it.F14, it.F60 = Freshness(e.Date, 7, now), Freshness(e.Date, 14, now), Freshness(e.Date, 60, now)
		out = append(out, it)
	}
	return out, true
}

// lbGenres answers the genre pages that hold ListenBrainz items.
func (s *Service) lbGenres() []Genre {
	s.lb.mu.RLock()
	defer s.lb.mu.RUnlock()
	var out []Genre
	for slug, p := range s.lb.genres {
		if n := len(p.albums) + len(p.songs); n > 0 {
			var artists []NamedArtist
			seen := map[string]bool{}
			for _, e := range p.albums {
				if e.ArtistMBID != "" && !seen[e.ArtistMBID] && len(artists) < 8 {
					seen[e.ArtistMBID] = true
					artists = append(artists, NamedArtist{ID: e.ArtistMBID, Name: e.Artist})
				}
			}
			out = append(out, Genre{Slug: slug, Name: slug, Hue: GenreHue(slug), Count: n, Artists: artists})
		}
	}
	return out
}

// AllChartArtists lists every artist the charts show now: the trending
// lists, every region's albums and songs charts, and the genre pages
// (stored data only, no call). New releases are added by the caller.
func (s *Service) AllChartArtists(ctx context.Context) []string {
	out := s.ChartArtists()
	regions := append([]string{"global"}, s.listRegions()...)
	for _, st := range Storefronts() {
		regions = append(regions, st)
	}
	seen := map[string]bool{}
	for _, r := range regions {
		if seen[r] {
			continue
		}
		seen[r] = true
		for _, kind := range []string{"albums", "songs"} {
			items, _ := s.Charts(ctx, r, kind, "", time.Now())
			for _, it := range items {
				out = append(out, it.ArtistMBID)
			}
		}
	}
	for _, g := range s.lbGenres() {
		for _, kind := range []string{"albums", "songs"} {
			items, _ := s.lbChart(ctx, kind, g.Slug, time.Now())
			for _, it := range items {
				out = append(out, it.ArtistMBID)
			}
		}
	}
	return out
}
