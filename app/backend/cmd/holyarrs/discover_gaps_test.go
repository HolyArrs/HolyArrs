package main

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"
)

// TestKidMoreFromAndSeasonRateLimited: more-from and season pages share a
// Kid's Explore limit; an adult is not limited.
func TestKidMoreFromAndSeasonRateLimited(t *testing.T) {
	k := newKidRig(t)
	for range discover.PerMinute {
		k.kai.call("GET", "/api/v1/metadata/search?q=any", "", 200)
	}
	paths := []string{"/api/v1/metadata/movie/4101/more-from", "/api/v1/metadata/show/4131/seasons/1"}
	for _, p := range paths {
		resp, err := k.kai.c.Get(k.srv.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != 429 || resp.Header.Get("Retry-After") != "60" {
			t.Errorf("kid %s: %d, Retry-After %q", p, resp.StatusCode, resp.Header.Get("Retry-After"))
		}
	}
	for range discover.PerMinute + 1 {
		k.ana.call("GET", "/api/v1/metadata/search?q=any", "", 200)
	}
	for _, p := range paths {
		resp, err := k.ana.c.Get(k.srv.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode == 429 {
			t.Errorf("adult %s limited", p)
		}
	}
}

// TestPartOffHidesMoreFrom: with Shows off, a show's more-from answers 409
// like its seasons do, and a movie's still works.
func TestPartOffHidesMoreFrom(t *testing.T) {
	t.Parallel()
	f := newDiscoverTMDB(t)
	srv, _ := discoverRig(t, f.provider(), parts.Shows)
	do(t, "GET", srv.URL+"/api/v1/metadata/show/21/more-from", "", 409)
	do(t, "GET", srv.URL+"/api/v1/metadata/show/21/seasons/1", "", 409)
	do(t, "GET", srv.URL+"/api/v1/metadata/movie/11/more-from", "", 200)
}

// TestDiscoverRowAndWatchServicesCacheHeaders: a Kid's rows and service list
// are never reused; an adult's keep their max-age.
func TestDiscoverRowAndWatchServicesCacheHeaders(t *testing.T) {
	k := newKidRig(t)
	for _, c := range []struct {
		who      *signedIn
		row, svc string
	}{{k.kai, "private, no-cache", "private, no-cache"}, {k.ana, "private, max-age=300", "private, max-age=3600"}} {
		for p, want := range map[string]string{"/api/v1/discover?row=trending&kind=movie": c.row, "/api/v1/watch-services?region=US": c.svc} {
			resp, err := c.who.c.Get(k.srv.URL + p)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			if resp.StatusCode != 200 || resp.Header.Get("Cache-Control") != want || resp.Header.Get("Vary") != "Cookie, Authorization" {
				t.Errorf("%s: %d, Cache-Control %q (want %q), Vary %q", p, resp.StatusCode, resp.Header.Get("Cache-Control"), want, resp.Header.Get("Vary"))
			}
		}
	}
}
