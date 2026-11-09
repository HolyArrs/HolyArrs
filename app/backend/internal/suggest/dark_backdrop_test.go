package suggest

import "testing"

func TestPickCategoryTiles_SkipsDarkBackdrop(t *testing.T) {
	DarkBackdrop = func(b string) bool { return b == "dark.jpg" }
	defer func() { DarkBackdrop = nil }()
	items := []Item{{TMDBID: "1", Kind: "movie", Backdrop: "dark.jpg", Pop: 99}}
	for i := 2; i < BrowseMin+2; i++ {
		items = append(items, Item{TMDBID: string(rune('a' + i)), Kind: "movie", Backdrop: string(rune('a'+i)) + ".jpg", Pop: float64(50 - i)})
	}
	res := []CategoryResult{{Cat: Category{ID: "thriller", Name: "Thriller"}, Items: items}}
	tiles := PickCategoryTiles(res, nil, map[string]bool{}, map[string]bool{})
	if len(tiles) != 1 || tiles[0].Backdrop == "dark.jpg" || tiles[0].Backdrop != items[1].Backdrop {
		t.Fatalf("tiles = %+v", tiles)
	}
	if got := darkLast([]Item{{Backdrop: "dark.jpg"}}); len(got) != 1 {
		t.Fatalf("dark-only list lost its item: %+v", got)
	}
}
