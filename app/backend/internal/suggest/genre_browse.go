package suggest

import "sort"

// GenreRowMax is the most cards the genre row holds. GenreRowMin is the
// fewest it needs to show; below that the row is left out as too thin.
const (
	GenreRowMax = 16
	GenreRowMin = 6
)

// kidGenres are the genres a kid's row may show.
var kidGenres = map[string]bool{"Family": true, "Animation": true, "Kids": true, "Comedy": true, "Adventure": true,
	"Action & Adventure": true, "Fantasy": true, "Sci-Fi & Fantasy": true, "Music": true, "Documentary": true}

// GenreCard is one card of the genre row: a genre of one kind with a
// backdrop taken from a popular title of that genre on the page.
type GenreCard struct {
	Kind     string `json:"kind"`
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Backdrop string `json:"backdrop"`
	// From is the title the backdrop came from, as kind:tmdb id.
	From string `json:"from"`
	// Both marks a genre page that holds movies and shows.
	Both bool `json:"both,omitempty"`
}

// BrowseGenres picks the genre cards for a tab from the rows already built,
// with no outside call. Liked genres lead in taste order, the rest follow by
// how much of the page they fill weighted by popularity. Each backdrop is
// used once, so a genre with no fresh backdrop is dropped. Kids only get
// kid-safe genres.
func BrowseGenres(pg Page, sw string, taste []string, kid bool) []GenreCard {
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
	// Under All a genre both kinds share becomes one card, as the kind with more of it.
	best := map[string]*cand{}
	for _, c := range by {
		if b := best[c.name]; b == nil || c.weight > b.weight || (c.weight == b.weight && c.kind < b.kind) {
			best[c.name] = c
		}
	}
	rank := map[string]int{}
	for i, g := range taste {
		if _, ok := rank[g]; !ok {
			rank[g] = i
		}
	}
	list := make([]*cand, 0, len(best))
	for _, c := range best {
		list = append(list, c)
	}
	sort.Slice(list, func(i, j int) bool {
		ri, iok := rank[list[i].name]
		rj, jok := rank[list[j].name]
		switch {
		case iok && jok && ri != rj:
			return ri < rj
		case iok != jok:
			return iok
		case list[i].weight != list[j].weight:
			return list[i].weight > list[j].weight
		}
		return list[i].name < list[j].name
	})
	used := map[string]bool{}
	out := []GenreCard{}
	for _, c := range list {
		if len(out) == GenreRowMax {
			break
		}
		items := append([]Item{}, c.items...)
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].Pop != items[j].Pop {
				return items[i].Pop > items[j].Pop
			}
			return items[i].Key() < items[j].Key()
		})
		for _, it := range items {
			if !used[it.Backdrop] {
				used[it.Backdrop] = true
				out = append(out, GenreCard{Kind: c.kind, ID: genreID(c.kind, c.name), Name: c.name, Backdrop: it.Backdrop, From: it.Key(), Both: sw == "all" && ShowGenreFor(c.name) != ""})
				break
			}
		}
	}
	if len(out) < GenreRowMin {
		return []GenreCard{}
	}
	return out
}

// genreID returns the TMDB id of genre name for kind, or 0 when unknown.
func genreID(kind, name string) int {
	for id, n := range Genres[kind] {
		if n == name {
			return id
		}
	}
	return 0
}
