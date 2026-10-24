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

// BrowseSections answers the Browse page for a region and day: seasons in
// their order (on now, then soon, then the rest), genres by the member's
// genres then the region's, and each tile's covers (the top albums of its
// pool, the first one not used by an earlier tile). lib may be nil.
func BrowseSections(b *Bundle, now time.Time, region string, lib *Lib) []BrowseSection {
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
		hues := Hues(len(ts), 4)
		for i := range ts {
			ts[i].Href = tileHref(ts[i].Key)
			ts[i].Hue = hues[i]
			var covers []string
			first := -1
			for _, m := range b.Keys[poolKey(ts[i].Key)] {
				if b.Items[m.I].Kind != "a" {
					continue
				}
				if first < 0 && !used[m.I] {
					first = m.I
					used[m.I] = true
					covers = append([]string{b.Items[m.I].ID}, covers...)
				} else if len(covers) < 3 && m.I != first {
					covers = append(covers, b.Items[m.I].ID)
				}
				if first >= 0 && len(covers) >= 3 {
					break
				}
			}
			for j := range covers {
				covers[j] = "/api/v1/music-trending/art/album/" + covers[j]
			}
			ts[i].Covers = covers
		}
		out = append(out, BrowseSection{ID: ls.ID, Title: ls.Title, Tiles: ts})
	}
	return out
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

// ExploreTiles answers Explore's Browse row: the top 4 genres, the first
// season (on now, else soon, else next), the top 2 moods and one decade.
func ExploreTiles(secs []BrowseSection) []BrowseTile {
	take := map[string]int{"genres": 4, "seasons": 1, "moods": 2, "decades": 1}
	var out []BrowseTile
	for _, id := range []string{"genres", "seasons", "moods", "decades"} {
		for _, s := range secs {
			if s.ID != id {
				continue
			}
			for _, t := range s.Tiles {
				if take[id] == 0 {
					break
				}
				if id == "decades" && t.Key != "decade:1990s" && take[id] == 1 && len(s.Tiles) > 3 {
					continue // the 90s: the decade most libraries and charts lean on
				}
				out = append(out, t)
				take[id]--
			}
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
