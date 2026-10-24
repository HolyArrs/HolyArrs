package rowtpl

import (
	"fmt"
	"testing"
	"time"
)

// browseBundle has every Browse key with a few albums each.
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
			// A shared top album: covers must still differ across tiles.
			b.Keys[t.Key] = append([]Member{{I: 0, T: 1000}}, b.Keys[t.Key]...)
		}
	}
	b.index()
	return b
}

func TestBrowseSections(t *testing.T) {
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

func TestBrowseTilesHaveNoNeighbourWithTheSameHue(t *testing.T) {
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

func TestExploreBrowseTiles(t *testing.T) {
	secs := BrowseSections(browseBundle(), time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC), "us", nil)
	got := ExploreTiles(secs)
	if len(got) != 8 {
		t.Fatalf("explore tiles = %d, want 8", len(got))
	}
	kinds := map[string]int{}
	for _, tl := range got {
		kinds[kindOf(tl.Key)]++
	}
	if kinds["genre"] != 4 || kinds["season"] != 1 || kinds["mood"] != 2 || kinds["decade"] != 1 {
		t.Fatalf("kinds = %v, want 4 genres, 1 season, 2 moods, 1 decade", kinds)
	}
	if got[4].Key != "season:halloween" {
		t.Fatalf("season tile = %s, want Halloween on Oct 24", got[4].Key)
	}
}
