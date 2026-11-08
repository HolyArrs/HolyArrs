package musicscore

import (
	"math"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Diversity is the score penalty for each item already placed with the same
// primary genre.
const Diversity = 0.15

const (
	// SlotMin is the fewest items a personal row needs for an exploration slot.
	SlotMin = 9
	// SlotAt is the zero-based place of the exploration slot, the fourth item.
	SlotAt = 3
)

// genreSpecial spells words and tags that plain title case gets wrong.
var genreSpecial = map[string]string{
	"hip hop": "Hip-Hop", "hiphop": "Hip-Hop", "hip-hop": "Hip-Hop",
	"r&b": "R&B", "rnb": "R&B", "r'n'b": "R&B", "k-pop": "K-Pop", "kpop": "K-Pop",
	"j-pop": "J-Pop", "jpop": "J-Pop", "j-rock": "J-Rock", "c-pop": "C-Pop",
	"edm": "EDM", "uk": "UK", "us": "US", "dj": "DJ", "idm": "IDM", "ebm": "EBM",
	"lo-fi": "Lo-Fi", "lofi": "Lo-Fi", "drum and bass": "Drum and Bass", "dnb": "Drum and Bass",
	"and": "and", "n": "n", "of": "of",
}

// genreSuffixes are the second halves of tags written as one word, such as
// "alternativerock" or "synthpop".
var genreSuffixes = []string{"rock", "pop", "metal", "punk", "hop", "soul", "folk", "jazz", "house", "funk", "country"}

// Scored is one candidate with what the in-row order needs: its artist
// affinity, its genre match and its primary genre. An empty genre is never
// penalised.
type Scored struct {
	Item
	A, GItem float64
	Genre    string
}

// ArtistPopularity returns 0.6*pcBest + 0.4*lnorm for a charted artist, and
// the uncharted value otherwise.
func ArtistPopularity(pcBest float64, charted bool, lnorm float64) float64 {
	if charted {
		return 0.6*pcBest + 0.4*lnorm
	}
	return Uncharted(lnorm)
}

// GenreKey folds a genre name for matching across sources: lower case with
// no spaces, hyphens or underscores.
func GenreKey(g string) string {
	return strings.NewReplacer("-", "", " ", "", "_", "").Replace(strings.ToLower(strings.TrimSpace(g)))
}

// ItemGenre returns the largest genre weight among the item's genres, or 0
// when none match. g is keyed by GenreKey.
func ItemGenre(g map[string]float64, genres []string) float64 {
	best := 0.0
	for _, x := range genres {
		k := GenreKey(x)
		best = math.Max(best, g[k])
		// A tag like "hiphop/rap" also matches "hiphop" and "rap".
		for _, part := range strings.Split(k, "/") {
			best = math.Max(best, g[part])
		}
	}
	return best
}

// Shares turns an artist's genre votes into vote shares, ignoring votes
// under 1. No votes gives an empty map.
func Shares(votes map[string]int) map[string]float64 {
	total := 0
	for _, n := range votes {
		if n >= 1 {
			total += n
		}
	}
	out := map[string]float64{}
	if total == 0 {
		return out
	}
	for g, n := range votes {
		if k := GenreKey(g); n >= 1 && k != "" {
			out[k] += float64(n) / float64(total)
		}
	}
	return out
}

// Similarity returns the ListenBrainz score (already divided by the seed's
// top score), or at least 0.8 when MusicBrainz lists a member or
// collaboration link.
func Similarity(lb float64, relation bool) float64 {
	if relation {
		return math.Max(lb, 0.8)
	}
	return lb
}

// ScoreNewFrom scores a New from artists row as 0.6*a + 0.4*f14.
func ScoreNewFrom(a, f14 float64) float64 { return 0.6*a + 0.4*f14 }

// ScoreFreshSong scores a fresh song as 0.7*p + 0.3*f14.
func ScoreFreshSong(p, f14 float64) float64 { return 0.7*p + 0.3*f14 }

// ScoreMissing scores a missing album as 0.5*a + 0.3*p + 0.2*latest.
func ScoreMissing(a, p, latest float64) float64 { return 0.5*a + 0.3*p + 0.2*latest }

// ScoreComingSoon scores a coming soon item as 0.7*a + 0.3*p.
func ScoreComingSoon(a, p float64) float64 { return 0.7*a + 0.3*p }

// ScoreSinceYouHave scores an item as 0.5*s + 0.3*p + 0.2*gItem.
func ScoreSinceYouHave(s, p, gItem float64) float64 { return 0.5*s + 0.3*p + 0.2*gItem }

// ScoreGenreShelf scores a genre shelf item as 0.5*p + 0.3*g + 0.2*m.
func ScoreGenreShelf(p, g, m float64) float64 { return 0.5*p + 0.3*g + 0.2*m }

// ScoreGuest scores a guest appearance as 0.6*a + 0.4*p.
func ScoreGuest(a, p float64) float64 { return 0.6*a + 0.4*p }

// ScoreNearTaste returns normed plus 0.2*p, capped at 1. normed is the
// already normalised sum of affinity times similarity.
func ScoreNearTaste(normed, p float64) float64 { return math.Min(1, normed+0.2*p) }

// Bool returns 1 for true and 0 for false.
func Bool(v bool) float64 {
	if v {
		return 1
	}
	return 0
}

// Rank sorts by score, best first, breaking ties with a daily hash.
func Rank(list []Scored, member int64, date string) {
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].Score != list[j].Score {
			return list[i].Score > list[j].Score
		}
		return Tie(member, date, list[i].Key) < Tie(member, date, list[j].Key)
	})
}

// Diversify re-ranks the top 12 greedily, each place taking the item with
// the best score after the diversity penalty. Ties keep the ranked order
// and the rest stay as they are.
func Diversify(list []Scored) []Scored {
	n := min(12, len(list))
	pool := append([]Scored{}, list[:n]...)
	out := make([]Scored, 0, len(list))
	placed := map[string]int{}
	for len(pool) > 0 {
		best, bv := 0, math.Inf(-1)
		for i, x := range pool {
			v := x.Score
			if x.Genre != "" {
				v -= Diversity * float64(placed[x.Genre])
			}
			if v > bv {
				best, bv = i, v
			}
		}
		x := pool[best]
		pool = append(pool[:best], pool[best+1:]...)
		if x.Genre != "" {
			placed[x.Genre]++
		}
		out = append(out, x)
	}
	return append(out, list[n:]...)
}

// Explore moves the best item with no affinity and a genre match of at
// least 0.3 into the exploration slot of a row with at least SlotMin items.
// It leaves the order alone when there is no such item, when it is already
// in the first four places, or when the row is shorter.
func Explore(list []Scored) []Scored {
	if len(list) < SlotMin {
		return list
	}
	pick := -1
	for i, x := range list {
		// The list is ranked, so the first match is the best.
		if x.A == 0 && x.GItem >= 0.3 {
			pick = i
			break
		}
	}
	// An item already in the first four places needs no slot.
	if pick < 0 || pick <= SlotAt {
		return list
	}
	out := make([]Scored, 0, len(list))
	x := list[pick]
	rest := append(append([]Scored{}, list[:pick]...), list[pick+1:]...)
	out = append(out, rest[:min(SlotAt, len(rest))]...)
	out = append(out, x)
	if len(rest) > SlotAt {
		out = append(out, rest[SlotAt:]...)
	}
	return out
}

// Items drops the scoring extras and returns the plain items.
func Items(list []Scored) []Item {
	out := make([]Item, len(list))
	for i, x := range list {
		out[i] = x.Item
	}
	return out
}

// Label upper-cases the first letter of a name, safe for multi-byte text.
func Label(name string) string {
	r, n := utf8.DecodeRuneInString(name)
	if n == 0 || r == utf8.RuneError {
		return name
	}
	return string(unicode.ToUpper(r)) + name[n:]
}

// GenreName returns a display name for a raw genre tag, such as
// "Alternative Rock" for "alternativerock" and "Hip-Hop" for "hip hop".
func GenreName(tag string) string {
	t := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(tag, "_", " ")))
	if s, ok := genreSpecial[t]; ok {
		return s
	}
	var out []string
	for i, w := range strings.Fields(t) {
		if s, ok := genreSpecial[w]; ok && (i > 0 || s != strings.ToLower(s)) {
			out = append(out, s)
			continue
		}
		parts := []string{w}
		if !strings.Contains(w, "-") {
			for _, suf := range genreSuffixes {
				if len(w) > len(suf)+2 && strings.HasSuffix(w, suf) {
					head := w[:len(w)-len(suf)]
					if s, ok := genreSpecial[head]; ok && s != strings.ToLower(s) {
						head = s
					}
					parts = []string{head, suf}
					break
				}
			}
		}
		for _, p := range parts {
			if s, ok := genreSpecial[p]; ok && s != strings.ToLower(s) {
				out = append(out, s)
				continue
			}
			segs := strings.Split(p, "-")
			for j, sg := range segs {
				segs[j] = Label(sg)
			}
			out = append(out, strings.Join(segs, "-"))
		}
	}
	return strings.Join(out, " ")
}
