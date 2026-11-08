package metadata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// docs/library/metadata.md promises that a request to the metadata server
// carries only the install token and a few plain headers: no cookies, user
// names or other details about the install.
func TestMetadataRequestsCarryOnlyAllowedHeaders(t *testing.T) {
	allowed := map[string]bool{"Authorization": true, "Accept": true, "Cache-Control": true,
		"If-None-Match": true, "X-Fetch": true, "Accept-Encoding": true, "Content-Length": true, CountryCountHeader: true}
	var mu sync.Mutex
	var bad []string
	seen := 0
	s := countedService(t, false, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		seen++
		for k, v := range r.Header {
			if k == "User-Agent" && strings.HasPrefix(v[0], "Go-http-client/") {
				continue
			}
			if !allowed[k] {
				bad = append(bad, r.URL.Path+" "+k)
			}
		}
		if a := r.Header.Get("Authorization"); a != "" && a != "Bearer fixture-installation" {
			bad = append(bad, r.URL.Path+" Authorization="+a)
		}
		if r.ContentLength > 0 {
			bad = append(bad, r.URL.Path+" has a body")
		}
		http.Error(w, "nope", http.StatusNotFound)
	})
	ctx := Ahead(context.Background())
	_, _ = s.Register(ctx)
	_, _ = s.Control(ctx)
	_, _, _ = s.MusicPools(ctx, `"etag"`)
	_, _ = s.MusicSimilar(ctx, "5b11f4ce-a62d-471e-81fc-a69a8278c7da")
	_, _ = s.MusicNewWindow(ctx, "week")
	_, _ = s.ServiceTop(ctx, "US")
	_, _ = s.TrailerPick(ctx, "movie", "603", "en")
	if seen < 5 {
		t.Fatalf("only %d requests reached the server", seen)
	}
	if len(bad) > 0 {
		t.Fatalf("headers or bodies beyond docs/library/metadata.md: %v", bad)
	}
}

// countedService is testService behind the Gate, with "Count this install"
// set as on says (off here: every header an install can send goes out).
func countedService(t *testing.T, on bool, handler http.HandlerFunc) *Service {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	g := &Gate{Next: srv.Client().Transport}
	g.SetCountryCount(on)
	s, err := NewService(ServiceOptions{URL: srv.URL, HTTP: &http.Client{Transport: g}, AllowLocal: true,
		Access: func(context.Context) (string, error) { return "fixture-installation", nil }})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// X-Country-Count: off goes out only when "Count this install" is off; on
// (the default) sends no header at all.
func TestCountryCountOffSentOnlyWhenSwitchedOff(t *testing.T) {
	var mu sync.Mutex
	var got []string
	handler := func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got = append(got, r.Header.Get(CountryCountHeader))
		mu.Unlock()
		http.Error(w, "nope", http.StatusNotFound)
	}
	srv := httptest.NewServer(http.HandlerFunc(handler))
	t.Cleanup(srv.Close)
	g := &Gate{Next: srv.Client().Transport}
	s, err := NewService(ServiceOptions{URL: srv.URL, HTTP: &http.Client{Transport: g}, AllowLocal: true,
		Access: func(context.Context) (string, error) { return "fixture-installation", nil }})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_, _ = s.Control(ctx) // default: on
	g.SetCountryCount(false)
	_, _ = s.Control(ctx)
	_, _ = s.MusicSimilar(ctx, "5b11f4ce-a62d-471e-81fc-a69a8278c7da")
	g.SetCountryCount(true)
	_, _ = s.Control(ctx)
	mu.Lock()
	defer mu.Unlock()
	if strings.Join(got, ",") != ",off,off," {
		t.Fatalf("count header per request: %q", got)
	}
}
