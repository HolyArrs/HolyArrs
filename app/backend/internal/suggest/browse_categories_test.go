package suggest

import (
	"strconv"
	"testing"
	"time"
)

func TestCategories_TableIsValid(t *testing.T) {
	ids := map[string]bool{}
	sections := map[string]bool{}
	for _, s := range BrowseSections {
		sections[s.ID] = true
	}
	for _, c := range Categories {
		if ids[c.ID] {
			t.Fatalf("category id %s twice", c.ID)
		}
		ids[c.ID] = true
		if !sections[c.Section] || c.Name == "" || (c.Movie == nil && c.Show == nil) {
			t.Fatalf("bad category %+v", c)
		}
		for _, q := range []*CatQuery{c.Movie, c.Show} {
			if q != nil && len(q.Keywords) > 20 {
				t.Fatalf("%s: too many keywords", c.ID)
			}
		}
	}
	oct := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	for _, sw := range []string{"movies", "shows", "all"} {
		n := 0
		names := map[string]bool{}
		for _, s := range BrowseSections {
			for _, c := range SectionCategories(s.ID, sw, false, oct) {
				n++
				names[c.Name] = true
			}
		}
		if n < 90 || n > 170 {
			t.Fatalf("%s: %d categories", sw, n)
		}
	}
	for _, c := range SectionCategories("collections", "movies", false, oct) {
		if c.ID == "holiday" || c.ID == "limited" {
			t.Fatalf("%s in October movies", c.ID)
		}
	}
	dec := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	found := false
	for _, c := range SectionCategories("collections", "movies", false, dec) {
		found = found || c.ID == "holiday"
	}
	if !found {
		t.Fatal("no Holiday in December")
	}
}

func TestSectionCategories_TVSplitsAndExtras(t *testing.T) {
	now := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	got := map[string]Request{}
	for _, c := range SectionCategories("genres", "shows", false, now) {
		r := c.Requests("shows", false)
		if len(r) != 1 || r[0].Kind != "show" {
			t.Fatalf("%s requests %+v", c.ID, r)
		}
		got[c.Name] = r[0]
	}
	for _, n := range []string{"Action", "Adventure", "Sci-Fi", "Fantasy", "War", "Politics", "Sitcoms", "Medical", "Legal", "Cop procedurals", "Teen", "Soaps", "Talk & late night", "Horror", "Thriller", "Romance", "History"} {
		if _, ok := got[n]; !ok {
			t.Fatalf("TV genres miss %s", n)
		}
	}
	if got["Adventure"].Genres != "10759" || got["Adventure"].Keywords == "" || got["Sci-Fi"].Genres != "10765" || got["Politics"].Genres != "10768" {
		t.Fatalf("splits %+v %+v %+v", got["Adventure"], got["Sci-Fi"], got["Politics"])
	}
	movies := map[string]bool{}
	for _, c := range SectionCategories("genres", "movies", false, now) {
		movies[c.Name] = true
	}
	for _, n := range []string{"Spy", "Sports", "Musicals", "Disaster"} {
		if !movies[n] {
			t.Fatalf("movie genres miss %s", n)
		}
	}
	if movies["Sitcoms"] {
		t.Fatal("Sitcoms under movies")
	}
	all := SectionCategories("genres", "all", false, now)
	names := map[string]bool{}
	for _, c := range all {
		if names[c.Name] {
			t.Fatalf("%s twice under All", c.Name)
		}
		names[c.Name] = true
	}
	if r := (Category{Movie: &CatQuery{Genres: "28"}, Show: &CatQuery{Genres: "10759"}}).Requests("all", false); len(r) != 2 {
		t.Fatalf("all requests %+v", r)
	}
}

func TestCategories_KidSafe(t *testing.T) {
	now := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	for _, s := range BrowseSections {
		for _, sw := range []string{"movies", "shows", "all"} {
			for _, c := range SectionCategories(s.ID, sw, true, now) {
				if !c.Kid {
					t.Fatalf("kid got %s", c.ID)
				}
				for _, r := range c.Requests(sw, true) {
					if r.Without != kidWithout || (r.Kind == "movie" && r.Cert != "PG") {
						t.Fatalf("kid request %+v", r)
					}
				}
			}
		}
	}
	for _, id := range []string{"horror", "serial-killers", "revenge", "dark", "zombies", "assassins"} {
		c, _ := CategoryByID(id)
		if c.Kid {
			t.Fatalf("%s kid-safe", id)
		}
	}
}

func TestPickCategoryTiles_MinAndDedupe(t *testing.T) {
	mk := func(prefix string, n int, pop float64) []Item {
		var out []Item
		for i := 0; i < n; i++ {
			out = append(out, Item{TMDBID: prefix + strconv.Itoa(i), Kind: "movie", Backdrop: "/b" + strconv.Itoa(i), Pop: pop - float64(i)})
		}
		return out
	}
	few := mk("f", 11, 500)
	res := []CategoryResult{
		{Cat: Category{ID: "a", Name: "A"}, Items: mk("a", 12, 100)},
		{Cat: Category{ID: "b", Name: "B"}, Items: mk("b", 13, 90)},
		{Cat: Category{ID: "f", Name: "Few"}, Items: few},
		{Cat: Category{ID: "s", Name: "Shown"}, Items: mk("s", 20, 999)},
	}
	used := map[string]bool{}
	got := PickCategoryTiles(res, nil, used, map[string]bool{"Shown": true})
	if len(got) != 2 || got[0].Category != "a" || got[1].Category != "b" {
		t.Fatalf("tiles %+v", got)
	}
	if got[0].Backdrop == got[1].Backdrop || !used[got[0].Backdrop] || !used[got[1].Backdrop] {
		t.Fatalf("backdrops %q %q", got[0].Backdrop, got[1].Backdrop)
	}
	// A category matching taste goes first.
	liked := mk("l", 12, 10)
	for i := range liked {
		liked[i].Backdrop = "/l" + strconv.Itoa(i)
		liked[i].Genres = []string{"Horror"}
	}
	got = PickCategoryTiles(append(res, CategoryResult{Cat: Category{ID: "l", Name: "L"}, Items: liked}), []string{"Horror"}, map[string]bool{}, nil)
	if got[0].Category != "l" {
		t.Fatalf("taste order %+v", got)
	}
}

func TestPageGenreTiles_MinTitles(t *testing.T) {
	var items []Item
	for i := 0; i < 12; i++ {
		items = append(items, Item{TMDBID: strconv.Itoa(i), Kind: "show", Backdrop: "/c" + strconv.Itoa(i), Pop: 50, Genres: []string{"Crime", "Sci-Fi & Fantasy"}})
	}
	items = append(items, Item{TMDBID: "x", Kind: "show", Backdrop: "/x", Genres: []string{"Western"}})
	got := PageGenreTiles(Page{Head: []Row{{Items: items}}}, "shows", false, map[string]bool{})
	if len(got) != 1 || got[0].Name != "Crime" {
		t.Fatalf("tiles %+v", got)
	}
}
