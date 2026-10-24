package api

import (
	"context"
	"fmt"
	"io/fs"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/CoderLuii/holyarrs/internal/metadata"
)

// fakeShelves answers every region with the same blocks: 30 items each,
// album MBIDs prefixed '7', artist MBIDs '6', 40 artists in turn, and
// blocks that overlap (top_now shares items with albums_week), so dedup
// across rows matters.
func fakeShelves(_ context.Context, region string) (metadata.MusicShelves, error) {
	sh := metadata.MusicShelves{Region: region, Chart: region, Neighbor: "ca", AsOf: "2026-10-24", Blocks: map[string][]metadata.MusicShelfItem{}}
	if region == "global" {
		sh.Neighbor = "us"
	}
	album := func(n int) metadata.MusicShelfItem {
		a := n % 40
		return metadata.MusicShelfItem{Kind: "album", MBID: mid('7', n), ArtistMBID: mid('6', a), Title: fmt.Sprintf("Album %d", n), Artist: fmt.Sprintf("Artist %d", a), Rank: n%30 + 1, Prev: n%30 + 4, P: 1 - float64(n%30)/30, M: 0.5, Days: 50, Date: "2026-10-20"}
	}
	artist := func(n int) metadata.MusicShelfItem {
		a := n % 40
		return metadata.MusicShelfItem{Kind: "artist", MBID: mid('6', a), Artist: fmt.Sprintf("Artist %d", a), Rank: n%30 + 1, P: 1 - float64(n%30)/30, Song: "Song " + fmt.Sprint(n)}
	}
	for b, start := range map[string]int{"top_now": 0, "top_world": 20, "albums_week": 10, "albums_world": 40, "climbing": 60, "new_entries": 80, "new_friday": 100,
		"big_new_albums": 120, "long_runners": 140, "number_ones": 160, "comebacks": 180, "neighbor": 200} {
		for i := 0; i < 30; i++ {
			sh.Blocks[b] = append(sh.Blocks[b], album(start+i))
		}
	}
	for _, b := range []string{"artists_week", "singles_artists"} {
		for i := 0; i < 30; i++ {
			sh.Blocks[b] = append(sh.Blocks[b], artist(i+map[string]int{"artists_week": 0, "singles_artists": 15}[b]))
		}
	}
	return sh, nil
}

func listRig(t *testing.T, name string) *askRig {
	r, _, _ := v7Rig(t, name)
	r.api.s.MusicListsV1 = true
	r.api.s.MusicShelves = fakeShelves
	return r
}

// route: GET /api/v1/explore/music (music_lists_v1)
func TestExploreMusicListRows(t *testing.T) {
	for _, c := range []struct {
		name string
		min  int
	}{{"household_empty.json", 16}, {"household_indie.json", 16}, {"household_large.json", 28}} {
		r := listRig(t, c.name)
		pages, _ := v7Pages(t, r.owner, "US")
		rows := v7Rows(pages)
		noAlbumTwice(t, rows)
		lists := 0
		for _, row := range rows {
			if strings.TrimSpace(row.Reason) == "" {
				t.Fatalf("%s: row %s has no reason", c.name, row.ID)
			}
			if strings.HasPrefix(row.ID, "lists_") {
				lists++
				per := map[string]int{}
				for _, it := range row.Items {
					per[it.ArtistMBID]++
					if per[it.ArtistMBID] > 2 {
						t.Fatalf("%s: %s has 3 cards of %s", c.name, row.ID, it.Artist)
					}
				}
				if len(per) < 4 {
					t.Fatalf("%s: %s has %d artists", c.name, row.ID, len(per))
				}
			}
		}
		if len(rows) < c.min || lists < 6 {
			t.Fatalf("%s: %d rows (%d list rows), want ≥ %d", c.name, len(rows), lists, c.min)
		}
		if h := pages[0].Hero; h == nil || h.Kicker != "Biggest right now" {
			t.Fatalf("%s: Spotlight not led by the chart: %+v", c.name, h)
		}
		// Browse grid: shape grid, every cover different.
		grid := false
		covers := map[string]bool{}
		for _, row := range rows {
			if row.ID != "genres" {
				continue
			}
			grid = row.Shape == "grid" && row.Title == "Browse"
			for _, it := range row.Items {
				if it.Cover != "" {
					if covers[it.Cover] {
						t.Fatalf("cover %s on two tiles", it.Cover)
					}
					covers[it.Cover] = true
				}
			}
		}
		if !grid || len(covers) == 0 {
			t.Fatalf("%s: no Browse grid with covers", c.name)
		}
	}
	// Same plan on reload the same day.
	r := listRig(t, "household_indie.json")
	a, _ := v7Pages(t, r.owner, "US")
	b, _ := v7Pages(t, r.owner, "US")
	if fmt.Sprint(rowIDs(v7Rows(a))) != fmt.Sprint(rowIDs(v7Rows(b))) {
		t.Fatal("plan changed on reload")
	}
}

func rowIDs(rows []MusicRow) []string {
	var out []string
	for _, r := range rows {
		out = append(out, r.ID)
	}
	return out
}

// route: GET /api/v1/explore/music (music_lists_v1 off)
func TestExploreMusicListRowsOffByDefault(t *testing.T) {
	r, _, _ := v7Rig(t, "household_indie.json")
	r.api.s.MusicShelves = fakeShelves
	for _, row := range v7Rows(func() []v7Answer { p, _ := v7Pages(t, r.owner, "US"); return p }()) {
		if strings.HasPrefix(row.ID, "lists_") || row.Shape == "grid" {
			t.Fatalf("flag off shows %s", row.ID)
		}
	}
}

func TestNewMusicListRowsLeadWithCharts(t *testing.T) {
	r := listRig(t, "household_indie.json")
	req := httptest.NewRequest("GET", "/api/v1/explore/new/music?region=US", nil)
	existing := []MusicRow{{ID: "old", Type: "old", Title: "Old", Reason: "r", Shape: "square", Items: []MusicCard{listCard(metadata.MusicShelfItem{Kind: "album", MBID: mid('7', 0), Title: "Album 0", Artist: "Artist 0"})}}}
	hero, rows := r.api.musicNewListRows(context.Background(), req, musicNewInput{}, nil, existing, v7Fixed)
	if len(hero) < 3 || hero[0].MBID != mid('7', 0) {
		t.Fatalf("hero is not the region's number one album: %+v", hero)
	}
	if len(rows) < 12 || rows[0].ID != "lists_new_friday" {
		t.Fatalf("%d rows, first %v", len(rows), rowIDs(rows))
	}
	noAlbumTwice(t, rows)
	for _, row := range rows {
		for _, c := range row.Items {
			for _, h := range hero {
				if c.Key == h.Key {
					t.Fatalf("hero album %s again in %s", c.Name, row.ID)
				}
			}
		}
	}
}

// Every reason line is one of our templates, rendered; never a list name.
func TestListReasonsAreTemplates(t *testing.T) {
	for _, region := range []string{"us", "global", "br", "zz"} {
		sh, _ := fakeShelves(context.Background(), region)
		for _, d := range listRows(&sh) {
			ok := false
			for _, tpl := range listTemplates {
				re := "^" + regexp.QuoteMeta(tpl.Reason) + "$"
				for _, slot := range []string{"{place2}", "{place}", "{neighbor}"} {
					re = strings.ReplaceAll(re, regexp.QuoteMeta(slot), "(worldwide|in [A-Z][A-Za-z ]+|in the [A-Z][A-Za-z ]+)")
				}
				if regexp.MustCompile(re).MatchString(d.Reason) {
					ok = true
				}
			}
			if !ok || strings.Contains(d.Title, "{") {
				t.Fatalf("%s: %q / %q not rendered from a template", region, d.Title, d.Reason)
			}
		}
	}
}

// The list source's brand never shows: not in the web app, the API's
// strings, log lines or settings (plan 2026-10-24 §8).
func TestNoListBrandWord(t *testing.T) {
	brand := regexp.MustCompile(`(?i)` + "spot" + "ify")
	roots := []string{"../../web/app/src", ".", "../../cmd", "../music", "../metadata"}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if n := d.Name(); n == "node_modules" || n == ".svelte-kit" || n == "testdata" || n == ".omc" {
					return filepath.SkipDir
				}
				return nil
			}
			ext := filepath.Ext(p)
			if strings.HasSuffix(p, "_test.go") || (ext != ".go" && ext != ".svelte" && ext != ".ts" && ext != ".json" && ext != ".html") {
				return nil
			}
			if strings.HasSuffix(p, ".test.ts") {
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			if loc := brand.FindIndex(b); loc != nil {
				t.Errorf("brand word in %s", p)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

// Every album and artist a list row shows is in the fetch-ahead set made
// when the shelves arrive (container start, list refresh).
func TestListAheadCoversEveryListCard(t *testing.T) {
	r := listRig(t, "household_large.json")
	sh, _ := fakeShelves(context.Background(), "us")
	ahead := map[aheadItem]bool{}
	for _, it := range listAhead(&sh) {
		ahead[it] = true
	}
	pages, _ := v7Pages(t, r.owner, "US")
	n := 0
	for _, row := range v7Rows(pages) {
		if !strings.HasPrefix(row.ID, "lists_") {
			continue
		}
		for _, c := range row.Items {
			n++
			if !ahead[aheadItem{c.Kind, c.MBID}] || c.Kind == "album" && !ahead[aheadItem{"artist", c.ArtistMBID}] {
				t.Fatalf("%s card %s not fetched ahead", row.ID, c.Key)
			}
		}
	}
	if n == 0 {
		t.Fatal("no list cards")
	}
	if pages[0].Hero != nil && !ahead[aheadItem{"album", pages[0].Hero.MBID}] {
		t.Fatal("hero not fetched ahead")
	}
}
