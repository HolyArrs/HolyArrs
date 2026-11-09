package suggest

import "sort"

// BrowseTile is one tile of the Search browse grid: a genre of one kind, or one
// of that genre's mood rows, with a backdrop from a popular title of the genre.
type BrowseTile struct {
	Kind  string `json:"kind"`
	Genre int    `json:"genre"`
	// GenreName is the genre's TMDB name. Name is the tile's own label, the
	// genre name or the mood row title.
	GenreName string `json:"genre_name"`
	Mood      string `json:"mood,omitempty"`
	Name      string `json:"name"`
	Backdrop  string `json:"backdrop"`
	From      string `json:"from"`
	// Both marks a genre page that holds movies and shows.
	Both bool `json:"both,omitempty"`
}

// kidMoodsOff are mood rows a kid never sees, even under a kid-safe genre.
var kidMoodsOff = map[string]bool{"Adult animation": true, "True crime": true, "Serial killers": true, "Revenge": true}

// BrowseTiles picks the Search browse tiles from the rows already built, with
// no outside call. Every genre of the tab's kinds that has a backdrop title
// comes biggest first, each followed by its mood rows from moods. A name shows
// once, so under All a genre both kinds have goes to the earlier kind. Each
// backdrop is used once, and a tile with no fresh backdrop is dropped.
func BrowseTiles(pg Page, sw string, kid bool, moods func(genre string) []string) []BrowseTile {
	kinds := Kinds(sw)
	type cand struct {
		kind, name string
		weight     float64
		items      []Item
	}
	by := map[string]*cand{}
	seen := map[string]bool{}
	for _, rows := range [][]Row{pg.Head, pg.Tail} {
		for _, r := range rows {
			for _, it := range r.Items {
				if it.Backdrop == "" || !kindIn(kinds, it.Kind) || seen[it.Key()] {
					continue
				}
				seen[it.Key()] = true
				for _, g0 := range it.Genres {
					kind, names := mergeGenre(sw, it.Kind, g0)
					for _, g := range names {
						if genreID(kind, g) == 0 || (kid && !kidGenres[g]) {
							continue
						}
						k := kind + "|" + g
						c := by[k]
						if c == nil {
							c = &cand{kind: kind, name: g}
							by[k] = c
						}
						c.weight += 1 + it.Pop/100
						c.items = append(c.items, it)
					}
				}
			}
		}
	}
	list := make([]*cand, 0, len(by))
	for _, c := range by {
		sort.SliceStable(c.items, func(i, j int) bool {
			if c.items[i].Pop != c.items[j].Pop {
				return c.items[i].Pop > c.items[j].Pop
			}
			return c.items[i].Key() < c.items[j].Key()
		})
		list = append(list, c)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].weight != list[j].weight {
			return list[i].weight > list[j].weight
		}
		if list[i].name != list[j].name {
			return list[i].name < list[j].name
		}
		return list[i].kind < list[j].kind
	})
	// pick takes the most popular title whose backdrop no earlier tile uses,
	// preferring ones that are not dark.
	used := map[string]bool{}
	named := map[string]bool{}
	pick := func(c *cand) (Item, bool) {
		for _, it := range darkLast(c.items) {
			if !used[it.Backdrop] {
				used[it.Backdrop] = true
				return it, true
			}
		}
		return Item{}, false
	}
	out := []BrowseTile{}
	for _, c := range list {
		if named[c.name] {
			continue
		}
		it, ok := pick(c)
		if !ok {
			continue
		}
		named[c.name] = true
		id := genreID(c.kind, c.name)
		both := sw == "all" && ShowGenreFor(c.name) != ""
		out = append(out, BrowseTile{Kind: c.kind, Genre: id, GenreName: c.name, Name: c.name, Backdrop: it.Backdrop, From: it.Key(), Both: both})
		if moods == nil {
			continue
		}
		for _, m := range moods(c.name) {
			if (kid && kidMoodsOff[m]) || named[m] {
				continue
			}
			mi, ok := pick(c)
			if !ok {
				break
			}
			named[m] = true
			out = append(out, BrowseTile{Kind: c.kind, Genre: id, GenreName: c.name, Mood: m, Name: m, Backdrop: mi.Backdrop, From: mi.Key(), Both: both})
		}
	}
	return out
}
