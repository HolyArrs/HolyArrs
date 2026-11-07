package metadata

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestMusicPoolsSendsNoQueryAndUsesETag(t *testing.T) {
	var queries []string
	s := testService(t, func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.RawQuery)
		if r.URL.Path != "/v1/music/pools" {
			w.WriteHeader(404)
			return
		}
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		w.Write([]byte(`{"version":"v1","items":[]}`))
	})
	body, etag, err := s.MusicPools(context.Background(), "")
	if err != nil || etag != `"v1"` || !strings.Contains(string(body), "v1") {
		t.Fatalf("first: %q %q %v", body, etag, err)
	}
	body, etag2, err := s.MusicPools(context.Background(), etag)
	if err != ErrNotModified || body != nil || etag2 != etag {
		t.Fatalf("second: %q %q %v, want not modified", body, etag2, err)
	}
	for _, q := range queries {
		if q != "" {
			t.Fatalf("query %q sent: the bundle request must be the same for everyone", q)
		}
	}
}
