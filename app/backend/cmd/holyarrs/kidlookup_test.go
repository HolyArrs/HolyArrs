package main

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// lightRater is a provider with the light rating lookup, counting calls;
// every title is rated G (movies) or TV-Y (shows).
type lightRater struct{ slowDetails }

var rateCalls atomic.Int32

func (l *lightRater) Rating(_ context.Context, kind, _ string) (string, error) {
	rateCalls.Add(1)
	if kind == "show" {
		return "TV-Y", nil
	}
	return "G", nil
}

func newRatingAPI(t *testing.T) (*handlers, *lightRater) {
	t.Helper()
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	l := &lightRater{}
	rateCalls.Store(0)
	return buildHandlers(services{Store: st, Metadata: l, Version: "test"}), l
}

func unknownRefs(from, n int) []ratingRef {
	var refs []ratingRef
	for i := range n {
		refs = append(refs, ratingRef{Kind: "movie", TMDBID: fmt.Sprint(from + i)})
	}
	return refs
}

func kidCtx(id int64) context.Context {
	return webserver.ContextWith(context.Background(), webserver.Caller{UserID: id, Role: webserver.RoleMember, MaxRating: "PG"})
}

// TestKidRatingLookupsCappedPerRequest: one request looks up at most 24
// ratings; the rest stay hidden.
func TestKidRatingLookupsCappedPerRequest(t *testing.T) {
	a, _ := newRatingAPI(t)
	got := a.kidAllowed(kidCtx(5), "PG", unknownRefs(7000, 40), 0)
	if c := rateCalls.Load(); c != mediafiles.RatingLookupsPerRequest {
		t.Fatalf("%d lookups in one request, want %d", c, mediafiles.RatingLookupsPerRequest)
	}
	if slices.Contains(got[:mediafiles.RatingLookupsPerRequest], false) || slices.Contains(got[mediafiles.RatingLookupsPerRequest:], true) {
		t.Fatalf("allowed %v", got)
	}
}

// TestKidRatingLookupBudgetPerPerson: a person gets 120 lookups a minute;
// over that, unknown titles stay hidden; someone else still gets theirs.
func TestKidRatingLookupBudgetPerPerson(t *testing.T) {
	a, _ := newRatingAPI(t)
	for i := range 5 {
		got := a.kidAllowed(kidCtx(5), "PG", unknownRefs(10000+100*i, 24), 0)
		if slices.Contains(got, false) {
			t.Fatalf("request %d: %v", i, got)
		}
	}
	got := a.kidAllowed(kidCtx(5), "PG", unknownRefs(20000, 24), 0)
	if slices.Contains(got, true) || rateCalls.Load() != 120 {
		t.Fatalf("over budget: %d lookups, allowed %v", rateCalls.Load(), got)
	}
	if got := a.kidAllowed(kidCtx(6), "PG", unknownRefs(30000, 3), 0); slices.Contains(got, false) {
		t.Fatalf("another person: %v", got)
	}
}

// TestKidRatingLightLookupsOwnCache: ratings come from the light lookup,
// not Details, and are kept apart from the Explore cache.
func TestKidRatingLightLookupsOwnCache(t *testing.T) {
	a, l := newRatingAPI(t)
	a.discover.Put("details|movie|1", discover.Entry{At: time.Now(), Details: &metadata.Details{}})
	a.kidAllowed(kidCtx(5), "PG", unknownRefs(7000, 10), 0)
	a.kidAllowed(kidCtx(5), "PG", unknownRefs(7000, 10), 0)
	if c := l.calls.Load(); c != 0 {
		t.Fatalf("%d Details lookups for ratings", c)
	}
	if c := rateCalls.Load(); c != 10 {
		t.Fatalf("%d rating lookups for 10 titles asked twice", c)
	}
	if a.discover.Len() != 1 {
		t.Fatalf("Explore cache has %d entries, want the 1 it had", a.discover.Len())
	}
}

// TestKidExploreUsesLightRatingLookups: a Kid's Explore row checks ratings
// without any full details lookup.
func TestKidExploreUsesLightRatingLookups(t *testing.T) {
	k := newKidRig(t)
	k.kai.call("GET", "/api/v1/discover?row=trending", "", 200)
	if d, l := k.tmdb.details.Load(), k.tmdb.light.Load(); d != 0 || l == 0 {
		t.Fatalf("%d details and %d light lookups", d, l)
	}
}

// TestKidMetadataRoutesRateLimited: a Kid's searches, lookups and details
// pages share the Explore limit of 30 a minute; others are not limited.
func TestKidMetadataRoutesRateLimited(t *testing.T) {
	k := newKidRig(t)
	for range discover.PerMinute {
		k.kai.call("GET", "/api/v1/metadata/search?q=any", "", 200)
	}
	for _, p := range []string{"/api/v1/metadata/search?q=any", "/api/v1/titles/lookup?term=any", "/api/v1/shows/lookup?term=any", "/api/v1/metadata/movie/4101"} {
		resp, err := k.kai.c.Get(k.srv.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != 429 || resp.Header.Get("Retry-After") != "60" {
			t.Errorf("%s: %d, Retry-After %q", p, resp.StatusCode, resp.Header.Get("Retry-After"))
		}
	}
	for range discover.PerMinute + 1 {
		k.ana.call("GET", "/api/v1/metadata/search?q=any", "", 200)
	}
}

// TestKidExploreNotCachedForOthers: Explore rows and TMDB title pages vary
// by who asks, and a Kid's answers are never reused without asking again.
func TestKidExploreNotCachedForOthers(t *testing.T) {
	k := newKidRig(t)
	for _, c := range []struct {
		who   *signedIn
		cache string
	}{{k.kai, "private, no-cache"}, {k.ana, ""}} {
		for _, p := range []string{"/api/v1/discover?row=trending", "/api/v1/metadata/movie/4101"} {
			resp, err := c.who.c.Get(k.srv.URL + p)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != 200 || resp.Header.Get("Vary") != "Cookie, Authorization, X-HolyArrs-Language, Accept-Language" {
				t.Errorf("%s: %d, Vary %q", p, resp.StatusCode, resp.Header.Get("Vary"))
			}
			if c.cache != "" && resp.Header.Get("Cache-Control") != c.cache {
				t.Errorf("kid %s: Cache-Control %q", p, resp.Header.Get("Cache-Control"))
			}
		}
	}
}

// TestKidAskRefusedByLibraryRating: an ask is refused when the Library's
// rating is above the limit, even if the provider's rating is not.
func TestKidAskRefusedByLibraryRating(t *testing.T) {
	k := newKidRig(t)
	k.add(t, "movie", "Sunny Pond", "4111", "R", 0, nil)     // TMDB says G
	k.add(t, "show", "Puppet Lane", "4131", "TV-MA", 0, nil) // TMDB says TV-Y
	const upTo = `{"error":"This account can ask only for titles rated up to PG."}`
	for _, body := range []string{`{"id":"tmdb:4111"}`, `{"kind":"show","id":"tmdb:4131"}`} {
		if got := strings.TrimSpace(string(k.kai.call("POST", "/api/v1/asks", body, 403))); got != upTo {
			t.Errorf("%s: %s", body, got)
		}
	}
}
