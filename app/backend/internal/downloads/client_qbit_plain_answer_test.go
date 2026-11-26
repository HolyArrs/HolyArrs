package downloads

import (
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// Every call asks qBittorrent for a plain answer. Go's own default is
// "Accept-Encoding: gzip", which a proxy that reads the answer too (qui's
// client proxy) passes on and then fails to parse.
func TestQbitAsksForAPlainAnswer(t *testing.T) {
	var mu sync.Mutex
	asked := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		asked[r.URL.Path] = r.Header.Get("Accept-Encoding")
		mu.Unlock()
		switch r.URL.Path {
		case "/api/v2/app/version":
			_, _ = io.WriteString(w, "v5.0.0")
		case "/api/v2/sync/maindata":
			_, _ = io.WriteString(w, `{"rid":1,"full_update":true,"torrents":{}}`)
		default:
			_, _ = io.WriteString(w, "{}")
		}
	}))
	defer srv.Close()
	q := NewQbittorrentKey(srv.URL, "a-key", srv.Client())
	ctx := context.Background()
	if _, err := q.Health(ctx); err != nil {
		t.Fatalf("Health: %v", err)
	}
	if _, err := q.SyncMain(ctx, 0); err != nil {
		t.Fatalf("SyncMain: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, p := range []string{"/api/v2/app/version", "/api/v2/sync/maindata"} {
		if got := asked[p]; got != "identity" {
			t.Errorf("%s: Accept-Encoding %q, want identity", p, got)
		}
	}
}

// A server that packs the answer although a plain one was asked for is still
// read: with the header set by hand, Go leaves the unpacking to the client.
func TestQbitReadsAGzipAnswerNobodyAskedFor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		_, _ = io.WriteString(gz, "v5.0.0")
		_ = gz.Close()
	}))
	defer srv.Close()
	q := NewQbittorrentKey(srv.URL, "a-key", srv.Client())
	h, err := q.Health(context.Background())
	if err != nil {
		t.Fatalf("Health on a gzip answer: %v", err)
	}
	if h.Version != "v5.0.0" {
		t.Errorf("version %q, want v5.0.0", h.Version)
	}
}

// An answer with no body that is still marked gzip (a bare 200 to a delete)
// is an empty answer, not a broken one.
func TestQbitReadsAnEmptyAnswerMarkedGzip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
	}))
	defer srv.Close()
	q := NewQbittorrentKey(srv.URL, "a-key", srv.Client())
	if err := q.Remove(context.Background(), "0123456789abcdef0123456789abcdef01234567", false); err != nil {
		t.Fatalf("Remove on an empty gzip-marked answer: %v", err)
	}
}
