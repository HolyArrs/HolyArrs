package rowtpl

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestBuild_GenrePageKeepsRules(t *testing.T) {
	c := pageCtx(fakeBundle(), "genre", "genre:rock")
	rows := Build(c)
	if len(rows) < 12 {
		t.Fatalf("rows = %d, want at least 12 on the fake catalog", len(rows))
	}
	if len(rows) > 30 {
		t.Fatalf("rows = %d, over 30", len(rows))
	}
	checkRows(t, c, rows)
	for _, r := range rows {
		t.Logf("%-22s %2d  %s", r.ID, len(r.Items), r.Title)
	}
	if rows[0].Template != "D01" {
		t.Errorf("head row = %s, want D01", rows[0].Template)
	}
}

func TestSpotlight_ItemsStayOffRows(t *testing.T) {
	c := pageCtx(fakeBundle(), "genre", "genre:rock")
	spot := Spotlight(c)
	if len(spot) != SpotlightMax {
		t.Fatalf("spotlight = %d items, want %d", len(spot), SpotlightMax)
	}
	in := map[int]bool{}
	artists := map[string]bool{}
	for _, i := range spot {
		in[i] = true
		if a := c.artistOf(i); artists[a] {
			t.Fatalf("spotlight has artist %s twice", a)
		} else {
			artists[a] = true
		}
	}
	for _, r := range Build(c) {
		for _, i := range r.Items {
			if in[i] {
				t.Fatalf("spotlight album %s repeated in %s", c.it(i).ID, r.ID)
			}
		}
	}
}

func TestLibraryKeys_GenresAndDecade(t *testing.T) {
	b := fakeBundle()
	l := &Lib{Artists: map[string]float64{"ar001": 1}, Albums: map[string]bool{}}
	for _, i := range []int{0, 1} {
		l.Albums[b.Items[i].ID] = true
	}
	g, d := LibraryKeys(b, l)
	if len(g) == 0 || g[0] != "genre:rock" {
		t.Fatalf("genres = %v", g)
	}
	if d == "" || d[:7] != "decade:" {
		t.Fatalf("decade = %q", d)
	}
}

func TestCandidates_Explore(t *testing.T) {
	c := pageCtx(fakeBundle(), "explore", "")
	cands := Candidates(c)
	if len(cands) == 0 {
		t.Fatal("no Explore candidates")
	}
	for _, cd := range cands {
		if len(cd.Items) < RowMin || len(cd.Scores) != len(cd.Items) || cd.Title == "" || cd.Reason == "" || cd.Family == "" {
			t.Fatalf("candidate %+v", cd.ID)
		}
		if cd.Template == "C01" || cd.Template == "B06" {
			t.Fatalf("%s needs a library or the Explore builder", cd.Template)
		}
	}
}

func TestBuild_StableWithinDay(t *testing.T) {
	b := fakeBundle()
	a := Build(pageCtx(b, "genre", "genre:rock"))
	c2 := pageCtx(b, "genre", "genre:rock")
	c2.Now = c2.Now.Add(5 * time.Hour)
	bb := Build(c2)
	if len(a) != len(bb) {
		t.Fatalf("row count changed within the day: %d vs %d", len(a), len(bb))
	}
	for i := range a {
		if a[i].ID != bb[i].ID || a[i].Title != bb[i].Title {
			t.Fatalf("row %d changed within the day: %s %q vs %s %q", i, a[i].ID, a[i].Title, bb[i].ID, bb[i].Title)
		}
	}
}

func TestBasis_FollowsRowItems(t *testing.T) {
	c := pageCtx(fakeBundle(), "genre", "genre:rock")
	var own, mixed []int
	for _, m := range c.B.Keys["mood:chill"] {
		if m.Own {
			own = append(own, m.I)
		}
		mixed = append(mixed, m.I)
	}
	if got := c.basis("mood:chill", own[:8]); got != "tagged chill by listeners" {
		t.Errorf("all own: %q", got)
	}
	if got := c.basis("mood:chill", mixed[:8]); got != "in styles that suit chill" {
		t.Errorf("mixed: %q", got)
	}
	_, reason := c.render(Catalog[indexOf("G04")], "mood:chill", mixed[:8], "G04:mood:chill")
	if !strings.HasPrefix(reason, "Rock albums in styles that suit chill") {
		t.Errorf("G04 reason on a genre page = %q", reason)
	}
}

func TestBuild_MoodAndDecadePagesKeepRules(t *testing.T) {
	b := fakeBundle()
	for _, k := range []string{"mood:chill", "decade:1990s"} {
		c := pageCtx(b, kindOf(k), k)
		rows := Build(c)
		if len(rows) < 4 {
			t.Errorf("%s: %d rows", k, len(rows))
		}
		checkRows(t, c, rows)
	}
}

// TestBuild_DryRunRealBundle builds every Browse page from a real bundle (ROWTPL_BUNDLE) and writes the row counts (ROWTPL_REPORT). It skips without a bundle.
func TestBuild_DryRunRealBundle(t *testing.T) {
	path := os.Getenv("ROWTPL_BUNDLE")
	if path == "" {
		t.Skip("set ROWTPL_BUNDLE to a bundle.json")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	b, err := ParseBundle(raw)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	var blocks map[string][]string
	neighbor := ""
	// ROWTPL_SHELVES optionally gives the region's live list blocks.
	if sp := os.Getenv("ROWTPL_SHELVES"); sp != "" {
		var sh struct {
			Neighbor string `json:"neighbor_region"`
			Blocks   map[string][]struct {
				Kind string `json:"kind"`
				MBID string `json:"mbid"`
			} `json:"blocks"`
		}
		if b, err := os.ReadFile(sp); err == nil && json.Unmarshal(b, &sh) == nil {
			blocks, neighbor = map[string][]string{}, sh.Neighbor
			for k, l := range sh.Blocks {
				for _, it := range l {
					if it.Kind == "artist" {
						blocks[k] = append(blocks[k], "r:"+strings.ToLower(it.MBID))
					} else {
						blocks[k] = append(blocks[k], "a:"+strings.ToLower(it.MBID))
					}
				}
			}
		}
	}
	type res struct {
		Key     string   `json:"key"`
		Rows    int      `json:"rows"`
		MinRow  int      `json:"min_items"`
		Albums  int      `json:"pool_albums"`
		Titles  []string `json:"titles"`
		Reasons []string `json:"reasons,omitempty"`
	}
	var out []res
	keys := []string{"explore:", "new:"}
	for k := range b.Names {
		if kk := kindOf(k); kk == "genre" || kk == "mood" || kk == "season" || kk == "decade" || kk == "activity" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		kind := kindOf(k)
		key := k
		if kind == "explore" || kind == "new" {
			key = ""
		}
		c := &Ctx{B: b, Kind: kind, Key: key, Region: "us", Now: now, Member: 1, Blocks: blocks, Neighbor: neighbor}
		Spotlight(c)
		rows := Build(c)
		checkRows(t, c, rows)
		r := res{Key: k, Rows: len(rows), MinRow: 99, Albums: len(c.albums(c.key(key)))}
		for _, row := range rows {
			r.MinRow = min(r.MinRow, len(row.Items))
			r.Titles = append(r.Titles, fmt.Sprintf("%s (%d)", row.Title, len(row.Items)))
		}
		out = append(out, r)
		t.Logf("%-28s rows %2d  min %2d  pool %5d", k, r.Rows, r.MinRow, r.Albums)
	}
	if p := os.Getenv("ROWTPL_REPORT"); p != "" {
		j, _ := json.MarshalIndent(out, "", " ")
		_ = os.WriteFile(p, j, 0o644)
	}
}

func TestBuild_NoArtistIDStillCapsPerArtist(t *testing.T) {
	b := fakeBundle()
	for i := range b.Items {
		if b.Items[i].Kind == "a" && i%3 == 0 {
			b.Items[i].ArtistID = ""
		}
	}
	b.index()
	c := pageCtx(b, "genre", "genre:rock")
	checkRows(t, c, Build(c))
}

func TestRender_TitleNames(t *testing.T) {
	c := pageCtx(fakeBundle(), "genre", "genre:rock")
	if got := c.country("cm"); got != "Cameroon" {
		t.Fatalf("cm = %q", got)
	}
	for in, want := range map[string]string{"Chill": "chill", "Cloud rap": "cloud rap", "K-R&B": "K-R&B", "Regional Mexican": "Regional Mexican", "High-energy": "high-energy"} {
		if got := lowerName(in); got != want {
			t.Errorf("lowerName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuild_OneSingleYearRowPerPage(t *testing.T) {
	for _, k := range []string{"genre:rock", "decade:1990s", "mood:chill"} {
		c := pageCtx(fakeBundle(), kindOf(k), k)
		n := 0
		for _, r := range Build(c) {
			if r.Family == "yr" {
				n++
			}
		}
		if n > 1 {
			t.Fatalf("%s: %d single-year rows", k, n)
		}
	}
}

func TestBuild_CountryRowsPickNearOrBigMarkets(t *testing.T) {
	c := pageCtx(fakeBundle(), "genre", "genre:rock")
	for _, v := range c.slotValues(Catalog[indexOf("D05")]) {
		near := false
		for _, cc := range areas[areaOf("us")] {
			near = near || cc == v
		}
		big := false
		for _, cc := range bigMarkets {
			big = big || cc == v
		}
		if !near && !big {
			t.Fatalf("country %s is neither near nor a big market", v)
		}
	}
}

func TestBuild_SeedsBelongToPage(t *testing.T) {
	b := fakeBundle()
	c := pageCtx(b, "genre", "genre:rock")
	c.Lib = &Lib{Artists: map[string]float64{"ar001": 1}, Seeds: []Seed{{ID: "ar001", Name: "In"}, {ID: "zz-outside", Name: "Out"}}}
	vals := c.slotValues(Catalog[indexOf("G13")])
	for _, v := range vals {
		if v == "zz-outside" {
			t.Fatal("a seed outside the page's genre was used")
		}
	}
}

func TestBundleLookup_CatalogItems(t *testing.T) {
	b := fakeBundle()
	if it, ok := b.Lookup("album", "rg001"); !ok || it.Title != "Album 1" || it.Artist != "Artist 0" {
		t.Fatalf("album %+v %v", it, ok)
	}
	if it, ok := b.Lookup("artist", "ar001"); !ok || it.Title != "Artist 1" {
		t.Fatalf("artist %+v %v", it, ok)
	}
	if _, ok := b.Lookup("album", "ar001"); ok {
		t.Fatal("an artist id answered as an album")
	}
	if _, ok := b.Lookup("album", "nope"); ok {
		t.Fatal("unknown id answered")
	}
}

// TestPageItems_HoldEveryShownItem checks that PageItems holds the Spotlight and every row item of any member's page, whatever the day's order.
func TestPageItems_HoldEveryShownItem(t *testing.T) {
	b := fakeBundle()
	for _, key := range []string{"genre:rock", "mood:chill", "decade:1990s"} {
		c := pageCtx(b, kindOf(key), key)
		in := map[int]bool{}
		for _, i := range PageItems(c) {
			in[i] = true
		}
		for m := int64(1); m <= 6; m++ {
			c := pageCtx(b, kindOf(key), key)
			c.Member = m
			for _, i := range Spotlight(c) {
				if !in[i] {
					t.Fatalf("%s member %d: spotlight item %d missing", key, m, i)
				}
			}
			for _, r := range Build(c) {
				for _, i := range r.Items {
					if !in[i] {
						t.Fatalf("%s member %d: %s item %d missing", key, m, r.ID, i)
					}
				}
			}
		}
	}
}

// TestBuild_StoredPicturesFirst checks that pictured items come first in every row, a row needs MinPictured of them, the Spotlight takes only pictured items and pages stay full.
func TestBuild_StoredPicturesFirst(t *testing.T) {
	b := fakeBundle()
	c := pageCtx(b, "genre", "genre:rock")
	all := len(Build(c))
	c = pageCtx(b, "genre", "genre:rock")
	c.HasArt = func(i int) bool { return i%3 != 0 }
	for _, i := range Spotlight(c) {
		if i%3 == 0 {
			t.Fatalf("spotlight item %d has no picture", i)
		}
	}
	rows := Build(c)
	if len(rows) < all-2 {
		t.Fatalf("%d rows with most pictures stored, %d with all: pages must stay full", len(rows), all)
	}
	for _, r := range rows {
		seenBare := false
		for _, i := range r.Items {
			if i%3 == 0 {
				seenBare = true
			} else if seenBare {
				t.Fatalf("%s: a pictured item after one without a picture", r.ID)
			}
		}
	}
	c = pageCtx(b, "genre", "genre:rock")
	c.HasArt = func(int) bool { return false }
	if n := len(Build(c)); n != 0 {
		t.Fatalf("no pictures at all: %d rows", n)
	}
}

// fakeBundle builds 400 rock albums by 120 artists across five decades. Some chart and some are tagged chill, with the own tag on even albums. Their artists are included.
func fakeBundle() *Bundle {
	b := &Bundle{Keys: map[string][]Member{}, Names: map[string]KeyInfo{
		"genre:rock": {Name: "Rock", Kind: "genre"}, "mood:chill": {Name: "Chill", Kind: "mood"},
		"decade:1990s": {Name: "90s", Kind: "decade"}, "decade:2000s": {Name: "00s", Kind: "decade"}, "decade:2010s": {Name: "10s", Kind: "decade"},
		"decade:1980s": {Name: "80s", Kind: "decade"}, "decade:2020s": {Name: "20s", Kind: "decade"},
		"sub:grunge": {Name: "Grunge", Kind: "sub", Parent: "genre:rock"},
	}}
	countries := []string{"us", "gb", "de", "br", "mx", "fr"}
	for a := 0; a < 120; a++ {
		for k := 0; k < 4; k++ {
			n := a*4 + k
			y := 1980 + (n*7)%46
			it := Item{ID: fmt.Sprintf("rg%03d", n), Kind: "a", Title: fmt.Sprintf("Album %d", n), Artist: fmt.Sprintf("Artist %d", a), ArtistID: fmt.Sprintf("ar%03d", a),
				Date: fmt.Sprintf("%d-03-01", y), Type: "Album", Listens: int64(1000000 / (n + 1)), Users: int64(10000/(n+1) + 1 + n%7), Debut: k == 0}
			if n%5 == 0 {
				it.C = map[string]int{}
				for ci, c := range countries[:1+n%6] {
					it.C[c] = 1 + (n+ci*13)%100
				}
				it.P = (101 - 1 - n%100) * 10
				it.G = map[string]int{"us": 100 + n%300, "gb": 50 + n%200}
			}
			if n < 60 {
				it.R[rWeek] = n + 1
				it.R[rThisYear] = n + 2
			}
			if n%9 == 0 {
				it.Date = time.Date(2026, 10, 1+n%20, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
			}
			b.Items = append(b.Items, it)
			i := len(b.Items) - 1
			b.Keys["genre:rock"] = append(b.Keys["genre:rock"], Member{I: i, T: 600 + n%400})
			if n%2 == 0 {
				b.Keys["mood:chill"] = append(b.Keys["mood:chill"], Member{I: i, T: 400, Own: n%4 == 0})
			}
			if n%3 == 0 {
				b.Keys["sub:grunge"] = append(b.Keys["sub:grunge"], Member{I: i, T: 700})
			}
			dk := fmt.Sprintf("decade:%d0s", y/10)
			b.Keys[dk] = append(b.Keys[dk], Member{I: i, T: 1000, Own: true})
		}
		b.Items = append(b.Items, Item{ID: fmt.Sprintf("ar%03d", a), Kind: "r", Title: fmt.Sprintf("Artist %d", a), Listens: int64(2000000 / (a + 1)), Users: int64(5000/(a+1) + 3)})
		b.Keys["genre:rock"] = append(b.Keys["genre:rock"], Member{I: len(b.Items) - 1, T: 800})
	}
	b.index()
	return b
}

// pageCtx makes a build context on a fixed day.
func pageCtx(b *Bundle, kind, key string) *Ctx {
	return &Ctx{B: b, Kind: kind, Key: key, Region: "us", Now: time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC), Member: 1}
}

// checkRows fails when rows break the page rules: size, filled text, title length, repeated albums and the per-artist caps.
func checkRows(t *testing.T, c *Ctx, rows []Row) {
	t.Helper()
	seenAlbum := map[string]string{}
	for _, r := range rows {
		if len(r.Items) < RowMin {
			t.Errorf("%s: %d items", r.ID, len(r.Items))
		}
		if len(r.Items) > 20 {
			t.Errorf("%s: %d items, over 20", r.ID, len(r.Items))
		}
		if r.Reason == "" || strings.Contains(r.Reason, "{") || strings.Contains(r.Title, "{") {
			t.Errorf("%s: unfilled text %q / %q", r.ID, r.Title, r.Reason)
		}
		if len([]rune(r.Title)) > 40 {
			t.Errorf("%s: title over 40: %q", r.ID, r.Title)
		}
		per := map[string]int{}
		for _, i := range r.Items {
			it := c.it(i)
			if it.Kind == "a" {
				if o, dup := seenAlbum[it.ID]; dup {
					t.Errorf("album %s in %s and %s", it.ID, o, r.ID)
				}
				seenAlbum[it.ID] = r.ID
			}
			per[c.artistOf(i)]++
		}
		if r.Shape != "round" && r.Template != "G13" {
			if len(per) < MinArtists {
				t.Errorf("%s: %d artists", r.ID, len(per))
			}
			for a, n := range per {
				if n > 2 {
					t.Errorf("%s: %d items by %s", r.ID, n, a)
				}
			}
		}
	}
}
