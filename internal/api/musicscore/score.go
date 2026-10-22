package musicscore

import (
	"math"
	"sort"
	"strings"
)

// Phase 3 (plan 2026-10-22 §2.1, §2.3, §2.6): the row formulas, the
// in-row order, diversity and the exploration slot.

// ArtistPopularity is P for an artist: 0.6·Pc_best + 0.4·Lnorm when it
// charts, else min(0.3, 0.3·Lnorm).
func ArtistPopularity(pcBest float64, charted bool, lnorm float64) float64 {
	if charted {
		return 0.6*pcBest + 0.4*lnorm
	}
	return Uncharted(lnorm)
}

// GenreKey folds a genre name for matching across sources: lower case,
// no spaces, hyphens or underscores ("Hip-Hop/Rap" → "hiphop/rap", "hip
// hop" → "hiphop").
func GenreKey(g string) string {
	return strings.NewReplacer("-", "", " ", "", "_", "").Replace(strings.ToLower(strings.TrimSpace(g)))
}

// ItemGenre is G_item: the max G over the item's genres (0 when none).
// g is keyed by GenreKey.
func ItemGenre(g map[string]float64, genres []string) float64 {
	best := 0.0
	for _, x := range genres {
		k := GenreKey(x)
		best = math.Max(best, g[k])
		// "Hip-Hop/Rap" also matches "hiphop" and "rap".
		for _, part := range strings.Split(k, "/") {
			best = math.Max(best, g[part])
		}
	}
	return best
}

// Shares turns an artist's genre votes into vote shares (votes under 1
// dropped); no votes gives an empty map.
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
		if n >= 1 {
			out[GenreKey(g)] += float64(n) / float64(total)
		}
	}
	return out
}

// Similarity is S over sources: the ListenBrainz score already divided by
// the seed's top score, and 0.8 for a MusicBrainz member-of or
// collaboration relation. The max wins.
func Similarity(lb float64, relation bool) float64 {
	if relation {
		return math.Max(lb, 0.8)
	}
	return lb
}

// Formulas, one per row (§2.6).
func ScoreNewFrom(a, f14 float64) float64           { return 0.6*a + 0.4*f14 }
func ScoreFreshSong(p, f14 float64) float64         { return 0.7*p + 0.3*f14 }
func ScoreMissing(a, p, latest float64) float64     { return 0.5*a + 0.3*p + 0.2*latest }
func ScoreComingSoon(a, p float64) float64          { return 0.7*a + 0.3*p }
func ScoreSinceYouHave(s, p, gItem float64) float64 { return 0.5*s + 0.3*p + 0.2*gItem }
func ScoreGenreShelf(p, g, m float64) float64       { return 0.5*p + 0.3*g + 0.2*m }
func ScoreGuest(a, p float64) float64               { return 0.6*a + 0.4*p }

// ScoreNearTaste is norm(Σ A_i·S_i) + 0.2·P, capped at 1; normed is the
// already-normed sum.
func ScoreNearTaste(normed, p float64) float64 { return math.Min(1, normed+0.2*p) }

// Bool is 1 for true.
func Bool(v bool) float64 {
	if v {
		return 1
	}
	return 0
}

// Scored is one candidate with what the in-row order needs: its score,
// A of its artist, G_item, and its primary genre (empty never penalised).
type Scored struct {
	Item
	A, GItem float64
	Genre    string
}

// Rank sorts by score, best first; ties by the day's hash.
func Rank(list []Scored, member int64, date string) {
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].Score != list[j].Score {
			return list[i].Score > list[j].Score
		}
		return Tie(member, date, list[i].Key) < Tie(member, date, list[j].Key)
	})
}

// Diversity is the diversity penalty per item already placed with the
// same primary genre.
const Diversity = 0.15

// Diversify re-ranks the top 12 greedily: each place takes the item with
// the best score − 0.15·(items placed with its genre). Ties keep the
// ranked order. The rest stay as they are.
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

// SlotMin is the fewest items a personal row needs for the exploration
// slot; SlotAt the slot's place (position 4).
const (
	SlotMin = 9
	SlotAt  = 3
)

// Explore puts the best candidate with A = 0 and G_item ≥ 0.3 at position
// 4 of a personal row with at least 9 items. With no such candidate, one
// already in the first four places, or a shorter row, the order is
// unchanged.
func Explore(list []Scored) []Scored {
	if len(list) < SlotMin {
		return list
	}
	pick := -1
	for i, x := range list {
		if x.A == 0 && x.GItem >= 0.3 {
			pick = i
			break // list is ranked, so the first is the best
		}
	}
	// One already in the first four places needs no slot.
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

// Items drops the extras.
func Items(list []Scored) []Item {
	out := make([]Item, len(list))
	for i, x := range list {
		out[i] = x.Item
	}
	return out
}
