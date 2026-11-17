package metadata

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// batchStandIn is a metadata server with (or, with batch off, without) the
// batch route; it counts what reached it.
func batchStandIn(t *testing.T, batch bool) (*httptest.Server, *atomic.Int64, *atomic.Int64) {
	t.Helper()
	var batches, singles atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(401)
			return
		}
		switch {
		case r.URL.Path == BatchTMDBPath && batch:
			batches.Add(1)
			var items []map[string]any
			for _, read := range r.URL.Query()["r"] {
				switch {
				case strings.Contains(read, "query=Gone"):
					items = append(items, map[string]any{"status": 404})
				case strings.Contains(read, "query=Busy"):
					items = append(items, map[string]any{"status": 429, "retry_after_seconds": 1})
				default:
					items = append(items, map[string]any{"status": 200, "cache": "hit", "body": json.RawMessage(`{"results":[{"id":11,"title":"From Batch","release_date":"1977-05-25"}]}`)})
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"items": items})
		case r.URL.Path == BatchTMDBPath:
			// Today's server: an unknown route is not a lookup.
			w.WriteHeader(403)
		case strings.HasPrefix(r.URL.Path, TMDBPath+"/"):
			singles.Add(1)
			_, _ = w.Write([]byte(`{"results":[{"id":22,"title":"From Single","release_date":"1977-05-25"}]}`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &batches, &singles
}

func batchTMDB(base string) *TMDB {
	return &TMDB{BaseURL: base + TMDBPath, Access: func(context.Context) (string, error) { return "tok", nil }, HTTP: http.DefaultClient, Gap: 1}
}

func TestPrefetchFillsSearchesInOneRequest(t *testing.T) {
	srv, batches, singles := batchStandIn(t, true)
	tm := batchTMDB(srv.URL)
	ctx := context.Background()
	reads := []TMDBRead{MovieSearchRead("Star Wars", 1977), MovieSearchRead("Alien", 0), MovieDetailRead(550)}
	if n := tm.Prefetch(ctx, reads); n != 3 {
		t.Fatalf("prefetched %d", n)
	}
	for _, q := range []string{"Star Wars", "Alien"} {
		year := 0
		if q == "Star Wars" {
			year = 1977
		}
		ms, err := tm.Search(ctx, q, year)
		if err != nil || len(ms) != 1 || ms[0].Name != "From Batch" {
			t.Fatalf("%s: %v %+v", q, err, ms)
		}
	}
	if batches.Load() != 1 || singles.Load() != 0 {
		t.Fatalf("batches %d singles %d", batches.Load(), singles.Load())
	}
	// Already kept: nothing is asked again.
	if n := tm.Prefetch(ctx, reads[:2]); n != 0 || batches.Load() != 1 {
		t.Fatalf("asked again: %d %d", n, batches.Load())
	}
}

func TestPrefetchKeepsNotFoundAndLeavesRefusedReads(t *testing.T) {
	srv, _, singles := batchStandIn(t, true)
	tm := batchTMDB(srv.URL)
	ctx := context.Background()
	tm.Prefetch(ctx, []TMDBRead{MovieSearchRead("Gone", 0), MovieSearchRead("Busy", 0)})
	if _, err := tm.Search(ctx, "Gone", 0); err != ErrNotFound || singles.Load() != 0 {
		t.Fatalf("not found kept: %v singles %d", err, singles.Load())
	}
	// A read the server could not pay for goes the usual way.
	if ms, err := tm.Search(ctx, "Busy", 0); err != nil || len(ms) != 1 || ms[0].Name != "From Single" || singles.Load() != 1 {
		t.Fatalf("refused read: %v %+v singles %d", err, ms, singles.Load())
	}
}

func TestPrefetchOnTodaysServerFallsBackAndStopsAsking(t *testing.T) {
	srv, _, singles := batchStandIn(t, false)
	tm := batchTMDB(srv.URL)
	ctx := context.Background()
	if n := tm.Prefetch(ctx, []TMDBRead{MovieSearchRead("Alien", 0)}); n != 0 {
		t.Fatalf("prefetched %d", n)
	}
	if !tm.batchOff(time.Now()) {
		t.Fatal("batch not switched off after the server lacked it")
	}
	if ms, err := tm.Search(ctx, "Alien", 0); err != nil || len(ms) != 1 || ms[0].Name != "From Single" || singles.Load() != 1 {
		t.Fatalf("single path: %v %+v", err, ms)
	}
}

func TestPrefetchOnlyThroughTheMetadataServer(t *testing.T) {
	srv, batches, _ := batchStandIn(t, true)
	tm := &TMDB{BaseURL: srv.URL + "/3", Access: func(context.Context) (string, error) { return "tok", nil }, HTTP: http.DefaultClient, Gap: 1}
	if n := tm.Prefetch(context.Background(), []TMDBRead{MovieSearchRead("Alien", 0)}); n != 0 || batches.Load() != 0 {
		t.Fatalf("own key went through the batch: %d %d", n, batches.Load())
	}
}

func TestPrefetchSplitsLongBatches(t *testing.T) {
	srv, batches, _ := batchStandIn(t, true)
	tm := batchTMDB(srv.URL)
	var reads []TMDBRead
	for i := range 95 {
		reads = append(reads, MovieSearchRead(strings.Repeat("n", 60)+string(rune('a'+i%26))+strings.Repeat("x", i), 0))
	}
	if n := tm.Prefetch(context.Background(), reads); n != 95 {
		t.Fatalf("prefetched %d", n)
	}
	if batches.Load() < 3 {
		t.Fatalf("batches %d", batches.Load())
	}
}
