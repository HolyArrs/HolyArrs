package suggest

import (
	"math"
	"sort"
	"time"
)

// Row types of the New & Popular page. Every title on it is new in one way, within that way's window.
const (
	TypeNPServices = "np_services"
	TypeNPTopShows = "np_top_shows"
	TypeNPTopFilms = "np_top_movies"
	TypeNPEpisodes = "np_episodes"
	TypeNPPicks    = "np_picks"
	TypeNPSeries   = "np_series"
	TypeNPCinema   = "np_cinema"
	TypeNPGenre    = "np_genre"
	TypeNPAcclaim  = "np_acclaimed"
	TypeNPWatching = "np_watching"
	TypeNPWorld    = "np_world"
)

// NewRowMin is the fewest good titles a New & Popular row needs.
const NewRowMin = 6

// Kinds of newness. A title carries the one that makes it newest.
const (
	NewService  = "service"  // added to a streaming service
	NewRelease  = "release"  // out (digital or cinema)
	NewSeason   = "season"   // a new season started
	NewEpisode  = "episode"  // a new episode aired
	NewSeries   = "series"   // a brand-new series (first season)
	NewUpcoming = "upcoming" // out in the next two weeks
)

// newWindows is how far back each kind counts as new, or ahead for upcoming.
var newWindows = map[string]time.Duration{
	NewService:  14 * day,
	NewRelease:  45 * day,
	NewSeason:   30 * day,
	NewEpisode:  14 * day,
	NewSeries:   60 * day,
	NewUpcoming: 14 * day,
}

// newTau is each kind's decay time: freshness falls to 1/e after it.
var newTau = map[string]time.Duration{
	NewService: 7 * day,
	NewEpisode: 7 * day,
	NewRelease: 14 * day,
	NewSeason:  14 * day,
	NewSeries:  21 * day,
}

// NewRowCap is a row type's own cap when it is not RowMax. The services row
// holds about 20 titles per service for its chips.
var NewRowCap = map[string]int{TypeNPServices: 120, TypeNPTopShows: 10, TypeNPTopFilms: 10}

// NewFixed are the row types that keep the top of the page in build order:
// the two Top 10s, what everyone is watching, then the critics' best. The rest
// are ordered by score.
var NewFixed = map[string]bool{TypeNPTopShows: true, TypeNPTopFilms: true, TypeNPWatching: true, TypeNPAcclaim: true}

// NewFact is why a title is new: its kind and the day it happened, or will
// happen for upcoming. Season is the season number for NewSeason.
type NewFact struct {
	Kind   string
	At     time.Time
	Season int
}

// Freshness returns exp(-age/tau) inside the kind's window and 0 outside it.
// An upcoming title within its window counts as 1.
func Freshness(f NewFact, now time.Time) float64 {
	w := newWindows[f.Kind]
	if w == 0 || f.At.IsZero() {
		return 0
	}
	if f.Kind == NewUpcoming {
		if f.At.After(now) && f.At.Sub(now) <= w {
			return 1
		}
		return 0
	}
	age := now.Sub(f.At)
	if age < 0 {
		// Earlier today (the day stamp is midnight) or a clock ahead of the source.
		if age > -day {
			age = 0
		} else {
			return 0
		}
	}
	if age > w {
		return 0
	}
	return math.Exp(-float64(age) / float64(newTau[f.Kind]))
}

// InWindow reports whether the fact is inside its kind's window at now.
func InWindow(f NewFact, now time.Time) bool { return Freshness(f, now) > 0 }

// NewScore returns F*(0.35+0.65*P)*(1+0.3*T): freshness first, then popularity
// within the pool (P), then fit to the household (T). P and T run 0 to 1.
func NewScore(f, p, t float64) float64 {
	return f * (0.35 + 0.65*clamp01(p)) * (1 + 0.3*clamp01(t))
}

// PopPercentile returns each title's popularity percentile within the pool, 1
// for the most popular. Ties share the higher place and a lone title is 1.
func PopPercentile(pool []Item) map[string]float64 {
	best := map[string]float64{}
	for _, it := range pool {
		if it.Pop > best[it.Key()] {
			best[it.Key()] = it.Pop
		} else if _, ok := best[it.Key()]; !ok {
			best[it.Key()] = it.Pop
		}
	}
	pops := make([]float64, 0, len(best))
	for _, p := range best {
		pops = append(pops, p)
	}
	sort.Float64s(pops)
	out := map[string]float64{}
	n := len(pops)
	for k, p := range best {
		if n <= 1 {
			out[k] = 1
			continue
		}
		// Titles at or below p, as a share of the rest.
		i := sort.Search(n, func(i int) bool { return pops[i] > p })
		out[k] = float64(i-1) / float64(n-1)
	}
	return out
}

// SortByScore orders items by score, best first. Ties keep their order.
func SortByScore(items []Item, score func(Item) float64) {
	sort.SliceStable(items, func(i, j int) bool { return score(items[i]) > score(items[j]) })
}

// DedupeRows keeps each title once on the page, first row wins, caps rows at
// RowMax or NewRowCap and drops rows left under min.
func DedupeRows(rows []Row, min int) []Row {
	seen := map[string]bool{}
	out := []Row{}
	for _, r := range rows {
		var keep []Item
		in := map[string]bool{}
		limit := RowMax
		if n := NewRowCap[r.Type]; n > 0 {
			limit = n
		}
		for _, it := range r.Items {
			if it.TMDBID == "" || seen[it.Key()] || in[it.Key()] || len(keep) >= limit {
				continue
			}
			in[it.Key()] = true
			keep = append(keep, it)
		}
		if len(keep) < min {
			continue
		}
		for _, it := range keep {
			seen[it.Key()] = true
		}
		r.Items = keep
		out = append(out, r)
	}
	return out
}

// OrderNewRows puts the NewFixed rows first, then the rest by the mean score
// of their first 8 titles times the row weight, 1 when unset. Ties keep order.
func OrderNewRows(rows []Row, score func(Item) float64, weight map[string]float64) []Row {
	var fixed, rest []Row
	for _, r := range rows {
		if NewFixed[r.Type] {
			fixed = append(fixed, r)
		} else {
			rest = append(rest, r)
		}
	}
	val := func(r Row) float64 {
		n, sum := 0, 0.0
		for _, it := range r.Items {
			if n == 8 {
				break
			}
			sum += score(it)
			n++
		}
		if n > 0 {
			sum /= float64(n)
		}
		w, ok := weight[r.Type]
		if !ok {
			w = 1
		}
		return sum * w
	}
	sort.SliceStable(rest, func(i, j int) bool { return val(rest[i]) > val(rest[j]) })
	return append(fixed, rest...)
}

// RankMerge takes turns between ranked lists, keeps each title once and
// returns up to n.
func RankMerge(lists [][]Item, n int) []Item {
	out := []Item{}
	seen := map[string]bool{}
	for i := 0; len(out) < n; i++ {
		more := false
		for _, l := range lists {
			if i >= len(l) {
				continue
			}
			more = true
			if !seen[l[i].Key()] && len(out) < n {
				seen[l[i].Key()] = true
				out = append(out, l[i])
			}
		}
		if !more {
			break
		}
	}
	return out
}

// CapPer keeps items in order with at most n per group and max in all.
func CapPer(items []Item, group func(Item) string, n, max int) []Item {
	out := []Item{}
	per := map[string]int{}
	for _, it := range items {
		g := group(it)
		if per[g] >= n || len(out) >= max {
			continue
		}
		per[g]++
		out = append(out, it)
	}
	return out
}

// clamp01 limits v to 0 through 1.
func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }
