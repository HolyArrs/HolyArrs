package explore

import "sort"

// Genres are TMDB's genre ids and names per kind (TMDB's own fixed lists),
// for the genre picker and genre pages.
var Genres = map[string]map[int]string{
	"movie": {28: "Action", 12: "Adventure", 16: "Animation", 35: "Comedy", 80: "Crime", 99: "Documentary", 18: "Drama",
		10751: "Family", 14: "Fantasy", 36: "History", 27: "Horror", 10402: "Music", 9648: "Mystery", 10749: "Romance",
		878: "Science Fiction", 53: "Thriller", 10752: "War", 37: "Western"},
	"show": {10759: "Action & Adventure", 16: "Animation", 35: "Comedy", 80: "Crime", 99: "Documentary", 18: "Drama",
		10751: "Family", 10762: "Kids", 9648: "Mystery", 10764: "Reality", 10765: "Sci-Fi & Fantasy", 10768: "War & Politics", 37: "Western"},
}

// GenreName is the name of genre id for kind, "" when unknown.
func GenreName(kind string, id int) string { return Genres[kind][id] }

// HasGenre says the item lists the genre.
func HasGenre(it Item, genre string) bool {
	for _, g := range it.Genres {
		if g == genre {
			return true
		}
	}
	return false
}

// MergeTop merges lists by popularity (highest first), a title once, and
// keeps the first n.
func MergeTop(lists [][]Item, n int) []Item {
	var all []Item
	seen := map[string]bool{}
	for _, l := range lists {
		for _, it := range l {
			if !seen[it.Key()] {
				seen[it.Key()] = true
				all = append(all, it)
			}
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].Pop > all[j].Pop })
	if len(all) > n {
		all = all[:n]
	}
	return all
}

// Interleave takes one from each list in turn (rank 1s first), a title
// once, and keeps the first n.
func Interleave(lists [][]Item, n int) []Item {
	out := uniq(interleave(lists))
	if len(out) > n {
		out = out[:n]
	}
	return out
}
