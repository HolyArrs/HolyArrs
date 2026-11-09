package suggest

import "sort"

// Genres holds TMDB's fixed genre ids and names per kind, for the genre picker
// and genre pages.
var Genres = map[string]map[int]string{
	"movie": {28: "Action", 12: "Adventure", 16: "Animation", 35: "Comedy", 80: "Crime", 99: "Documentary", 18: "Drama",
		10751: "Family", 14: "Fantasy", 36: "History", 27: "Horror", 10402: "Music", 9648: "Mystery", 10749: "Romance",
		878: "Science Fiction", 53: "Thriller", 10752: "War", 37: "Western"},
	"show": {10759: "Action & Adventure", 16: "Animation", 35: "Comedy", 80: "Crime", 99: "Documentary", 18: "Drama",
		10751: "Family", 10762: "Kids", 9648: "Mystery", 10764: "Reality", 10765: "Sci-Fi & Fantasy", 10768: "War & Politics", 37: "Western"},
}

// showCombined maps a movie genre to the TV genre that merges it with another,
// because TMDB's TV genres are combined while its movie genres are separate.
var showCombined = map[string]string{"Science Fiction": "Sci-Fi & Fantasy", "Fantasy": "Sci-Fi & Fantasy",
	"Action": "Action & Adventure", "Adventure": "Action & Adventure", "War": "War & Politics"}

// movieSplit is the reverse of showCombined: the movie genres a combined TV
// genre stands for.
var movieSplit = map[string][]string{"Sci-Fi & Fantasy": {"Science Fiction", "Fantasy"},
	"Action & Adventure": {"Action", "Adventure"}, "War & Politics": {"War"}}

// ShowGenreFor returns the combined TV genre a movie genre merges into, or "".
func ShowGenreFor(movieGenre string) string { return showCombined[movieGenre] }

// GenreName returns the name of genre id for kind, or "" when unknown.
func GenreName(kind string, id int) string { return Genres[kind][id] }

// HasGenre reports whether the item lists the genre.
func HasGenre(it Item, genre string) bool {
	for _, g := range it.Genres {
		if g == genre {
			return true
		}
	}
	return false
}

// MergeTop merges the lists by popularity, highest first, keeps each title
// once and returns the first n.
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

// Interleave takes one title from each list in turn, keeps each title once and
// returns the first n.
func Interleave(lists [][]Item, n int) []Item {
	out := uniq(interleave(lists))
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// mergeGenre returns the kind and genre names an item's genre g counts for.
// Under All a combined TV genre counts as its movie genres so both kinds share
// one card.
func mergeGenre(sw, kind, g string) (string, []string) {
	if sw == "all" && kind == "show" {
		if m := movieSplit[g]; m != nil {
			return "movie", m
		}
	}
	return kind, []string{g}
}
