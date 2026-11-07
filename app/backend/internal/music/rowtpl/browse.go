package rowtpl

import (
	"strings"
	"time"
)

// coversPerTile is how many covers a tile brings to try in order.
const coversPerTile = 3

// MinPageRows is the fewest rows a page needs to be shown. A tile with fewer is hidden until its data grows.
const MinPageRows = 12

// BrowseLayout is the fixed Browse page: genres, moods, seasons, decades and activities.
var BrowseLayout = []layoutSection{
	{"genres", "Genres", tiles("genre", "pop", "Pop", "hip-hop", "Hip-hop", "rock", "Rock", "latin", "Latin", "rnb", "R&B", "country", "Country", "dance", "Dance & electronic", "indie", "Indie", "alternative", "Alternative", "metal", "Metal", "kpop", "K-pop", "afro", "Afro", "jazz", "Jazz", "classical", "Classical", "soul-funk", "Soul & funk", "folk", "Folk & acoustic", "reggae", "Reggae & Caribbean", "blues", "Blues", "punk", "Punk", "christian", "Christian & gospel", "soundtracks", "Soundtracks", "kids", "Kids & family")},
	{"moods", "Moods", tiles("mood", "chill", "Chill", "party", "Party", "love", "Love", "sad", "Sad", "happy", "Happy", "focus", "Focus", "workout", "Workout", "sleep", "Sleep", "romance", "Romance", "energy", "High-energy", "throwback", "Throwback", "feel-good", "Feel-good")},
	{"seasons", "Celebrations & seasons", tiles("season", "halloween", "Halloween", "christmas", "Christmas", "new-year", "New Year", "valentines", "Valentine's", "summer", "Summer", "dia-de-muertos", "Día de los Muertos", "lunar-new-year", "Lunar New Year", "pride", "Pride", "thanksgiving", "Thanksgiving", "back-to-school", "Back to school", "fall", "Fall", "spring", "Spring")},
	{"decades", "Decades", tiles("decade", "1960s", "60s", "1970s", "70s", "1980s", "80s", "1990s", "90s", "2000s", "2000s", "2010s", "2010s", "2020s", "2020s")},
	{"activities", "Activities", tiles("activity", "driving", "Driving", "cooking", "Cooking", "gaming", "Gaming", "studying", "Studying", "party", "Party", "gym", "Gym", "running", "Running", "home", "Chill at home")},
}

// exploreOrder is the order kinds alternate in Explore, so the first two rows read as a mix. A kind with no tile left is skipped.
var exploreOrder = []string{"genres", "genres", "moods", "genres", "seasons", "genres", "decades", "moods", "genres", "activities", "genres", "moods", "decades", "genres", "seasons", "moods", "activities", "genres"}

// exploreDecades is the order decades are offered in. The 90s lead because most libraries and charts lean on them.
var exploreDecades = []string{"decade:1990s", "decade:2000s", "decade:2010s", "decade:1980s", "decade:2020s", "decade:1970s", "decade:1960s"}

// BrowseTile is one Browse tile: its name, the page it opens, and up to three covers to try in order.
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

// layoutSection is a section of the fixed layout before covers and hues are filled in.
type layoutSection struct {
	ID, Title string
	Tiles     []BrowseTile
}

// BrowseSections answers the Browse page for a region and day. lib may be nil. A tile whose pool has no album is left out, so a tile never shows without a cover.
func BrowseSections(b *Bundle, now time.Time, region string, lib *Lib) []BrowseSection {
	return BrowseSectionsArt(b, now, region, lib, nil)
}

// BrowseSectionsArt is BrowseSections that takes covers only from albums hasArt accepts, by item index. A nil hasArt accepts all. Tiles and sections left with nothing are dropped.
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

// Hues gives n tiles one of 24 hues. Steps of 7 over 24 never repeat within three places, so neighbours differ in 2, 3 and 4 column layouts.
func Hues(n, cols int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = (i * 7) % 24
	}
	return out
}

// ExploreTiles answers up to 18 mixed tiles for Explore. A screen shows two rows of up to 7, and the rest stand in for tiles that drop out. Party in Activities is left out because its page already opens from Moods.
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

// PageCounts builds each key's page as a member with an empty library sees it and answers its row count. Keys the bundle lacks count 0.
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

// BrowseKeys answers every key a Browse tile opens, once each.
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

// VisibleSections keeps the tiles whose page has at least MinPageRows rows. Season tiles also need the season on now or starting within 45 days.
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

// Similar answers up to n shown tiles of the key's own kind, ranked by how many albums their pools share with the key's pool.
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

// tiles builds tiles of one kind from key and name pairs.
func tiles(kind string, pairs ...string) []BrowseTile {
	var out []BrowseTile
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, BrowseTile{Key: kind + ":" + pairs[i], Name: pairs[i+1]})
	}
	return out
}

// tileHref is the page a tile opens. Party in Activities opens the mood page.
func tileHref(key string) string {
	kind, slug, _ := strings.Cut(key, ":")
	if kind == "decade" && len(slug) == 5 {
		slug = slug[2:]
	}
	if key == "activity:party" {
		return "/music/mood/party"
	}
	return "/music/" + kind + "/" + slug
}

// poolKey is the bundle key a tile reads. Party in Activities reads the mood.
func poolKey(key string) string {
	if key == "activity:party" {
		return "mood:party"
	}
	return key
}

// tileCovers answers a pool's cover addresses, up to coversPerTile albums with art. The first one is not used by an earlier tile, so no two tiles start with the same cover. A pool whose top albums were all taken still brings its own, so it is not dropped while it has art.
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
	for i, id := range covers {
		covers[i] = "/api/v1/music-trending/art/album/" + id
	}
	return covers
}

// sortTiles orders tiles by rank, lowest first. It is an insertion sort, so equal ranks keep their order.
func sortTiles(ts []BrowseTile, rank map[string]int) {
	for i := 1; i < len(ts); i++ {
		for j := i; j > 0 && rank[ts[j].Key] < rank[ts[j-1].Key]; j-- {
			ts[j], ts[j-1] = ts[j-1], ts[j]
		}
	}
}
