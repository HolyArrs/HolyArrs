package library

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// fakeTraktRatings answers like Trakt: tt0000001 rated, tt0000002 not
// found (204), and 429 while busy is set. It refuses any request carrying a
// sign-in or without the client id.
func fakeTraktRatings(t *testing.T, asked *atomic.Int32, busy *atomic.Bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		switch {
		case r.Header.Get("trakt-api-key") != "trakt-fixture" || r.Header.Get("Authorization") != "":
			w.WriteHeader(http.StatusForbidden)
		case busy.Load():
			w.WriteHeader(http.StatusTooManyRequests)
		case r.URL.Path == "/movies/tt0000001/ratings":
			_, _ = w.Write([]byte(`{"rating":8.3347,"votes":52731,"distribution":{"10":1}}`))
		default:
			w.WriteHeader(http.StatusNoContent) // how Trakt answers an IMDb id it does not know
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestTraktPassKeepsRatingsAndAsksWeekly: the pass keeps a rating with its
// votes, marks an unknown title asked, asks again only after a week, does
// nothing with no client id, and rests after Trakt asks it to slow down
// or refuses the client id.
func TestTraktPassKeepsRatingsAndAsksWeekly(t *testing.T) {
	var asked atomic.Int32
	var busy atomic.Bool
	srv := fakeTraktRatings(t, &asked, &busy)
	key := ""
	svc, s, clock := omdbRig(t, srv.URL, &key)
	svc.OMDb = nil
	svc.TraktRatings = &metadata.TraktRatings{Base: srv.URL, Key: func(context.Context) (string, error) { return key, nil }}
	ctx := context.Background()
	rated := addWithIMDb(t, s, "Dune: Part Two", "693134", "tt0000001")
	unknown := addWithIMDb(t, s, "The Lantern Keepers", "900001", "tt0000002")
	addWithIMDb(t, s, "No IMDb", "900002", "")

	if err := svc.TraktPass(ctx); err != nil || asked.Load() != 0 {
		t.Fatalf("no client id: %v, asked %d", err, asked.Load())
	}
	key = "trakt-fixture"
	if err := svc.TraktPass(ctx); err != nil || asked.Load() != 2 {
		t.Fatalf("first pass: %v, asked %d", err, asked.Load())
	}
	if f := omdbTitle(t, s, rated); f.TraktRating == nil || *f.TraktRating != 8.3347 || f.TraktVotes != 52731 || f.TraktCheckedAt == nil {
		t.Fatalf("rated %+v %d", f.TraktRating, f.TraktVotes)
	}
	if u := omdbTitle(t, s, unknown); u.TraktRating != nil || u.TraktCheckedAt == nil {
		t.Fatalf("unknown %+v %v", u.TraktRating, u.TraktCheckedAt)
	}
	if err := svc.TraktPass(ctx); err != nil || asked.Load() != 2 {
		t.Fatalf("asked again within the week: %d", asked.Load())
	}
	*clock = clock.Add(8 * 24 * time.Hour)
	busy.Store(true)
	outbound.Reset()
	if err := svc.TraktPass(ctx); err != nil || asked.Load() != 3 {
		t.Fatalf("busy pass asked %d", asked.Load())
	}
	busy.Store(false)
	if err := svc.TraktPass(ctx); err != nil || asked.Load() != 3 {
		t.Fatalf("asked while resting: %d", asked.Load())
	}
	*clock = clock.Add(2 * time.Hour)
	outbound.Reset()
	if err := svc.TraktPass(ctx); err != nil || asked.Load() != 5 {
		t.Fatalf("after the rest asked %d", asked.Load())
	}
	// A refused client id rests the pass until a new one is saved.
	*clock = clock.Add(8 * 24 * time.Hour)
	key = "refused-id"
	outbound.Reset()
	if err := svc.TraktPass(ctx); err != nil || asked.Load() != 6 {
		t.Fatalf("refused pass asked %d", asked.Load())
	}
	if err := svc.TraktPass(ctx); err != nil || asked.Load() != 6 {
		t.Fatalf("asked with a refused id: %d", asked.Load())
	}
	key = "trakt-fixture"
	if err := svc.TraktPass(ctx); err != nil || asked.Load() != 8 {
		t.Fatalf("a new id did not end the rest: %d", asked.Load())
	}
}
