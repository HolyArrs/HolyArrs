package suggest

import (
	"fmt"
	"testing"
)

func TestBrowseGenres_TasteFirstThenPopular(t *testing.T) {
	got := names(BrowseGenres(browsePage(movieSpread()...), "movies", []string{"Western", "Horror"}, false))
	want := []string{"Western", "Horror", "Drama", "Comedy", "Action", "Thriller", "Crime", "Romance"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestBrowseGenres_NoDuplicateBackdrops(t *testing.T) {
	shared := browseItem("movie", 900, 999, "Drama", "Comedy", "Action", "Thriller", "Horror", "Crime", "Romance", "Western")
	cards := BrowseGenres(browsePage(append(movieSpread(), shared)...), "movies", nil, false)
	seen := map[string]bool{}
	for _, c := range cards {
		if seen[c.Backdrop] {
			t.Fatalf("backdrop %s used twice: %+v", c.Backdrop, cards)
		}
		seen[c.Backdrop] = true
	}
	if cards[0].Backdrop != shared.Backdrop {
		t.Fatalf("first card should take the most popular title, got %+v", cards[0])
	}
}

func TestBrowseGenres_KindAndIDs(t *testing.T) {
	items := movieSpread()
	items = append(items, browseItem("show", 1, 50, "Reality"), browseItem("show", 2, 50, "Drama"))
	cards := BrowseGenres(browsePage(items...), "movies", nil, false)
	for _, c := range cards {
		if c.Kind != "movie" || c.Name == "Reality" || GenreName(c.Kind, c.ID) != c.Name {
			t.Fatalf("Movies tab card %+v", c)
		}
	}
	all := BrowseGenres(browsePage(items...), "all", nil, false)
	drama := 0
	for _, c := range all {
		if c.Name == "Drama" {
			drama++
			if c.Kind != "movie" {
				t.Fatalf("Drama should be the kind with more of it: %+v", c)
			}
		}
	}
	if drama != 1 || len(names(all)) != 9 {
		t.Fatalf("All tab = %v", names(all))
	}
}

func TestBrowseGenres_KidSafe(t *testing.T) {
	items := movieSpread()
	for i, g := range []string{"Family", "Animation", "Adventure", "Fantasy", "Music", "Documentary"} {
		items = append(items, browseItem("movie", 500+i, 20, g))
	}
	for _, c := range BrowseGenres(browsePage(items...), "movies", []string{"Horror"}, true) {
		if !kidGenres[c.Name] {
			t.Fatalf("Kid got %s", c.Name)
		}
	}
}

func TestBrowseGenres_TooFewAndCap(t *testing.T) {
	few := BrowseGenres(browsePage(browseItem("movie", 1, 1, "Drama", "Comedy")), "movies", nil, false)
	if len(few) != 0 {
		t.Fatalf("under the minimum should be empty, got %v", names(few))
	}
	var items []Item
	for id, g := range Genres["movie"] {
		items = append(items, browseItem("movie", id, 1, g))
	}
	if got := BrowseGenres(browsePage(items...), "movies", nil, false); len(got) != GenreRowMax {
		t.Fatalf("cap: got %d", len(got))
	}
}

func TestBrowseGenres_MergeShowCombinedUnderAll(t *testing.T) {
	items := append(movieSpread(),
		browseItem("show", 500, 50, "Sci-Fi & Fantasy"),
		browseItem("show", 501, 60, "Action & Adventure"),
		browseItem("show", 502, 70, "War & Politics"),
		browseItem("movie", 503, 40, "Fantasy"))
	cards := BrowseGenres(browsePage(items...), "all", nil, false)
	by := map[string]GenreCard{}
	for _, c := range cards {
		by[c.Name] = c
	}
	for _, n := range []string{"Sci-Fi & Fantasy", "Action & Adventure", "War & Politics"} {
		if _, ok := by[n]; ok {
			t.Fatalf("%q still a card under All: %v", n, names(cards))
		}
	}
	if c, ok := by["Fantasy"]; !ok || c.Kind != "movie" || c.ID != 14 || !c.Both {
		t.Fatalf("Fantasy card = %+v", c)
	}
	if c := by["Action"]; !c.Both || c.ID != 28 {
		t.Fatalf("Action card = %+v", c)
	}
	if by["Drama"].Both {
		t.Fatal("Drama is not merged")
	}
	for _, c := range BrowseGenres(browsePage(items...), "shows", nil, false) {
		if c.Both {
			t.Fatalf("Both on Shows: %+v", c)
		}
	}
}

func browseItem(kind string, id int, pop float64, genres ...string) Item {
	return Item{TMDBID: fmt.Sprint(id), Kind: kind, Name: fmt.Sprint("t", id), Backdrop: fmt.Sprintf("/b%s%d.jpg", kind, id), Pop: pop, Genres: genres}
}

func browsePage(items ...Item) Page {
	return Page{Head: []Row{{ID: "a", Type: TypeTrending, Items: items}}}
}

func names(cards []GenreCard) []string {
	out := []string{}
	for _, c := range cards {
		out = append(out, c.Name)
	}
	return out
}

func movieSpread() []Item {
	gs := []string{"Drama", "Comedy", "Action", "Thriller", "Horror", "Crime", "Romance", "Western"}
	var items []Item
	id := 1
	for i, g := range gs {
		for n := 0; n < len(gs)-i; n++ {
			items = append(items, browseItem("movie", id, float64(10+id), g))
			id++
		}
	}
	return items
}
