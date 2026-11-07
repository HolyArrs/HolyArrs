package rowtpl

import (
	"strings"
	"time"
)

// BrowseTile is one Search > Music Browse tile: our name, the page it
// opens, and up to 3 covers to try in order (no two tiles start with the
// same one).
type BrowseTile struct {
	Key    string   `json:"key"`
	Name   string   `json:"name"`
	Href   string   `json:"href"`
	Hue    int      `json:"hue"`
	Covers []string `json:"covers"`
}

// BrowseSection is one titled group of tiles.
type BrowseSection struct {
	ID    string       `json:"id"`
	Title string       `json:"title"`
	Tiles []BrowseTile `json:"tiles"`
}

type layoutSection struct {
	ID, Title string
	Tiles     []BrowseTile
}

func tiles(kind string, pairs ...string) []BrowseTile {
	var out []BrowseTile
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, BrowseTile{Key: kind + ":" + pairs[i], Name: pairs[i+1]})
	}
	return out
}

// BrowseLayout is the Browse page: 22 genres, 12 moods, 12 seasons,
// 7 decades and 8 activities (61 tiles; Party opens the mood page).
var BrowseLayout = []layoutSection{
	{"genres", "Genres", tiles("genre", "pop", "Pop", "hip-hop", "Hip-hop", "rock", "Rock", "latin", "Latin", "rnb", "R&B", "country", "Country", "dance", "Dance & electronic", "indie", "Indie", "alternative", "Alternative", "metal", "Metal", "kpop", "K-pop", "afro", "Afro", "jazz", "Jazz", "classical", "Classical", "soul-funk", "Soul & funk", "folk", "Folk & acoustic", "reggae", "Reggae & Caribbean", "blues", "Blues", "punk", "Punk", "christian", "Christian & gospel", "soundtracks", "Soundtracks", "kids", "Kids & family")},
	{"moods", "Moods", tiles("mood", "chill", "Chill", "party", "Party", "love", "Love", "sad", "Sad", "happy", "Happy", "focus", "Focus", "workout", "Workout", "sleep", "Sleep", "romance", "Romance", "energy", "High-energy", "throwback", "Throwback", "feel-good", "Feel-good")},
	{"seasons", "Celebrations & seasons", tiles("season", "halloween", "Halloween", "christmas", "Christmas", "new-year", "New Year", "valentines", "Valentine's", "summer", "Summer", "dia-de-muertos", "Día de los Muertos", "lunar-new-year", "Lunar New Year", "pride", "Pride", "thanksgiving", "Thanksgiving", "back-to-school", "Back to school", "fall", "Fall", "spring", "Spring")},
	{"decades", "Decades", tiles("decade", "1960s", "60s", "1970s", "70s", "1980s", "80s", "1990s", "90s", "2000s", "2000s", "2010s", "2010s", "2020s", "2020s")},
	{"activities", "Activities", tiles("activity", "driving", "Driving", "cooking", "Cooking", "gaming", "Gaming", "studying", "Studying", "party", "Party", "gym", "Gym", "running", "Running", "home", "Chill at home")},
}

// tileHref is a tile's page.
func tileHref(key string) string {
	kind, slug, _ := strings.Cut(key, ":")
	if kind == "decade" && len(slug) == 5 {
		slug = slug[2:] // "1990s" → "90s"
	}
	if key == "activity:party" {
		return "/music/mood/party"
	}
	return "/music/" + kind + "/" + slug
}

// poolKey is the bundle key a tile reads (Party in Activities reads the
// mood).
func poolKey(key string) string {
	if key == "activity:party" {
		return "mood:party"
	}
	return key
}

// coversPerTile is how many covers a tile brings to try in order.
const coversPerTile = 3

// BrowseSections answers the Browse page for a region and day: seasons in
// their order (on now, then soon, then the rest), genres by the member's
// genres then the region's, and each tile's covers (the top albums of its
// pool, the first one not used by an earlier tile). lib may be nil. A tile
// whose pool has no album is left out: a tile never shows without a cover.
func BrowseSections(b *Bundle, now time.Time, region string, lib *Lib) []BrowseSection {
	return BrowseSectionsArt(b, now, region, lib, nil)
}

// BrowseSectionsArt is BrowseSections with the cover rule: a tile's covers
// are taken only from albums whose picture is stored or known to load
// (hasArt, by item index; nil checks nothing), and a tile with no such album
// is left out of its section (a section with no tile is left out too).
func BrowseSectionsArt(b *Bundle, now time.Time, region string, lib *Lib, hasArt func(int) bool) []BrowseSection {
	c := &Ctx{B: b, Region: region, Now: now}
	var out []BrowseSection
	used := map[int]bool{}
	for _, ls := range BrowseLayout {
		ts := append([]BrowseTile{}, ls.Tiles...)
		switch ls.ID {
		case "seasons":
			var order []BrowseTile
			for _, s := range SeasonOrder(now, c.region()) {
				for _, t := range ts {
					if t.Key == "season:"+s.Key {
						order = append(order, t)
					}
				}
			}
			ts = order
		case "genres":
			var keys []string
			for _, t := range ts {
				keys = append(keys, t.Key)
			}
			rank := map[string]int{}
			for i, k := range c.regionWeight(keys) {
				rank[k] = i + 100
			}
			if lib != nil {
				for i, k := range lib.Genres {
					if _, ok := rank[k]; ok {
						rank[k] = i
					}
				}
			}
			sortTiles(ts, rank)
		}
		var kept []BrowseTile
		for _, t := range ts {
			covers := tileCovers(b, poolKey(t.Key), used, hasArt)
			if len(covers) == 0 {
				continue
			}
			t.Href = tileHref(t.Key)
			t.Covers = covers
			kept = append(kept, t)
		}
		hues := Hues(len(kept), 4)
		for i := range kept {
			kept[i].Hue = hues[i]
		}
		if len(kept) > 0 {
			out = append(out, BrowseSection{ID: ls.ID, Title: ls.Title, Tiles: kept})
		}
	}
	return out
}

// tileCovers answers a pool's cover addresses: up to coversPerTile albums in
// pool order that have art, the first one not used by an earlier tile (and
// now used). None when the pool has no album with art.
func tileCovers(b *Bundle, pool string, used map[int]bool, hasArt func(int) bool) []string {
	var covers []string
	first := -1
	for _, m := range b.Keys[pool] {
		if b.Items[m.I].Kind != "a" || (hasArt != nil && !hasArt(m.I)) {
			continue
		}
		if first < 0 && !used[m.I] {
			first = m.I
			used[m.I] = true
			covers = append([]string{b.Items[m.I].ID}, covers...)
		} else if len(covers) < coversPerTile && m.I != first {
			covers = append(covers, b.Items[m.I].ID)
		}
		if first >= 0 && len(covers) >= coversPerTile {
			break
		}
	}
	// A pool whose albums earlier tiles took first still brings its own top
	// albums to try, so it is not left out while it has art.
	for i, id := range covers {
		covers[i] = "/api/v1/music-trending/art/album/" + id
	}
	return covers
}

func sortTiles(ts []BrowseTile, rank map[string]int) {
	for i := 1; i < len(ts); i++ {
		for j := i; j > 0 && rank[ts[j].Key] < rank[ts[j-1].Key]; j-- {
			ts[j], ts[j-1] = ts[j-1], ts[j]
		}
	}
}

// Hues assigns n tiles one of 24 hues so that no tile shares a hue with
// the tile before it or the one above it, for the given column count; the
// layout uses 4 columns and the same hues read well at 2 and 3 (steps of
// 7 over 24 never repeat within 3 places either way).
func Hues(n, cols int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = (i * 7) % 24
	}
	return out
}

// exploreOrder is how Explore's and New & Popular's tiles alternate, kind by
// kind, so the first two rows read as a mix: genres lead, moods, a season,
// decades and activities between them. A kind with no tile left is skipped.
var exploreOrder = []string{"genres", "genres", "moods", "genres", "seasons", "genres", "decades", "moods", "genres", "activities", "genres", "moods", "decades", "genres", "seasons", "moods", "activities", "genres"}

// exploreDecades is the order decades are offered in: the 90s first (the decade
// most libraries and charts lean on), then the neighbours.
var exploreDecades = []string{"decade:1990s", "decade:2000s", "decade:2010s", "decade:1980s", "decade:2020s", "decade:1970s", "decade:1960s"}

// ExploreTiles answers the mix Explore's Browse and New & Popular's Explore
// more offer: up to 18 tiles (a screen shows two rows of up to 7 and the rest
// stand in for any that drop out): the top genres, a few moods, the season on
// now (and the next), two decades and two activities, alternating kinds. A
// page opens once, so Party in Activities (the mood's page) is left out.
func ExploreTiles(secs []BrowseSection) []BrowseTile {
	queue := map[string][]BrowseTile{}
	for _, s := range secs {
		for _, t := range s.Tiles {
			if t.Key == "activity:party" {
				continue
			}
			queue[s.ID] = append(queue[s.ID], t)
		}
	}
	// Decades in the offered order.
	var decades []BrowseTile
	for _, k := range exploreDecades {
		for _, t := range queue["decades"] {
			if t.Key == k {
				decades = append(decades, t)
			}
		}
	}
	queue["decades"] = decades
	var out []BrowseTile
	for _, id := range exploreOrder {
		if q := queue[id]; len(q) > 0 {
			out = append(out, q[0])
			queue[id] = q[1:]
		}
	}
	return out
}

// MinPageRows is the fewest rows a Browse page needs to be shown: a tile
// (and its page) with fewer is hidden until its data grows.
const MinPageRows = 12

// PageCounts builds each key's page the way a member with an empty
// library sees it and answers its row count (0 for keys the bundle lacks).
func PageCounts(b *Bundle, now time.Time, region string, blocks map[string][]string, keys []string) map[string]int {
	out := map[string]int{}
	for _, k := range keys {
		if len(b.Keys[k]) == 0 {
			out[k] = 0
			continue
		}
		c := &Ctx{B: b, Kind: kindOf(k), Key: k, Region: region, Now: now, Blocks: blocks}
		Spotlight(c)
		out[k] = len(Build(c))
	}
	return out
}

// BrowseKeys answers every key a Browse tile opens.
// ShownKeys answers the page keys of the sections' tiles, in order, once each.
func ShownKeys(secs []BrowseSection) []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range secs {
		for _, t := range s.Tiles {
			if k := poolKey(t.Key); !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	return out
}

func BrowseKeys() []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range BrowseLayout {
		for _, t := range s.Tiles {
			if k := poolKey(t.Key); !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	return out
}

// VisibleSections keeps the tiles whose page has at least MinPageRows
// rows, and the seasons that are on now or start within 45 days.
func VisibleSections(secs []BrowseSection, counts map[string]int, now time.Time, region string) []BrowseSection {
	if region == "" || region == "global" {
		region = "us"
	}
	var out []BrowseSection
	for _, s := range secs {
		var ts []BrowseTile
		for _, t := range s.Tiles {
			if counts[poolKey(t.Key)] < MinPageRows {
				continue
			}
			if s.ID == "seasons" && !SeasonShown(strings.TrimPrefix(t.Key, "season:"), now, region) {
				continue
			}
			ts = append(ts, t)
		}
		if len(ts) > 0 {
			out = append(out, BrowseSection{ID: s.ID, Title: s.Title, Tiles: ts})
		}
	}
	return out
}

// SeasonShown reports a season that is on now or starts within 45 days.
func SeasonShown(key string, now time.Time, region string) bool {
	w, on := window(key, now, region)
	return on || w.Start.Sub(dayOf(now)) <= 45*24*time.Hour
}

// Similar answers up to n shown tiles of the key's own kind, the ones
// whose pools share most albums with the key's first.
func Similar(b *Bundle, key string, shown []BrowseSection, n int) []BrowseTile {
	mine := b.byKey[key]
	type sc struct {
		t BrowseTile
		n int
	}
	var l []sc
	for _, s := range shown {
		for _, t := range s.Tiles {
			k := poolKey(t.Key)
			if k == key || kindOf(k) != kindOf(key) {
				continue
			}
			common := 0
			for i := range b.byKey[k] {
				if _, ok := mine[i]; ok {
					common++
				}
			}
			l = append(l, sc{t, common})
		}
	}
	for i := 1; i < len(l); i++ {
		for j := i; j > 0 && l[j].n > l[j-1].n; j-- {
			l[j], l[j-1] = l[j-1], l[j]
		}
	}
	var out []BrowseTile
	for _, x := range l[:min(n, len(l))] {
		out = append(out, x.t)
	}
	return out
}
