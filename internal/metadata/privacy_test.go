package metadata

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
)

// docs/metadata-privacy.md promises that a request to the metadata server
// carries only the install token and a few plain headers: no cookies, user
// names or other details about the install.
func TestMetadataRequestsCarryOnlyAllowedHeaders(t *testing.T) {
	allowed := map[string]bool{"Authorization": true, "Accept": true, "Cache-Control": true,
		"If-None-Match": true, "X-Fetch": true, "Accept-Encoding": true, "Content-Length": true}
	var mu sync.Mutex
	var bad []string
	seen := 0
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
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
		t.Fatalf("headers or bodies beyond docs/metadata-privacy.md: %v", bad)
	}
}
