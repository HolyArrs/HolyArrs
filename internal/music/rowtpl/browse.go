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
	{"decades", "Decades", tiles("decade", "1960s", "60s", "1970s", "70s", "1980s", "80s", "1990s", "90s", "2000s", "00s", "2010s", "10s", "2020s", "20s")},
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
