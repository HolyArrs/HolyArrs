package sites

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// The site's key and sign-in token go only to the site's own host. A download
// link on another host, such as a CDN, gets neither.
func TestCredentialsStayOnSiteHost(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]http.Header{}
	rec := func(name string) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			seen[name] = r.Header.Clone()
			mu.Unlock()
			_, _ = w.Write([]byte("ok"))
		})
	}
	cdn := httptest.NewTLSServer(rec("cdn"))
	t.Cleanup(cdn.Close)
	base := httptest.NewTLSServer(rec("base"))
	t.Cleanup(base.Close)
	for _, kind := range []string{"opensubtitles", "subdl"} {
		c, err := newClient(Config{URL: base.URL, Key: fakeKey, GapMS: 1, HTTP: cdn.Client()})
		if err != nil {
			t.Fatal(err)
		}
		if kind == "opensubtitles" {
			newOpenSubtitles(c)
		}
		ctx := context.Background()
		if _, err := c.do(ctx, http.MethodGet, base.URL+"/x", nil, map[string]string{"Authorization": "Bearer tok"}, "", 1<<10, false); err != nil {
			t.Fatal(err)
		}
		if _, err := c.fetch(ctx, cdn.URL+"/file.srt", 1<<10); err != nil {
			t.Fatal(err)
		}
		mu.Lock()
		if seen["base"].Get(c.keyHeader) != fakeKey || seen["base"].Get("Authorization") == "" {
			t.Errorf("%s: base host lost its credentials: %v", kind, seen["base"])
		}
		if seen["cdn"].Get(c.keyHeader) != "" || seen["cdn"].Get("Authorization") != "" || seen["cdn"].Get("Api-Key") != "" || seen["cdn"].Get("X-Api-Key") != "" {
			t.Errorf("%s: credentials sent to the download host", kind)
		}
		mu.Unlock()
	}
}
