package musictrend

import (
	"context"
	"encoding/json"
	"github.com/CoderLuii/holyarrs-metadata/internal/srctag"
	"hash/fnv"
	"log"
	"math"
	"sort"
	"strings"
	"time"
)

// Chart reads for the app's Music rows: region charts with popularity (P)
// and freshness (F), a genre list for the tiles, a release calendar and a
// suggest index. A region chart is the list source's when it has one,
// else Apple Music's; a genre chart is Apple Music's chart for that genre
// in the region's storefront, else the region's Apple chart filtered by
// the items' own Apple genres. The genre tiles are the list source's
// Browse genre pages.

// RankDays is how many days of daily chart ranks are kept (Luis,
// 2026-10-22): enough for a 7-day rank gain.
const RankDays = 7

// GenreHues is the number of genre colours the app knows.
const GenreHues = 24

func openCharts(s *Service) error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS chart_days (day TEXT NOT NULL, store TEXT NOT NULL, body BLOB NOT NULL, PRIMARY KEY(day, store)) WITHOUT ROWID`)
	if err != nil {
		return err
	}
	// A daily chart row is an Apple Music storefront chart; its day is when it was fetched.
	if err := srctag.Stamp(s.db, srctag.Spec{Table: "chart_days", Key: []string{"day", "store"}, Body: "body", Src: "'applemusic'", Known: "CAST(strftime('%s', day) AS INTEGER)"}); err != nil {
		return err
	}
	// The newest day of each storefront, so charts answer straight after
	// a restart.
	rs, err := s.db.Query(`SELECT store, body FROM chart_days c WHERE day = (SELECT max(day) FROM chart_days WHERE store = c.store)`)
	if err != nil {
		return err
	}
	defer rs.Close()
	s.amu.Lock()
	defer s.amu.Unlock()
	for rs.Next() {
		var st string
		var b []byte
		if rs.Scan(&st, &b) != nil {
			continue
		}
		var d dayBody
		if json.Unmarshal(b, &d) == nil && len(d.Albums) > 0 {
			// Days kept before titles lost Apple's EP/Single tag.
			for _, l := range [][]Entry{d.Albums, d.Songs} {
				for i := range l {
					if t, typ := SplitReleaseTag(l[i].Name); typ != "" {
						l[i].Name, l[i].Type = t, typ
					}
				}
			}
			s.apple[st] = chart{artists: RankArtists(d.Songs, d.Albums), albums: d.Albums, songs: d.Songs}
		}
	}
	return rs.Err()
}

type dayBody struct {
	Albums []Entry `json:"albums"`
	Songs  []Entry `json:"songs"`
}

// keepRanks stores today's chart of one storefront and drops days older
// than RankDays (plus today).
func (s *Service) keepRanks(st string, c chart, now time.Time) {
	b, err := json.Marshal(dayBody{Albums: c.albums, Songs: c.songs})
	if err != nil {
		return
	}
	day := now.UTC().Format("2006-01-02")
	_, _ = s.db.Exec(`INSERT OR REPLACE INTO chart_days (day, store, body) VALUES (?, ?, ?)`, day, st, b)
	_, _ = s.db.Exec(`DELETE FROM chart_days WHERE day < ?`, now.UTC().AddDate(0, 0, -RankDays).Format("2006-01-02"))
}

// rankAgo answers each item's rank (by norm key) RankDays ago, or false
// when that day is not kept.
func (s *Service) rankAgo(st, kind string, now time.Time) (map[string]int, bool) {
	var b []byte
	day := now.UTC().AddDate(0, 0, -RankDays).Format("2006-01-02")
	if s.db.QueryRow(`SELECT body FROM chart_days WHERE day = ? AND store = ?`, day, st).Scan(&b) != nil {
		return nil, false
	}
	var d dayBody
	if json.Unmarshal(b, &d) != nil {
		return nil, false
	}
	list := d.Albums
	if kind == "songs" {
		list = d.Songs
	}
	out := map[string]int{}
	for i, e := range list {
		out[entryKey(e)] = i + 1
	}
	return out, true
}

func entryKey(e Entry) string { return norm(e.Artist) + "|" + norm(baseTitle(e.Name)) }

// Popularity is P for a charted item: 1 − (rank − 1)/L, L ≥ 1.
func Popularity(rank, length int) float64 {
	if length < 1 || rank < 1 {
		return 0
	}
	return 1 - float64(rank-1)/float64(length)
}

// Freshness is F_t: exp(−d/t), d whole days since release, 0 for a future
// date, unknown date → 0.
func Freshness(date string, t float64, now time.Time) float64 {
	if len(date) < 10 || t <= 0 {
		return 0
	}
	d, err := time.Parse("2006-01-02", date[:10])
	if err != nil {
		return 0
	}
	days := math.Floor(now.UTC().Sub(d).Hours() / 24)
	if days < 0 {
		days = 0
	}
	return math.Exp(-days / t)
}

// RankGain is ΔR: clamp((rank_7d_ago − rank_today)/L, 0, 1); an item not
// charted then counts as L + 1.
func RankGain(ago, today, length int) float64 {
	if length < 1 {
		return 0
	}
	if ago <= 0 {
		ago = length + 1
	}
	return math.Max(0, math.Min(1, float64(ago-today)/float64(length)))
}

// GenreSlug is a genre's URL name.
func GenreSlug(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(fold.Replace(strings.ToLower(name))) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case !dash && b.Len() > 0:
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}

// GenreHue is a genre's colour index, 0..GenreHues-1, stable by slug.
func GenreHue(slug string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(slug))
	return int(h.Sum32() % GenreHues)
}

// ChartItem is one album or song of /v1/music/charts.
type ChartItem struct {
	Rank       int    `json:"rank"`
	ChartRank  int    `json:"chart_rank"` // rank in the whole region chart
	MBID       string `json:"mbid,omitempty"`
	ArtistMBID string `json:"artist_mbid,omitempty"`
	// Unlinked: the artist is shown as plain text (a shared name without
	// exact evidence); nothing links it again.
	Unlinked bool   `json:"-"`
	Title    string `json:"title"`
	Artist   string `json:"artist_name"`
	Date     string `json:"release_date,omitempty"`
	Explicit bool   `json:"explicit"`
	// ReleaseType is "ep" or "single" when the source's title said so.
	ReleaseType string `json:"release_type,omitempty"`
	// PreOrder marks a release dated after today (release_date says when).
	PreOrder   bool              `json:"pre_order,omitempty"`
	Genres     []string          `json:"genres"`
	Cover      string            `json:"cover,omitempty"`
	CoverSizes map[string]string `json:"cover_sizes,omitempty"`
	P          float64           `json:"p"`
	F7         float64           `json:"f7"`
	F14        float64           `json:"f14"`
	F60        float64           `json:"f60"`
	// RankAgo is the rank RankDays ago (0: not charted then); RankGain is
	// ΔR. Both are left out until RankDays of ranks are kept.
	RankAgo  *int     `json:"rank_7d_ago,omitempty"`
	RankGain *float64 `json:"rank_gain,omitempty"`
	// Source is where the item came from (SrcLists, SrcApple or
	// SrcListenBrainz); GenreSource where its genre page came from
	// (SrcDump: the slim copy's tags) on a genre page.
	Source      string `json:"source"`
	GenreSource string `json:"genre_source,omitempty"`
}

func storeOf(region string) string {
	if region == "" || strings.EqualFold(region, "global") {
		return "us"
	}
	return strings.ToLower(region)
}

// Charts answers a region's albums or songs chart, optionally one genre
// (by slug), with P computed against that list's own length. The source
// order is SourceOrder: the list source's chart, else Apple Music's, else
// ListenBrainz's sitewide one. A genre page is ListenBrainz's sitewide
// lists sorted by the slim copy's tags (the list source has no ranked
// genre chart).
func (s *Service) Charts(ctx context.Context, region, kind, genre string, now time.Time) ([]ChartItem, bool) {
	if genre != "" {
		items, ok := s.lbChart(ctx, kind, genre, now)
		return s.finish(ctx, kind, items, now), ok
	}
	if items, ok := s.listChart(ctx, region, kind, now); ok {
		t := time.Now()
		items = s.finish(ctx, kind, items, now)
		if d := time.Since(t); d > 2*time.Second {
			log.Printf("musictrend: chart %s %s finish slow: %s (%d rows)", region, kind, d.Round(time.Millisecond), len(items))
		}
		return items, true
	}
	if items, ok := s.appleChart(ctx, region, kind, "", now); ok && len(items) > 0 {
		return s.finish(ctx, kind, items, now), true
	}
	items, ok := s.lbChart(ctx, kind, "", now)
	return s.finish(ctx, kind, items, now), ok
}

// AppleCharts answers a region's kept Apple Music chart (all genres), for
// the catalog's chart rows.
func (s *Service) AppleCharts(ctx context.Context, region, kind string, now time.Time) ([]ChartItem, bool) {
	return s.appleChart(ctx, region, kind, "", now)
}

// appleChart answers a region's kept Apple Music chart, optionally only
// the items of one genre page, with rank history.
func (s *Service) appleChart(ctx context.Context, region, kind, genre string, now time.Time) ([]ChartItem, bool) {
	st := storeOf(region)
	s.amu.RLock()
	c, ok := s.apple[st]
	s.amu.RUnlock()
	if !ok {
		return nil, false
	}
	list := c.albums
	if kind == "songs" {
		list = c.songs
	}
	return s.chartFrom(ctx, st, kind, genre, list, true, now), true
}

// chartFrom makes chart items from Apple entries: the genre page's only
// when genre is set, with the rank RankDays ago when history is kept.
func (s *Service) chartFrom(ctx context.Context, st, kind, genre string, list []Entry, history bool, now time.Time) []ChartItem {
	type pick struct {
		e    Entry
		rank int
	}
	var picks []pick
	for i, e := range list {
		if genre != "" && !inGenre(e, genre) {
			continue
		}
		picks = append(picks, pick{e, i + 1})
	}
	var ago map[string]int
	haveAgo := false
	if history {
		ago, haveAgo = s.rankAgo(st, kind, now)
	}
	agoRank := map[string]int{}
	if haveAgo && genre != "" {
		// Rank within the genre list RankDays ago.
		var keys []string
		for k, r := range ago {
			_ = r
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return ago[keys[i]] < ago[keys[j]] })
		inGenre := map[string]bool{}
		for _, p := range picks {
			inGenre[entryKey(p.e)] = true
		}
		n := 0
		for _, k := range keys {
			if inGenre[k] {
				n++
				agoRank[k] = n
			}
		}
	} else {
		agoRank = ago
	}
	L := len(picks)
	out := make([]ChartItem, 0, L)
	for i, p := range picks {
		it := ChartItem{Rank: i + 1, ChartRank: p.rank, Title: p.e.Name, Artist: p.e.Artist, Date: p.e.Date, Explicit: p.e.Explicit, ReleaseType: p.e.Type, Genres: p.e.Genres, Cover: p.e.Cover, CoverSizes: CoverVariants(p.e.Cover), Source: SrcApple}
		if it.Genres == nil {
			it.Genres = []string{}
		}
		if kind != "songs" {
			if m, ok, _ := s.matcher.Known(ctx, "album", p.e.Name, p.e.Artist); ok {
				it.MBID, it.ArtistMBID = m.MBID, m.ArtistMBID
			}
		} else if m, ok, _ := s.matcher.Known(ctx, "artist", p.e.Artist, ""); ok {
			it.ArtistMBID = m.MBID
		}
		// The row's own artist id (or its album's barcode), resolved
		// exactly, beats the name match.
		if s.opts.ExactArtist != nil {
			var codes []string
			if c := EntryBarcode(p.e); c != "" {
				codes = []string{c}
			}
			x := s.opts.ExactArtist(ctx, "applemusic", p.e.AppleArtist, p.e.Artist, codes)
			if x == "" && s.shared(ctx, p.e.Artist) {
				it.ArtistMBID, it.Unlinked = "", true
			}
			if x != "" && x != it.ArtistMBID {
				it.ArtistMBID = x
				if kind != "songs" {
					it.MBID = "" // found again under the right artist (finish)
				}
			}
		}
		it.P = Popularity(it.Rank, L)
		it.F7, it.F14, it.F60 = Freshness(p.e.Date, 7, now), Freshness(p.e.Date, 14, now), Freshness(p.e.Date, 60, now)
		if haveAgo {
			r := agoRank[entryKey(p.e)]
			g := RankGain(r, it.Rank, L)
			it.RankAgo, it.RankGain = &r, &g
		}
		out = append(out, it)
	}
	return out
}

// Genre is one genre tile of /v1/music/genres.
type Genre struct {
	Slug  string `json:"slug"`
	Name  string `json:"name"`
	Hue   int    `json:"hue"`
	Count int    `json:"count"`
	// Art is the tile picture; empty means the app paints the flat hue.
	Art string `json:"art"`
	// Arts is the slideshow, 1 to 4 artist photos (Art is the first).
	Arts []string `json:"arts,omitempty"`
	// Artists are the page's artists for the tile art (MusicBrainz ids).
	Artists []NamedArtist `json:"-"`
}

// Genres lists the list source's Browse genre pages that hold items, the
// same for every region (the pages are read for the US).
//
// Only pages whose genre chart (slim tags plus ListenBrainz) holds items
// are listed; without the list source, those pages by slug.
func (s *Service) Genres(region string) ([]Genre, bool) {
	lb := s.lbGenres()
	has := map[string]bool{}
	for _, g := range lb {
		has[g.Slug] = true
	}
	var tiles []GenreTile
	if s.opts.Lists != nil {
		tiles = s.opts.Lists.GenreTiles()
	}
	if len(tiles) == 0 {
		sort.Slice(lb, func(i, j int) bool { return lb[i].Count > lb[j].Count })
		return lb, len(lb) > 0
	}
	out := make([]Genre, 0, len(tiles))
	for _, t := range tiles {
		if !has[t.Slug] {
			continue
		}
		out = append(out, Genre{Slug: t.Slug, Name: t.Name, Hue: GenreHue(t.Slug), Count: t.Count, Artists: t.Artists})
	}
	return out, len(out) > 0
}

// Suggestion is one row of /v1/music/suggest.
type Suggestion struct {
	Kind       string  `json:"kind"` // artist, album, song
	Name       string  `json:"name"`
	Artist     string  `json:"artist_name,omitempty"`
	MBID       string  `json:"mbid,omitempty"`
	ArtistMBID string  `json:"artist_mbid,omitempty"`
	Cover      string  `json:"cover,omitempty"`
	P          float64 `json:"p"`
	Score      float64 `json:"score"`
}

// Suggest matches q against every kept chart (all storefronts): names that
// start with q first, then words that start with it, then any match;
// popularity breaks ties. At most limit rows and 3 of a kind per 10.
func (s *Service) Suggest(ctx context.Context, q string, limit int) []Suggestion {
	nq := norm(q)
	if len([]rune(nq)) < 2 {
		return []Suggestion{}
	}
	s.amu.RLock()
	charts := make([]chart, 0, len(s.apple))
	stores := make([]string, 0, len(s.apple))
	for st := range s.apple {
		stores = append(stores, st)
	}
	sort.Strings(stores)
	for _, st := range stores {
		charts = append(charts, s.apple[st])
	}
	s.amu.RUnlock()
	text := func(name string) float64 {
		n := norm(name)
		switch {
		case strings.HasPrefix(n, nq):
			return 1
		case strings.Contains(" "+strings.ToLower(fold.Replace(strings.ToLower(name))), " "+strings.ToLower(q)):
			return 0.8
		case strings.Contains(n, nq):
			return 0.5
		}
		return 0
	}
	best := map[string]Suggestion{}
	add := func(kind string, e Entry, name string, rank, length int) {
		t := text(name)
		if t == 0 {
			return
		}
		p := Popularity(rank, length)
		sg := Suggestion{Kind: kind, Name: name, Artist: e.Artist, Cover: e.Cover, P: p, Score: 0.7*t + 0.3*p}
		if kind == "artist" {
			sg.Artist, sg.Cover = "", ""
		}
		k := kind + "|" + norm(name) + "|" + norm(sg.Artist)
		if old, ok := best[k]; ok && old.Score >= sg.Score {
			return
		}
		best[k] = sg
	}
	for _, c := range charts {
		for i, e := range c.albums {
			add("album", e, e.Name, i+1, len(c.albums))
		}
		for i, e := range c.songs {
			add("song", e, e.Name, i+1, len(c.songs))
		}
		for i, e := range c.artists {
			add("artist", Entry{Artist: e.Name}, e.Name, i+1, len(c.artists))
		}
	}
	list := make([]Suggestion, 0, len(best))
	for _, v := range best {
		list = append(list, v)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Score != list[j].Score {
			return list[i].Score > list[j].Score
		}
		if list[i].Kind != list[j].Kind {
			return list[i].Kind < list[j].Kind
		}
		return list[i].Name < list[j].Name
	})
	out := []Suggestion{}
	per := map[string]int{}
	for _, v := range list {
		if len(out) >= limit {
			break
		}
		if per[v.Kind] >= 3 {
			continue
		}
		switch v.Kind {
		case "album":
			if m, ok, _ := s.matcher.Known(ctx, "album", v.Name, v.Artist); ok {
				v.MBID, v.ArtistMBID = m.MBID, m.ArtistMBID
			}
		case "artist":
			if m, ok, _ := s.matcher.Known(ctx, "artist", v.Name, ""); ok {
				v.MBID, v.ArtistMBID = m.MBID, m.MBID
			}
		case "song":
			if m, ok, _ := s.matcher.Known(ctx, "artist", v.Artist, ""); ok {
				v.ArtistMBID = m.MBID
			}
		}
		per[v.Kind]++
		out = append(out, v)
	}
	return out
}

// Releases answers the new-music windows' albums (ListenBrainz fresh
// releases matched to MusicBrainz, plus Deezer) dated from..to, by any of
// the artists when artists is not empty, each with F14. The calendar is
// built by the daily new-music job, so no MusicBrainz call is made here.
func (n *NewMusic) Releases(from, to string, artists map[string]bool, now time.Time) []ReleaseItem {
	out := []ReleaseItem{}
	seen := map[string]bool{}
	for _, w := range []string{WindowRecent, WindowWeek, WindowUpcoming} {
		body, _, ok := n.Window(w)
		if !ok {
			continue
		}
		var d struct {
			Items []NewItem `json:"items"`
		}
		if json.Unmarshal(body, &d) != nil {
			continue
		}
		for _, it := range d.Items {
			day := it.ReleaseDate
			if len(day) >= 10 {
				day = day[:10]
			}
			if it.MBID == "" || seen[it.MBID] || day == "" || day < from || day > to {
				continue
			}
			if len(artists) > 0 && !artists[strings.ToLower(it.ArtistMBID)] {
				continue
			}
			seen[it.MBID] = true
			out = append(out, ReleaseItem{NewItem: it, F14: Freshness(day, 14, now)})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ReleaseDate != out[j].ReleaseDate {
			return out[i].ReleaseDate > out[j].ReleaseDate
		}
		return out[i].MBID < out[j].MBID
	})
	return out
}

// ReleaseItem is one album of /v1/music/releases.
type ReleaseItem struct {
	NewItem
	F14 float64 `json:"f14"`
	// SecondaryTypes and Tags come with a release read from an artist's
	// stored discography (Tags: the artist's genres).
	SecondaryTypes []string `json:"secondary_types,omitempty"`
	Tags           []string `json:"tags,omitempty"`
}
