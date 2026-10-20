package explore

import "testing"

func TestBrowseTilesGenresMoodsAndKids(t *testing.T) {
	it := func(id, kind string, pop float64, g ...string) Item {
		return Item{TMDBID: id, Kind: kind, Name: id, Backdrop: "/b" + id, Pop: pop, Genres: g}
	}
	pg := Page{Head: []Row{{Items: []Item{
		it("1", "movie", 90, "Crime"), it("2", "movie", 80, "Crime"), it("3", "movie", 70, "Crime"),
		it("4", "movie", 60, "Animation"), it("5", "movie", 50, "Animation"), it("6", "show", 40, "Animation"),
	}}}}
	moods := func(g string) []string {
		return map[string][]string{"Crime": {"Heist", "Serial killers"}, "Animation": {"Anime", "Adult animation"}}[g]
	}
	got := BrowseTiles(pg, "movies", false, moods)
	names := []string{}
	for _, x := range got {
		names = append(names, x.Name)
	}
	want := "[Crime Heist Serial killers Animation Anime]"
	if s := sprint(names); s != want {
		t.Fatalf("tiles %s, want %s", s, want)
	}
	if got[1].Mood != "Heist" || got[1].Genre != 80 || got[1].Backdrop != "/b2" {
		t.Fatalf("mood tile %+v", got[1])
	}
	kid := BrowseTiles(pg, "all", true, moods)
	for _, x := range kid {
		if x.GenreName == "Crime" || x.Mood == "Adult animation" {
			t.Fatalf("kid saw %+v", x)
		}
	}
	if len(kid) == 0 {
		t.Fatal("kid got no tiles")
	}
}

func sprint(s []string) string {
	out := "["
	for i, x := range s {
		if i > 0 {
			out += " "
		}
		out += x
	}
	return out + "]"
}
