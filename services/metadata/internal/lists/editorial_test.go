package lists

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestParseCategoryAndList(t *testing.T) {
	ids, err := ParseCategory(strings.NewReader(string(readFixture(t, "category.json"))))
	if err != nil || len(ids) < 20 || !strings.HasPrefix(ids[0].ExtAlbum, "37i9dQZF1D") || ids[0].Rank != 1 {
		t.Fatalf("category %d %v", len(ids), err)
	}
	seen := map[string]bool{}
	for _, e := range ids {
		if seen[e.ExtAlbum] {
			t.Fatal("list id twice")
		}
		seen[e.ExtAlbum] = true
	}
	songs, err := ParsePlaylist(strings.NewReader(string(readFixture(t, "playlist.json"))))
	if err != nil || len(songs) != 30 {
		t.Fatalf("list %d %v", len(songs), err)
	}
	s := songs[0]
	if s.Rank != 1 || s.Title == "" || s.Artist == "" || s.ExtArtist == "" || s.Album == "" || s.ExtAlbum == "" || len(s.Image) != 40 {
		t.Fatalf("song %+v", s)
	}
}

func TestPlanSpendsSmallShareOnTiles(t *testing.T) {
	free := Plan(900)
	if c := PlanCost(free); c > 810 {
		t.Fatalf("free plan cost %d", c)
	}
	tileCost, lists := 0, 0
	for _, s := range free {
		if s.Kind == KindCategory || s.Kind == KindList {
			tileCost += monthCost(s.Every)
		}
		if s.Kind == KindList {
			lists++
			if s.Slot != 0 || s.Every != weekly {
				t.Fatalf("free list %+v", s)
			}
		}
	}
	// One list per free tile plus the seasonal slot; tiles take at most
	// a seventh of the cap.
	if lists != freeTiles+1 || tileCost > 900/7 {
		t.Fatalf("free plan: %d lists for %d calls", lists, tileCost)
	}
	// The daily charts of the first wave still come first.
	for i := range firstWave {
		if free[i].Kind != KindSongs {
			t.Fatalf("slot %d is %s", i, free[i].Key)
		}
	}
	pro := Plan(6000)
	n := 0
	for _, s := range pro {
		if s.Kind == KindList {
			n++
		}
	}
	if n != 3*len(Tiles)+1 || PlanCost(pro) > 5400 {
		t.Fatalf("pro lists %d cost %d", n, PlanCost(pro))
	}
}

func TestTileBlocksFromLists(t *testing.T) {
	src := &fakeSource{}
	s, logs := openTest(t, src, Options{MonthCap: 900, PlanLimit: 1800, DayCap: 1000, MatchMax: 5000})
	if err := s.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if src.n != len(s.Sources()) {
		t.Fatalf("calls %d of %d sources", src.n, len(s.Sources()))
	}
	body, _, ok := s.Shelves("us")
	if !ok {
		t.Fatal("no shelves")
	}
	// Oct 24: the seasonal slot holds Halloween's list.
	for _, w := range []string{`"tile:pop"`, `"tile:chill"`, `"video_artists"`, `"tile:season"`, `"season":"halloween"`} {
		if !strings.Contains(string(body), w) {
			t.Fatalf("no %s block", w)
		}
	}
	if strings.Contains(strings.ToLower(string(body)), brand) || strings.Contains(string(body), "37i9") || strings.Contains(string(body), "ext_") {
		t.Fatal("source ids or brand in shelves")
	}
	for _, l := range *logs {
		if strings.Contains(strings.ToLower(l), brand) {
			t.Fatalf("log %q", l)
		}
	}
	// A list song on its own single becomes its artist; one on an album
	// becomes the album.
	songs, _ := ParsePlaylist(strings.NewReader(string(readFixture(t, "playlist.json"))))
	albumNames := map[string]bool{}
	for i, e := range songs {
		if isAlbum(e) {
			albumNames[e.Album] = true
		} else if i%2 == 0 {
			albumNames[e.Title] = true // half the singles are in MusicBrainz
		}
	}
	look := func(kind, name, artist string) (string, string, bool) {
		if kind == "album" && !albumNames[name] {
			return "", "", false
		}
		if kind == "artist" {
			return fakeID("artist|" + name), "", true
		}
		return fakeID("album|" + name + "|" + artist), fakeID("artist|" + artist), true
	}
	in := Inputs{Now: time.Now(), Lookup: look, Lists: map[string][]Entry{"list:pop:0": songs}}
	sh := Build("us", in)
	tile := sh.Blocks["tile:pop"]
	if len(tile) == 0 {
		t.Fatal("empty tile")
	}
	albums, artists := 0, 0
	for i, it := range tile {
		if it.Rank != 0 && i > 0 && it.Rank < tile[i-1].Rank {
			t.Fatal("tile not in list order")
		}
		if it.Kind == "album" {
			albums++
		} else {
			artists++
		}
	}
	if albums == 0 || artists == 0 {
		t.Fatalf("albums %d artists %d", albums, artists)
	}
	if len(sh.Blocks["video_artists"]) != 0 {
		t.Fatal("video artists without charts")
	}
}

func TestListWaitsForItsCategoryPage(t *testing.T) {
	src := &fakeSource{}
	s, _ := openTest(t, src, Options{MonthCap: 900, PlanLimit: 1800, DayCap: 1000})
	lst := Source{Key: "list:pop:0", Kind: KindList, Region: "global", Every: weekly, Tile: Tiles[0]}
	if s.listPath(lst) != "" {
		t.Fatal("list path before its category page")
	}
	_ = s.Refresh(context.Background())
	if p := s.listPath(lst); !strings.HasPrefix(p, "/playlist_tracks?id=37i9dQZF1D") {
		t.Fatalf("list path %q", p)
	}
	if s.listPath(Source{Kind: KindList, Tile: Tiles[0], Slot: 999}) != "" {
		t.Fatal("slot past the page")
	}
}

type fakeLink struct{ n int }

func (f *fakeLink) link(_ context.Context, kind, ext string) (string, error) {
	f.n++
	return fakeID("link|" + ext), nil
}

func TestLinkMatchesBeforeNames(t *testing.T) {
	src := &fakeSource{}
	srch := &fakeSearch{}
	fl := &fakeLink{}
	s, _ := openTest(t, src, Options{MonthCap: 900, PlanLimit: 1800, DayCap: 1000, MatchMax: 5000, Search: srch, Link: fl.link})
	_ = s.Refresh(context.Background())
	if fl.n == 0 {
		t.Fatal("no link lookups")
	}
	// Artists come by link; only albums (no link) are searched by name.
	c, ok, _ := s.matcher.Known(context.Background(), "artist", "Maisie Peters", "")
	if !ok || c.MBID != fakeID("link|2RVvqRBon9NgaGXKfywDSs") {
		t.Fatalf("artist not matched by link: %+v", c)
	}
	if s.Backlog() {
		t.Fatalf("searches %d links %d backlog %v", srch.n, fl.n, s.Backlog())
	}
}
