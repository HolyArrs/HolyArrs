package api

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/explore"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// Wide card art: picked once per title and language, kept in card_art,
// read on page answers without going outside, and not fetched again while fresh.
func TestCardArtCached(t *testing.T) {
	a, _, f := exploreRig(t)
	a.explore.cards.gap = time.Millisecond
	ctx := context.Background()
	pool := &explore.Pool{Rows: []explore.Row{{Items: []explore.Item{{Kind: "movie", TMDBID: "7", Backdrop: "b.jpg"}, {Kind: "artist", TMDBID: "9"}}}}}
	wait := func() {
		for i := 0; i < 500; i++ {
			a.explore.cards.mu.Lock()
			done := !a.explore.cards.running
			a.explore.cards.mu.Unlock()
			if done {
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		t.Fatal("card art filler did not finish")
	}
	a.queueCardArt(ctx, pool)
	wait()
	if a.explore.cards.fetched != 1 {
		t.Fatalf("one image list call expected, got %d", a.explore.cards.fetched)
	}
	a.queueCardArt(ctx, pool)
	wait()
	if a.explore.cards.fetched != 1 {
		t.Fatalf("a fresh pick was fetched again: %d calls", a.explore.cards.fetched)
	}
	before := len(f.urls)
	items := []discoverItemView{{DiscoverItem: metadata.DiscoverItem{Kind: "movie", TMDBID: "7"}}}
	a.fillCardArt(httptest.NewRequest("GET", "/x", nil), items)
	if len(f.urls) != before {
		t.Fatal("filling a page answer went outside")
	}
	// The fake lists no images: the pick keeps the known backdrop (name variant).
	if items[0].Art != "b.jpg" || items[0].Logo != "" || items[0].HasTitle {
		t.Fatalf("pick: %+v", items[0].CardArt)
	}
}
