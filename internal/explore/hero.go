package explore

import (
	"encoding/binary"
	"hash/fnv"
	"math/rand"
	"sort"
)

// HeroPool is how many candidates the hero looks at before the quality gate
// (trailer, logo, backdrop); HeroCount is how many it shows.
const (
	HeroPool  = 15
	HeroCount = 6
	// HeroShuffleTop is how many of the best-ranked titles the daily shuffle
	// draws from (and, per kind, what All leaves to the Movies and TV heroes).
	HeroShuffleTop = 10
	// heroTasteBoost is what a full taste match adds to a candidate's rank
	// score (1 for the first candidate down to 0 for the last).
	heroTasteBoost = 0.35
)

// heroQuiet are the genres the hero leaves out unless the household leans
// on them.
var heroQuiet = []string{"Animation", "Kids", "Family", "Documentary", "Reality"}

// LeanShare is the share of the household's Library and asks a genre needs
// before the hero shows it despite heroQuiet.
const LeanShare = 0.10

// HeroExcluded says whether the hero leaves the title out for its genres:
// one of heroQuiet the household does not lean on (leans: genres at
// LeanShare or more).
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

// HeroCandidates are up to n titles not in the Library from Trending, then
// Trending on streaming, without the genres HeroExcluded leaves out.
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
				if it.Owned || seen[it.Key()] || HeroExcluded(it.Genres, leans) {
					continue
				}
				seen[it.Key()] = true
				out = append(out, it)
			}
		}
	}
	return out
}

// HeroTaste is the share of the household's taste genres the title has, 0-1.
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

// HeroRank orders candidates by their place in the source rows plus a small
// taste boost; ties keep the source order.
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
// member and the day, so the hero changes daily and holds within a day.
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

// PickHero picks HeroCount titles from ranked, ready candidates, shuffled
// for the member and day (HeroShuffle). On All it leaves out each kind's
// first HeroShuffleTop (what the Movies and TV shows heroes draw from) and
// alternates a movie and a show; it falls back to the left-out ones only
// when too few are left.
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

func hasString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
