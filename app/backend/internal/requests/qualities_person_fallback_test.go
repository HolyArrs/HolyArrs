package requests

import (
	"context"
	"errors"
	"testing"
)

// A person whose own choice names only a line that is gone (removed, renamed
// or no longer set up) is offered nothing: the first line does not stand in.
func TestPersonQualitiesGoneLineOffersNothing(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	if err := SetPersonQualities(ctx, r.st, r.ana.UserID, PersonQualities{Movie: []string{"own-x"}}); err != nil {
		t.Fatal(err)
	}
	set, _ := ReadSettings(ctx, r.st.Read())
	ls, _ := qualityLines(ctx, r.st.Read(), set, "movie")
	p := r.ana
	if err := loadPersonQualities(ctx, r.st.Read(), &p); err != nil {
		t.Fatal(err)
	}
	if got := ids(offered(ls, p, "movie")); got != "" {
		t.Fatalf("offered %q", got)
	}
	for _, q := range []string{"", QualityStandard} {
		if _, err := r.svc.AskQuality(ctx, r.ana, "693134", q); !errors.Is(err, ErrQualityNotOffered) {
			t.Fatalf("ask %q: %v", q, err)
		}
	}
	// Without a choice the first line still stands in.
	if got := ids(offered(ls[:1], r.bo, "movie")); got != QualityStandard {
		t.Fatalf("bo offered %q", got)
	}
}

// A Plex watchlist ask keeps to the person's own choice: one who may not
// pick Standard gets no Standard ask from it.
func TestWatchlistKeepsPersonQualities(t *testing.T) {
	r, f, w := watchRig(t)
	ctx := context.Background()
	if err := SaveWatchlistSwitches(ctx, r.st, r.ana.UserID, WatchlistSwitches{Movies: true, Shows: true}); err != nil {
		t.Fatal(err)
	}
	if err := SetPersonQualities(ctx, r.st, r.ana.UserID, PersonQualities{Movie: []string{"own-x"}, Show: []string{"own-x"}}); err != nil {
		t.Fatal(err)
	}
	f.set("tok-ana",
		plexMovie("k-dune", "Dune: Part Two", 2024, "tmdb://693134"),
		map[string]any{"ratingKey": "k-show", "type": "show", "title": "Lab Show", "year": "2025", "Guid": []map[string]string{{"id": "tvdb://200"}}},
	)
	if _, err := w.Pass(ctx, r.ana.UserID); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = r.st.Read().QueryRow("SELECT count(*) FROM ask_people WHERE user_id = ?", r.ana.UserID).Scan(&n)
	if n != 0 {
		t.Fatalf("%d asks made outside ana's choice", n)
	}
	got := itemStates(t, r, r.ana.UserID)
	if got["k-dune"] != WatchSkipped || got["k-show"] != WatchSkipped {
		t.Fatalf("states %v", got)
	}
}
