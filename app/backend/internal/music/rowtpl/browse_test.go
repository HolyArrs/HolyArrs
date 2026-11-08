package rowtpl

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestBrowseSections_Layout(t *testing.T) {
	secs := BrowseSections(browseBundle(), time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC), "us", nil)
	want := map[string]int{"genres": 22, "moods": 12, "seasons": 12, "decades": 7, "activities": 8}
	total := 0
	used := map[string]string{}
	for _, s := range secs {
		if len(s.Tiles) != want[s.ID] {
			t.Errorf("%s: %d tiles, want %d", s.ID, len(s.Tiles), want[s.ID])
		}
		total += len(s.Tiles)
		for _, tile := range s.Tiles {
			if len(tile.Covers) == 0 {
				t.Errorf("%s: no cover", tile.Key)
				continue
			}
			c := tile.Covers[0]
			if o, dup := used[c]; dup && tile.Key != o {
				t.Errorf("%s and %s share cover %s", o, tile.Key, c)
			}
			used[c] = tile.Key
			if tile.Href == "" || tile.Name == "" {
				t.Errorf("%s: no href or name", tile.Key)
			}
		}
	}
	if total != 61 {
		t.Fatalf("tiles = %d, want 61", total)
	}
	if secs[2].ID != "seasons" || secs[2].Tiles[0].Key != "season:halloween" {
		t.Fatalf("seasons first tile = %s, want Halloween on Oct 24", secs[2].Tiles[0].Key)
	}
}

func TestBrowseSections_NeighbourHuesDiffer(t *testing.T) {
	for _, cols := range []int{2, 3, 4} {
		for _, s := range BrowseSections(browseBundle(), time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC), "us", nil) {
			hues := Hues(len(s.Tiles), cols)
			for i := range hues {
				if i%cols > 0 && hues[i] == hues[i-1] {
					t.Fatalf("%s at %d columns: tiles %d and %d share hue %d", s.ID, cols, i-1, i, hues[i])
				}
				if i >= cols && hues[i] == hues[i-cols] {
					t.Fatalf("%s at %d columns: tiles %d and %d share hue %d", s.ID, cols, i-cols, i, hues[i])
				}
			}
		}
	}
}

func TestExploreTiles_MixKinds(t *testing.T) {
	secs := BrowseSections(browseBundle(), time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC), "us", nil)
	got := ExploreTiles(secs)
	if len(got) != 18 {
		t.Fatalf("explore tiles = %d, want 18", len(got))
	}
	kinds := map[string]int{}
	seen := map[string]bool{}
	for _, tl := range got {
		kinds[kindOf(tl.Key)]++
		if seen[tl.Href] {
			t.Errorf("%s: page %s offered twice", tl.Key, tl.Href)
		}
		seen[tl.Href] = true
		if len(tl.Covers) == 0 {
			t.Errorf("%s: no cover", tl.Key)
		}
	}
	if kinds["genre"] != 8 || kinds["mood"] != 4 || kinds["season"] != 2 || kinds["decade"] != 2 || kinds["activity"] != 2 {
		t.Fatalf("kinds = %v, want 8 genres, 4 moods, 2 seasons, 2 decades, 2 activities", kinds)
	}
	for i, want := range []string{"genre", "genre", "mood", "genre", "season", "genre", "decade", "mood"} {
		if kindOf(got[i].Key) != want {
			t.Fatalf("tile %d is %s, want a %s", i, got[i].Key, want)
		}
	}
	if got[4].Key != "season:halloween" {
		t.Fatalf("season tile = %s, want Halloween on Oct 24", got[4].Key)
	}
	if got[6].Key != "decade:1990s" {
		t.Fatalf("decade tile = %s, want the 90s first", got[6].Key)
	}
	for _, tl := range got {
		if tl.Key == "activity:party" {
			t.Fatal("Party opens the mood page; it is offered once, as a mood")
		}
	}
}

func TestExploreTiles_ShortBundle(t *testing.T) {
	// Thin pages leave fewer tiles, so the mix skips the missing kinds.
	secs := []BrowseSection{{ID: "genres", Tiles: []BrowseTile{{Key: "genre:pop", Covers: []string{"c"}}, {Key: "genre:rock", Covers: []string{"c"}}}}, {ID: "decades", Tiles: []BrowseTile{{Key: "decade:1980s", Covers: []string{"c"}}}}}
	got := ExploreTiles(secs)
	if len(got) != 3 || got[0].Key != "genre:pop" || got[1].Key != "genre:rock" || got[2].Key != "decade:1980s" {
		t.Fatalf("got %+v", got)
	}
	if len(ExploreTiles(nil)) != 0 {
		t.Fatal("tiles from nothing")
	}
}

// TestBrowseSectionsArt_CoversNeedArt checks that covers come only from albums whose picture is stored or known to load.
func TestBrowseSectionsArt_CoversNeedArt(t *testing.T) {
	b := browseBundle()
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	// Only odd albums have art, so the shared top album 0 has none.
	hasArt := func(i int) bool { return i%2 == 1 }
	secs := BrowseSectionsArt(b, now, "us", nil, hasArt)
	if len(secs) == 0 {
		t.Fatal("no tiles")
	}
	firsts := map[string]string{}
	for _, s := range secs {
		for _, tl := range s.Tiles {
			if len(tl.Covers) == 0 {
				t.Errorf("%s: shown without a cover", tl.Key)
			}
			for _, c := range tl.Covers {
				if i := itemOfCover(b, c); i < 0 || !hasArt(i) {
					t.Errorf("%s: cover %s has no art", tl.Key, c)
				}
			}
			if len(tl.Covers) > 0 {
				if o, dup := firsts[tl.Covers[0]]; dup {
					t.Errorf("%s and %s start with the same cover", o, tl.Key)
				}
				firsts[tl.Covers[0]] = tl.Key
			}
		}
	}
}

func TestBrowseSectionsArt_TileWithoutArtLeftOut(t *testing.T) {
	b := browseBundle()
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	// Blues has no album with art, its own or the shared one.
	noArt := map[int]bool{0: true}
	for _, m := range b.Keys["genre:blues"] {
		noArt[m.I] = true
	}
	secs := BrowseSectionsArt(b, now, "us", nil, func(i int) bool { return !noArt[i] })
	shown := map[string]bool{}
	for _, s := range secs {
		for _, tl := range s.Tiles {
			shown[tl.Key] = true
		}
	}
	if shown["genre:blues"] {
		t.Fatal("a tile with no album with art is shown")
	}
	if !shown["genre:pop"] || !shown["genre:rock"] {
		t.Fatal("tiles that have art were left out")
	}
}

func TestBrowseSectionsArt_SectionWithoutArtLeftOut(t *testing.T) {
	b := browseBundle()
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	noArt := map[int]bool{0: true}
	for _, key := range []string{"decade:1960s", "decade:1970s", "decade:1980s", "decade:1990s", "decade:2000s", "decade:2010s", "decade:2020s"} {
		for _, m := range b.Keys[key] {
			noArt[m.I] = true
		}
	}
	for _, s := range BrowseSectionsArt(b, now, "us", nil, func(i int) bool { return !noArt[i] }) {
		if s.ID == "decades" {
			t.Fatal("a section with no tile with art is shown")
		}
	}
}

func TestBrowseSectionsArt_NoCheckKeepsAlbumTiles(t *testing.T) {
	secs := BrowseSectionsArt(browseBundle(), time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC), "us", nil, nil)
	n := 0
	for _, s := range secs {
		n += len(s.Tiles)
	}
	if n != 61 {
		t.Fatalf("%d tiles, want 61", n)
	}
	// A pool of artists only brings no cover, so its tile is left out.
	b := browseBundle()
	for _, m := range b.Keys["genre:jazz"] {
		b.Items[m.I].Kind = "r"
	}
	for _, s := range BrowseSections(b, time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC), "us", nil) {
		for _, tl := range s.Tiles {
			if tl.Key == "genre:jazz" {
				t.Fatal("a tile with no album is shown")
			}
		}
	}
}

func TestVisibleSections_ThinAndOutOfSeason(t *testing.T) {
	secs := BrowseSections(browseBundle(), time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC), "us", nil)
	counts := map[string]int{}
	for _, s := range secs {
		for _, tl := range s.Tiles {
			counts[poolKey(tl.Key)] = 20
		}
	}
	counts["genre:blues"] = 10       // thin: hidden
	counts["season:christmas"] = 30  // starts within 45 days: shown
	counts["season:valentines"] = 30 // far off: hidden
	vis := VisibleSections(secs, counts, time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC), "us")
	shown := map[string]bool{}
	for _, s := range vis {
		for _, tl := range s.Tiles {
			shown[tl.Key] = true
		}
	}
	for k, want := range map[string]bool{"genre:pop": true, "genre:blues": false, "season:halloween": true, "season:christmas": true, "season:valentines": false, "season:fall": true, "activity:party": true} {
		if shown[k] != want {
			t.Errorf("%s shown = %v, want %v", k, shown[k], want)
		}
	}
}

func TestPageCounts_CountsRows(t *testing.T) {
	b := fakeBundle()
	got := PageCounts(b, time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC), "us", nil, []string{"genre:rock", "mood:chill", "genre:nope"})
	if got["genre:rock"] < 12 || got["mood:chill"] == 0 || got["genre:nope"] != 0 {
		t.Fatalf("counts = %v", got)
	}
}

func TestSimilar_SharedAlbums(t *testing.T) {
	b := browseBundle()
	secs := BrowseSections(b, time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC), "us", nil)
	got := Similar(b, "genre:blues", secs, 4)
	if len(got) != 4 {
		t.Fatalf("similar = %d", len(got))
	}
	for _, tl := range got {
		if tl.Key == "genre:blues" || kindOf(tl.Key) != "genre" {
			t.Fatalf("similar tile %s", tl.Key)
		}
	}
}

// browseBundle builds a bundle with every Browse key holding a few albums.
func browseBundle() *Bundle {
	b := &Bundle{Keys: map[string][]Member{}, Names: map[string]KeyInfo{}}
	n := 0
	for _, sec := range BrowseLayout {
		for _, t := range sec.Tiles {
			b.Names[t.Key] = KeyInfo{Name: t.Name, Kind: kindOf(t.Key)}
			for k := 0; k < 5; k++ {
				b.Items = append(b.Items, Item{ID: fmt.Sprintf("rg%d", n), Kind: "a", Title: "A", Artist: "B", ArtistID: "ar", Listens: int64(1000 - k)})
				b.Keys[t.Key] = append(b.Keys[t.Key], Member{I: n, T: 900})
				n++
			}
			b.Keys[t.Key] = append([]Member{{I: 0, T: 1000}}, b.Keys[t.Key]...)
		}
	}
	b.index()
	return b
}

// itemOfCover answers the bundle item index behind a cover address.
func itemOfCover(b *Bundle, cover string) int {
	id := strings.TrimPrefix(cover, "/api/v1/music-trending/art/album/")
	for i, it := range b.Items {
		if it.ID == id {
			return i
		}
	}
	return -1
}
