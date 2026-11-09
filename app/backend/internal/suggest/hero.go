package suggest

import (
	"encoding/binary"
	"hash/fnv"
	"math/rand"
	"sort"
)

// HeroPool is how many candidates the hero looks at before the quality gate.
// HeroCount is how many it shows.
const (
	HeroPool  = 15
	HeroCount = 6
	// HeroShuffleTop is how many top-ranked titles the daily shuffle draws from.
	// On All it is also what each kind leaves to the Movies and TV heroes.
	HeroShuffleTop = 10
	// heroTasteBoost is the most a full taste match adds to a rank score; a
	// small push so position in the source row still decides most orders.
	heroTasteBoost = 0.35
	// LeanShare is the share of the household's Library and asks a genre needs
	// before the hero shows it despite heroQuiet.
	LeanShare = 0.10
)

// heroQuiet are the genres the hero leaves out unless the household leans on them.
var heroQuiet = []string{"Animation", "Kids", "Family", "Documentary", "Reality"}

// HeroExcluded reports whether the hero skips a title for its genres: one of
// heroQuiet that the household does not lean on.
func HeroExcluded(genres, leans []string) bool {
	for _, g := range genres {
		for _, q := range heroQuiet {
			if g == q && !hasString(leans, g) {
				return true
			}
		}
	}
	return false
}

// HeroCandidates returns up to n titles not in the Library, from Trending then
// Trending on streaming, that have known genres and pass HeroExcluded.
func HeroCandidates(head []Row, leans []string, n int) []Item {
	var out []Item
	seen := map[string]bool{}
	for _, t := range []string{TypeTrending, TypeTopServices} {
		for _, row := range head {
			if row.Type != t {
				continue
			}
			for _, it := range row.Items {
				if len(out) >= n {
					return out
				}
				if it.Owned || seen[it.Key()] || len(it.Genres) == 0 || HeroExcluded(it.Genres, leans) {
					continue
				}
				seen[it.Key()] = true
				out = append(out, it)
			}
		}
	}
	return out
}

// HeroTaste returns the share of the household's taste genres the title has, 0 to 1.
func HeroTaste(it Item, taste []string) float64 {
	if len(taste) == 0 {
		return 0
	}
	n := 0
	for _, g := range taste {
		if hasString(it.Genres, g) {
			n++
		}
	}
	return float64(n) / float64(len(taste))
}

// HeroRank orders candidates by place in the source rows plus a small taste
// boost. Ties keep the source order.
func HeroRank(cands []Item, taste []string) []Item {
	out := append([]Item(nil), cands...)
	score := map[string]float64{}
	for i, it := range out {
		pos := 1.0
		if len(out) > 1 {
			pos = 1 - float64(i)/float64(len(out)-1)
		}
		score[it.Key()] = pos + heroTasteBoost*HeroTaste(it, taste)
	}
	sort.SliceStable(out, func(i, j int) bool { return score[out[i].Key()] > score[out[j].Key()] })
	return out
}

// HeroShuffle shuffles the first HeroShuffleTop titles with a seed from the
// member and the day, so the hero changes daily but holds within a day.
func HeroShuffle(items []Item, member int64, date string) []Item {
	out := append([]Item(nil), items...)
	h := fnv.New64a()
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], uint64(member))
	h.Write(b[:])
	h.Write([]byte("|hero|" + date))
	top := min(len(out), HeroShuffleTop)
	rand.New(rand.NewSource(int64(h.Sum64()))).Shuffle(top, func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}

// PickHero picks HeroCount titles from ranked candidates, shuffled per member
// and day. On All it takes each kind's titles beyond the first HeroShuffleTop,
// so the kind heroes keep their own, and alternates movie and show. It uses
// the held-back titles only when too few are left.
func PickHero(ranked []Item, all bool, member int64, date string) []Item {
	if !all {
		out := HeroShuffle(ranked, member, date)
		return out[:min(len(out), HeroCount)]
	}
	seen := map[string]int{}
	byKind := map[string][]Item{}
	var spare []Item
	for _, it := range ranked {
		if seen[it.Kind] < HeroShuffleTop {
			seen[it.Kind]++
			spare = append(spare, it)
			continue
		}
		byKind[it.Kind] = append(byKind[it.Kind], it)
	}
	for k := range byKind {
		byKind[k] = HeroShuffle(byKind[k], member, date+"|"+k)
	}
	var out []Item
	for i := 0; len(out) < HeroCount && (i < len(byKind["movie"]) || i < len(byKind["show"])); i++ {
		for _, k := range []string{"movie", "show"} {
			if i < len(byKind[k]) && len(out) < HeroCount {
				out = append(out, byKind[k][i])
			}
		}
	}
	for _, it := range spare {
		if len(out) >= HeroCount {
			break
		}
		out = append(out, it)
	}
	return out
}

// hasString reports whether list holds s.
func hasString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
