package discover

import "testing"

// The warm-up builds Genres for every tab first, then each section in page order.
func TestBrowseWarmOrder(t *testing.T) {
	got := browseWarmOrder([]string{"all", "movies", "shows"})
	if len(got) != 21 {
		t.Fatalf("jobs %d", len(got))
	}
	for i, sw := range []string{"all", "movies", "shows"} {
		if got[i] != (browseJob{sw, "genres"}) {
			t.Fatalf("job %d = %v", i, got[i])
		}
	}
	if got[3].section != "moods" || got[20] != (browseJob{"shows", "collections"}) {
		t.Fatalf("order %v", got)
	}
}

// The shared cache is per tab and rating limit: a Kid's maximum rating never
// shares an adult's sections.
func TestBrowseShareKeyedByKidMax(t *testing.T) {
	h := &Handlers{}
	open, pg, pg13 := h.browseShare("all", false, ""), h.browseShare("all", true, "PG"), h.browseShare("all", true, "PG-13")
	if open == pg || pg == pg13 || h.browseShare("all", true, "PG") != pg || h.browseShare("movies", false, "") == open {
		t.Fatal("shares not keyed by tab, kid and maximum rating")
	}
	ls := h.browseLimits()
	if ls[0] != (browseLimit{false, ""}) || len(ls) != 3 {
		t.Fatalf("limits %v", ls)
	}
}
