package explore

import (
	"sort"
	"time"
)

// New & Popular row types. The page's rows are all within "new": titles
// out (or added to a service) in about the last 90 days.
const (
	TypeNPServices = "np_services"
	TypeNPTopShows = "np_top_shows"
	TypeNPTopFilms = "np_top_movies"
	TypeNPReturn   = "np_returning"
	TypeNPEpisodes = "np_episodes"
	TypeNPCinema   = "np_cinema"
	TypeNPReleased = "np_released"
	TypeNPSeries   = "np_series"
	TypeNPGenre    = "np_genre"
	TypeNPPeople   = "np_people"
	TypeNPPicks    = "np_picks"
	TypeNPWeek     = "np_week"
	TypeNPAcclaim  = "np_acclaimed"
	TypeNPWorld    = "np_world"
)

// NewRowMin is the fewest good titles a New & Popular row needs.
const NewRowMin = 6

// NewWindow is how far back "new" reaches.
const NewWindow = 90 * 24 * time.Hour

// NewFixed are the row types that keep their place at the top of the page;
// the others are ordered by how much they suit the household.
var NewFixed = map[string]bool{TypeNPServices: true, TypeNPTopShows: true, TypeNPTopFilms: true}

// IsNew says a title came out within the window up to now (an unknown date
// is let through for shows, whose first air date is often years back).
func IsNew(it Item, now time.Time) bool {
	d := ParseDay(it.Released)
	if d.IsZero() {
		return it.Kind == "show"
	}
	return !d.After(now) && now.Sub(d) <= NewWindow
}

// FilterNew keeps the titles that are new at now.
func FilterNew(items []Item, now time.Time) []Item {
	out := []Item{}
	for _, it := range items {
		if IsNew(it, now) {
			out = append(out, it)
		}
	}
	return out
}

// DedupeRows keeps each title once on the page (the first row wins), caps
// rows at RowMax and drops rows left under min.
func DedupeRows(rows []Row, min int) []Row {
	seen := map[string]bool{}
	out := []Row{}
	for _, r := range rows {
		var keep []Item
		in := map[string]bool{}
		for _, it := range r.Items {
			if it.TMDBID == "" || seen[it.Key()] || in[it.Key()] || len(keep) >= RowMax {
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

// OrderNewRows puts the fixed rows first (in their order), then the rest
// by the mean suit of their first 8 titles plus the row's own weight; ties
// keep their order.
func OrderNewRows(rows []Row, suit func(Item) float64, weight map[string]float64) []Row {
	var fixed, rest []Row
	for _, r := range rows {
		if NewFixed[r.Type] {
			fixed = append(fixed, r)
		} else {
			rest = append(rest, r)
		}
	}
	score := func(r Row) float64 {
		n, sum := 0, 0.0
		for _, it := range r.Items {
			if n == 8 {
				break
			}
			sum += suit(it)
			n++
		}
		if n > 0 {
			sum /= float64(n)
		}
		return sum + weight[r.Type]
	}
	sort.SliceStable(rest, func(i, j int) bool { return score(rest[i]) > score(rest[j]) })
	return append(fixed, rest...)
}

// RankMerge takes turns between ranked lists (each in rank order), each
// title once, up to n.
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
