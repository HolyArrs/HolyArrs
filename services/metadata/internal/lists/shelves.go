package lists

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// Item is one shelf member: an album (release group) or an artist, with
// its chart signals. Every id is a MusicBrainz id.
type Item struct {
	Kind       string  `json:"kind"` // album or artist
	MBID       string  `json:"mbid"`
	ArtistMBID string  `json:"artist_mbid,omitempty"`
	Title      string  `json:"title,omitempty"`
	Artist     string  `json:"artist_name"`
	Rank       int     `json:"rank"`
	Prev       int     `json:"prev,omitempty"`
	Peak       int     `json:"peak,omitempty"`
	Days       int     `json:"days,omitempty"`
	Streams    int64   `json:"streams,omitempty"`
	Date       string  `json:"release_date,omitempty"`
	Song       string  `json:"song,omitempty"` // the charted song behind the card
	P          float64 `json:"p"`
	M          float64 `json:"m,omitempty"`
}

func (it Item) members() [][2]string {
	if it.Kind == "artist" {
		return [][2]string{{"artist", it.MBID}}
	}
	out := [][2]string{{"release-group", it.MBID}}
	if it.ArtistMBID != "" {
		out = append(out, [2]string{"artist", it.ArtistMBID})
	}
	return out
}

// Shelves is one region's building blocks.
type Shelves struct {
	Region string `json:"region"`
	// Chart is the region whose song chart fed top_now ("global" when the
	// region has none).
	Chart    string `json:"chart_region"`
	Neighbor string `json:"neighbor_region,omitempty"`
	// Season is the paid season the "tile:season" block holds.
	Season string            `json:"season,omitempty"`
	AsOf   string            `json:"as_of"`
	Blocks map[string][]Item `json:"blocks"`
}

// Empty reports a shelf set with no items.
func (s Shelves) Empty() bool {
	for _, l := range s.Blocks {
		if len(l) > 0 {
			return false
		}
	}
	return true
}

// blockOrder is every block name, in serving priority.
var blockOrder = []string{"top_now", "top_world", "albums_week", "albums_world", "artists_week", "climbing", "new_entries", "new_friday", "big_new_albums", "singles_artists", "long_runners", "number_ones", "comebacks", "neighbor", "video_artists"}

// blockNames is blockOrder then the tile blocks ("tile:pop"), sorted.
func blockNames(blocks map[string][]Item) []string {
	out := append([]string{}, blockOrder...)
	var tiles []string
	for k := range blocks {
		if strings.HasPrefix(k, "tile:") {
			tiles = append(tiles, k)
		}
	}
	sort.Strings(tiles)
	return append(out, tiles...)
}

// isAlbum says whether a list song sits on a real album (its album has
// another name), not on a single of its own.
func isAlbum(e Entry) bool {
	return e.Album != "" && !strings.EqualFold(strings.TrimSpace(e.Album), strings.TrimSpace(e.Title))
}

// Lookup answers a stored MusicBrainz match: kind "album" (name, artist)
// or "artist" (name, "").
type Lookup func(kind, name, artist string) (mbid, artistMBID string, ok bool)

// Inputs are the stored lists a build reads.
type Inputs struct {
	Now    time.Time
	AsOf   string
	Lookup Lookup
	Lists  map[string][]Entry // by source key
	Ago    map[string][]Entry // song charts 7 days ago, by source key
	// Date answers a release group's first release date ("" unknown).
	Date func(mbid string) string
	// Rematch is told of a song charting now that would have shown as an
	// album older than two years (from, to: what it shows as instead).
	Rematch func(song, artist, from, to string)
	// Fresh are ListenBrainz's fresh releases (already MusicBrainz ids),
	// most listened first; they fill the new-release shelf after the US
	// new-release list.
	Fresh []FreshAlbum
}

// FreshAlbum is one album of ListenBrainz's fresh releases.
type FreshAlbum struct {
	MBID, ArtistMBID, Title, Artist, Date string
	Listens                               int64
}

// neighbors is each region's closest big neighbour for "Top in {neighbor}".
var neighbors = map[string]string{"us": "ca", "ca": "us", "gb": "ie", "ie": "gb", "de": "at", "at": "de", "ch": "de", "fr": "be", "be": "fr", "nl": "be", "es": "mx", "mx": "us", "br": "ar", "ar": "br", "au": "nz", "nz": "au", "it": "es", "se": "no", "no": "se", "dk": "se", "fi": "se", "pt": "br", "co": "mx", "cl": "ar", "pe": "co", "jp": "kr", "kr": "jp", "in": "gb", "ph": "us", "id": "my", "my": "sg", "sg": "my", "pl": "de", "tr": "de", "za": "ng", "ng": "za", "global": "us"}

func (in Inputs) list(kind Kind, region string) ([]Entry, string) {
	if kind == KindNew {
		// One new-release list for every region; entries past NewDays are
		// dropped even when the stored list holds them.
		return FreshReleases(in.Lists["new:us"], in.Now), "us"
	}
	if es := in.Lists[string(kind)+":"+region]; len(es) > 0 {
		return es, region
	}
	return in.Lists[string(kind)+":global"], "global"
}

// NewDays is how old a new release may be (from its release date).
const NewDays = 21

// FreshReleases keeps the entries released within NewDays of now.
func FreshReleases(es []Entry, now time.Time) []Entry {
	out := make([]Entry, 0, len(es))
	for _, e := range es {
		if within(e.ReleaseDate, NewDays, now) && released(e.ReleaseDate, now) {
			out = append(out, e)
		}
	}
	return out
}

func clamp01(x float64) float64 { return math.Max(0, math.Min(1, x)) }

// pop is P' = 0.8·P + 0.2·streams lift, P = 1 − (rank−1)/L.
func pop(rank, length int, streams, maxStreams int64) float64 {
	if length < 1 || rank < 1 {
		return 0
	}
	p := 1 - float64(rank-1)/float64(length)
	if streams > 1 && maxStreams > 1 {
		p = 0.8*p + 0.2*clamp01(math.Log10(float64(streams))/math.Log10(float64(maxStreams)))
	}
	return math.Round(p*1000) / 1000
}

// momentum is M = clamp(0.6·ΔR7 + 0.4·ΔR1); new entries get 1 − rank/L.
func momentum(e Entry, ago int, length int) float64 {
	if length < 1 {
		return 0
	}
	L := float64(length)
	if e.Status == "new" {
		return math.Round(clamp01(1-float64(e.Rank)/L)*1000) / 1000
	}
	r7, r1 := float64(length+1), float64(length+1)
	if ago > 0 {
		r7 = float64(ago)
	}
	if e.Prev > 0 {
		r1 = float64(e.Prev)
	}
	if ago == 0 && e.Prev > 0 {
		r7 = r1 // fewer than 7 days kept: yesterday stands in
	}
	m := 0.6*(r7-float64(e.Rank))/L + 0.4*(r1-float64(e.Rank))/L
	return math.Round(clamp01(m)*1000) / 1000
}

func entryKey(e Entry) string { return strings.ToLower(e.Artist + "|" + e.Title) }

// released reports a date of today or before: a pre-order (dated later,
// even tomorrow) is never new.
func released(date string, now time.Time) bool {
	return len(date) >= 10 && date[:10] <= now.UTC().Format("2006-01-02")
}

func within(date string, days int, now time.Time) bool {
	if len(date) < 10 {
		return false
	}
	d, err := time.Parse("2006-01-02", date[:10])
	if err != nil {
		return false
	}
	age := now.UTC().Sub(d).Hours() / 24
	return age >= -1 && age <= float64(days)
}

type builder struct {
	in     Inputs
	images map[string]Entry
}

// album makes an album item from a chart or new-release entry: its title
// without edition junk and its first release date from MusicBrainz (the
// list's own date when MusicBrainz's is not known yet).
func (b builder) album(e Entry) (Item, bool) {
	id, art, ok := b.in.Lookup("album", e.Title, e.Artist)
	if t := cleanTitle(e.Title); !ok && t != e.Title {
		id, art, ok = b.in.Lookup("album", t, e.Artist)
	}
	if !ok {
		return Item{}, false
	}
	if art == "" {
		art, _, _ = b.in.Lookup("artist", e.Artist, "")
	}
	date := e.ReleaseDate
	if b.in.Date != nil {
		if d := b.in.Date(id); d != "" {
			date = d
		}
	}
	return Item{Kind: "album", MBID: id, ArtistMBID: art, Title: cleanTitle(e.Title), Artist: e.Artist, Rank: e.Rank, Prev: e.Prev, Peak: e.Peak, Days: e.Days, Streams: e.Streams, Date: date}, true
}

// single is a song's own single or EP release group, when matched.
func (b builder) single(e Entry) (Item, bool) {
	it, ok := b.album(Entry{Title: e.Title, Artist: e.Artist, ReleaseDate: e.ReleaseDate})
	if !ok {
		return Item{}, false
	}
	it.Rank, it.Prev, it.Peak, it.Days, it.Streams, it.Song = e.Rank, e.Prev, e.Peak, e.Days, e.Streams, cleanTitle(e.Title)
	return it, true
}

func (b builder) artist(e Entry) (Item, bool) {
	id, _, ok := b.in.Lookup("artist", e.Artist, "")
	if !ok {
		return Item{}, false
	}
	return Item{Kind: "artist", MBID: id, Artist: e.Artist, Rank: e.Rank, Prev: e.Prev, Peak: e.Peak, Days: e.Days, Streams: e.Streams}, true
}

// song turns a charted song into its album (same artwork on an album
// list, else the album the song's own answer names), else its own single,
// else its artist. A song out in the last two years that would show as an
// album older than two years is re-checked: its single when MusicBrainz
// has one, else its artist (logged).
func (b builder) song(e Entry) (Item, bool) {
	al, ok := b.images[e.Image]
	if !ok || e.Image == "" {
		al, ok = Entry{Title: e.Album, Artist: e.Artist, ReleaseDate: e.ReleaseDate}, isAlbum(e) && e.AlbumType != "single"
	}
	if ok {
		if it, ok := b.album(Entry{Title: al.Title, Artist: al.Artist, ReleaseDate: al.ReleaseDate}); ok {
			albumAge, known := ageDays(it.Date, b.in.Now)
			songAge, songKnown := ageDays(e.ReleaseDate, b.in.Now)
			if !(known && albumAge > recheckAge && songKnown && songAge <= recheckAge) {
				it.Rank, it.Prev, it.Peak, it.Days, it.Streams, it.Song = e.Rank, e.Prev, e.Peak, e.Days, e.Streams, cleanTitle(e.Title)
				return it, true
			}
			from := it.Date
			if len(from) > 4 {
				from = from[:4]
			}
			if s, ok := b.single(e); ok {
				if d, ok := ageDays(s.Date, b.in.Now); !ok || d <= recheckAge {
					b.rematch(e, from, "its single")
					return s, true
				}
			}
			b.rematch(e, from, "its artist")
			it, ok := b.artist(e)
			it.Song = cleanTitle(e.Title)
			return it, ok
		}
	}
	if s, ok := b.single(e); ok {
		return s, true
	}
	it, ok := b.artist(e)
	it.Song = cleanTitle(e.Title)
	return it, ok
}

func (b builder) rematch(e Entry, from, to string) {
	if b.in.Rematch != nil {
		b.in.Rematch(e.Title, e.Artist, from, to)
	}
}

// fold keeps each MBID once (its best rank, summed streams), at most limit.
func fold(items []Item, limit int) []Item {
	at := map[string]int{}
	var out []Item
	for _, it := range items {
		if i, ok := at[it.MBID]; ok {
			out[i].Streams += it.Streams
			continue
		}
		if len(out) >= limit {
			continue
		}
		at[it.MBID] = len(out)
		out = append(out, it)
	}
	return out
}

func maxStreams(es []Entry) int64 {
	var m int64
	for _, e := range es {
		m = max(m, e.Streams)
	}
	return m
}

func (b builder) songs(es []Entry, ago []Entry, keep func(Entry) bool, limit int) []Item {
	agoRank := map[string]int{}
	for _, e := range ago {
		agoRank[entryKey(e)] = e.Rank
	}
	ms := maxStreams(es)
	var out []Item
	for _, e := range es {
		if keep != nil && !keep(e) {
			continue
		}
		it, ok := b.song(e)
		if !ok {
			continue
		}
		it.P = pop(e.Rank, len(es), e.Streams, ms)
		it.M = momentum(e, agoRank[entryKey(e)], len(es))
		out = append(out, it)
	}
	return fold(out, limit)
}

func (b builder) albums(es []Entry, keep func(Entry) bool, limit int) []Item {
	var out []Item
	for _, e := range es {
		if keep != nil && !keep(e) {
			continue
		}
		it, ok := b.album(e)
		if !ok {
			continue
		}
		it.P = pop(e.Rank, len(es), 0, 0)
		out = append(out, it)
	}
	return fold(out, limit)
}

func (b builder) artists(es []Entry, limit int) []Item {
	var out []Item
	for _, e := range es {
		it, ok := b.artist(e)
		if !ok {
			continue
		}
		it.P = pop(e.Rank, len(es), 0, 0)
		out = append(out, it)
	}
	return fold(out, limit)
}

// Build makes one region's shelves. Items without a MusicBrainz match are
// left out (never shown with source ids).
func Build(region string, in Inputs) Shelves {
	b := builder{in: in, images: map[string]Entry{}}
	for k, es := range in.Lists {
		if strings.HasPrefix(k, "albums:") || strings.HasPrefix(k, "new:") {
			for _, e := range es {
				if _, ok := b.images[e.Image]; !ok && e.Image != "" {
					b.images[e.Image] = e
				}
			}
		}
	}
	songs, chart := in.list(KindSongs, region)
	world := in.Lists["songs:global"]
	albumsR, _ := in.list(KindAlbums, region)
	albumsG := in.Lists["albums:global"]
	artistsR, _ := in.list(KindArtists, region)
	newR, _ := in.list(KindNew, region)
	ago := in.Ago["songs:"+chart]
	now := in.Now
	sh := Shelves{Region: region, Chart: chart, AsOf: in.AsOf, Blocks: map[string][]Item{}}
	put := func(name string, l []Item) {
		if len(l) > 0 {
			sh.Blocks[name] = l
		}
	}
	put("top_now", b.songs(songs, ago, nil, 60))
	if chart != "global" {
		put("top_world", b.songs(world, in.Ago["songs:global"], nil, 60))
	} else if es := in.Lists["songs:us"]; len(es) > 0 && region == "global" {
		// Worldwide visitors get the biggest single country as a second chart.
		put("top_world", b.songs(es, in.Ago["songs:us"], nil, 60))
	}
	put("albums_week", b.albums(albumsR, nil, 60))
	if len(albumsG) > 0 {
		put("albums_world", b.albums(albumsG, nil, 60))
	}
	put("artists_week", b.artists(artistsR, 60))
	climb := b.songs(songs, ago, func(e Entry) bool { return e.Status != "new" && e.Prev > e.Rank }, 200)
	sort.SliceStable(climb, func(i, j int) bool {
		if climb[i].M != climb[j].M {
			return climb[i].M > climb[j].M
		}
		return climb[i].Rank < climb[j].Rank
	})
	put("climbing", trim(climbOnly(climb), 40))
	isNew := func(e Entry) bool { return e.Status == "new" || e.Status != "back" && within(e.EntryDate, 7, now) }
	put("new_entries", mergeItems(b.songs(songs, ago, isNew, 40), b.albums(albumsR, isNew, 40), 40))
	put("new_friday", b.newReleases(newR, 40))
	put("big_new_albums", b.albums(albumsR, func(e Entry) bool { return within(e.ReleaseDate, 30, now) }, 40))
	var singles []Item
	for _, it := range b.songs(songs, ago, nil, 200) {
		if it.Kind == "artist" {
			singles = append(singles, it)
		}
	}
	put("singles_artists", trim(singles, 40))
	long := mergeItems(b.songs(songs, ago, func(e Entry) bool { return e.Days >= 40 }, 60), b.albums(albumsR, func(e Entry) bool { return e.Days >= 26 }, 60), 60)
	sort.SliceStable(long, func(i, j int) bool { return long[i].Days > long[j].Days })
	put("long_runners", trim(long, 40))
	ones := func(e Entry) bool { return e.Peak == 1 && within(e.PeakDate, 30, now) }
	put("number_ones", mergeItems(b.songs(songs, ago, ones, 30), b.albums(albumsR, ones, 30), 30))
	back := func(e Entry) bool { return e.Status == "back" }
	put("comebacks", mergeItems(b.songs(songs, ago, back, 30), b.albums(albumsR, back, 30), 30))
	put("tile:"+SeasonTile.Key, b.tile(SeasonTile))
	for _, t := range Tiles {
		put("tile:"+t.Key, b.tile(t))
	}
	put("video_artists", videoArtists(sh.Blocks))
	if n := neighbors[region]; n != "" {
		if es := in.Lists["songs:"+n]; len(es) > 0 && n != chart {
			sh.Neighbor = n
			put("neighbor", b.songs(es, in.Ago["songs:"+n], nil, 60))
		}
	}
	return sh
}

// newReleases is the new-release shelf: the US new-release list's albums
// out within 14 days, then ListenBrainz's fresh releases out within 14
// days (not yet out: never), most listened first, each id once.
func (b builder) newReleases(es []Entry, limit int) []Item {
	now := b.in.Now
	out := b.albums(es, func(e Entry) bool { return within(e.ReleaseDate, 14, now) && released(e.ReleaseDate, now) }, limit)
	seen := map[string]bool{}
	kept := out[:0]
	for _, it := range out {
		seen[it.MBID] = true
		if it.Date == "" || released(it.Date, now) {
			kept = append(kept, it)
		}
	}
	out = kept
	fresh := append([]FreshAlbum{}, b.in.Fresh...)
	sort.SliceStable(fresh, func(i, j int) bool { return fresh[i].Listens > fresh[j].Listens })
	today := now.UTC().Format("2006-01-02")
	for _, f := range fresh {
		if len(out) >= limit {
			break
		}
		if !mbidRe.MatchString(f.MBID) || seen[f.MBID] || len(f.Date) < 10 || f.Date[:10] > today || !within(f.Date, 14, now) {
			continue
		}
		seen[f.MBID] = true
		out = append(out, Item{Kind: "album", MBID: f.MBID, ArtistMBID: f.ArtistMBID, Title: cleanTitle(f.Title), Artist: f.Artist, Rank: len(out) + 1, Date: f.Date[:10]})
	}
	for i := range out {
		out[i].Rank, out[i].P = i+1, pop(i+1, len(out), 0, 0)
	}
	return out
}

// tile merges a tile's lists (slot 0 first at each position) into albums
// (a song on a real album) and artists (a song on its own single), in
// list order, each MusicBrainz id once.
func (b builder) tile(t Tile) []Item {
	var slots [][]Entry
	for slot := 0; slot < 3; slot++ {
		if es := b.in.Lists[fmt.Sprintf("list:%s:%d", t.Key, slot)]; len(es) > 0 {
			slots = append(slots, es)
		}
	}
	var out []Item
	pos := 0
	for i := 0; ; i++ {
		more := false
		for _, es := range slots {
			if i >= len(es) {
				continue
			}
			more = true
			e := es[i]
			var it Item
			ok := false
			if isAlbum(e) {
				it, ok = b.album(Entry{Title: e.Album, Artist: e.Artist})
			} else {
				it, ok = b.single(e)
			}
			if !ok {
				it, ok = b.artist(e)
			}
			if !ok {
				continue
			}
			pos++
			it.Rank, it.Song = pos, cleanTitle(e.Title)
			out = append(out, it)
		}
		if !more {
			break
		}
	}
	l := fold(out, 40)
	for i := range l {
		l[i].P = pop(i+1, len(l), 0, 0)
	}
	return l
}

// videoArtists are the shelves' artists for the music video row: the
// week's artist chart, then the artists behind today's top songs.
func videoArtists(blocks map[string][]Item) []Item {
	var out []Item
	seen := map[string]bool{}
	add := func(id, name string, rank int) {
		if id == "" || seen[id] || len(out) >= 40 {
			return
		}
		seen[id] = true
		out = append(out, Item{Kind: "artist", MBID: id, Artist: name, Rank: len(out) + 1})
	}
	for _, it := range blocks["artists_week"] {
		add(it.MBID, it.Artist, it.Rank)
	}
	for _, name := range []string{"top_now", "top_world"} {
		for _, it := range blocks[name] {
			if it.Kind == "artist" {
				add(it.MBID, it.Artist, it.Rank)
			} else {
				add(it.ArtistMBID, it.Artist, it.Rank)
			}
		}
	}
	for i := range out {
		out[i].P = pop(i+1, len(out), 0, 0)
	}
	return out
}

func climbOnly(l []Item) []Item {
	var out []Item
	for _, it := range l {
		if it.M > 0 {
			out = append(out, it)
		}
	}
	return out
}

func trim(l []Item, n int) []Item { return l[:min(n, len(l))] }

// mergeItems joins two lists by rank, each MBID once.
func mergeItems(a, b []Item, limit int) []Item {
	all := append(append([]Item{}, a...), b...)
	sort.SliceStable(all, func(i, j int) bool { return all[i].Rank < all[j].Rank })
	return fold(all, limit)
}
