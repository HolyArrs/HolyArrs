package metadata

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// tmdbLane is a stand-in TMDB that answers by a switchable mode and notes
// each request's X-Fetch header.
type tmdbLane struct {
	mode   atomic.Value // "ok", "429", "500", "404"
	hits   atomic.Int32
	mu     sync.Mutex
	marks  []string
	paths  []string
	server *httptest.Server
}

func newTMDBLane(t *testing.T) *tmdbLane {
	l := &tmdbLane{}
	l.mode.Store("ok")
	l.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		l.hits.Add(1)
		l.mu.Lock()
		l.marks = append(l.marks, r.Header.Get("X-Fetch"))
		l.paths = append(l.paths, r.URL.Path)
		l.mu.Unlock()
		switch l.mode.Load().(string) {
		case "429":
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(429)
		case "500":
			w.WriteHeader(500)
		case "404":
			w.WriteHeader(404)
		default:
			_, _ = w.Write([]byte(`{"id":693134,"title":"Dune: Part Two","release_date":"2024-02-27"}`))
		}
	}))
	t.Cleanup(l.server.Close)
	return l
}

func (l *tmdbLane) client() *TMDB {
	return &TMDB{BaseURL: l.server.URL + "/3", Access: func(context.Context) (string, error) { return fakeCred, nil }, Gap: time.Millisecond, AheadGap: time.Millisecond}
}

func (l *tmdbLane) lastMark() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.marks) == 0 {
		return "none"
	}
	return l.marks[len(l.marks)-1]
}

// A fetch-ahead carries X-Fetch: ahead, a page call does not.
func TestTMDBAheadMarked(t *testing.T) {
	l := newTMDBLane(t)
	p := l.client()
	if _, err := p.Movie(Ahead(context.Background()), "tmdb:693134"); err != nil {
		t.Fatal(err)
	}
	if l.lastMark() != "ahead" {
		t.Fatalf("ahead mark %q", l.lastMark())
	}
	if _, err := p.Search(context.Background(), "dune", 0); err != nil {
		t.Fatal(err)
	}
	if l.lastMark() != "" {
		t.Fatalf("page call marked %q", l.lastMark())
	}
}

// A 429 to a fetch-ahead holds every later fetch-ahead back (no request is
// sent), while a person's page call is still asked.
func TestTMDBAheadHonoursHold(t *testing.T) {
	l := newTMDBLane(t)
	p := l.client()
	l.mode.Store("429")
	ahead := Ahead(context.Background())
	if _, err := p.Search(ahead, "a", 0); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("first ahead: %v", err)
	}
	if l.hits.Load() != 1 {
		t.Fatalf("a fetch-ahead waits out a 429 instead of retrying: %d hits", l.hits.Load())
	}
	if _, err := p.Search(ahead, "b", 0); !errors.Is(err, ErrRateLimited) || l.hits.Load() != 1 {
		t.Fatalf("held ahead was sent: %v, %d hits", err, l.hits.Load())
	}
	l.mode.Store("ok")
	if _, err := p.Search(context.Background(), "c", 0); err != nil || l.hits.Load() != 2 {
		t.Fatalf("page call during the hold: %v, %d hits", err, l.hits.Load())
	}
}

// Fetch-ahead failures open only their own breaker: a page call still goes out.
func TestTMDBAheadOwnBreaker(t *testing.T) {
	l := newTMDBLane(t)
	p := l.client()
	l.mode.Store("500")
	ahead := Ahead(context.Background())
	for i := range 4 {
		_, _ = p.Search(ahead, "w"+strings.Repeat("x", i), 0)
	}
	before := l.hits.Load()
	if _, err := p.Search(ahead, "again", 0); err == nil || l.hits.Load() != before {
		t.Fatalf("ahead breaker did not open: %v (%d -> %d hits)", err, before, l.hits.Load())
	}
	l.mode.Store("ok")
	if _, err := p.Search(context.Background(), "page", 0); err != nil {
		t.Fatalf("page call held back by the warm-ups' breaker: %v", err)
	}
}

// Answers on disk survive a restart; an old one is served at once and asked
// again in the background; a 404 is remembered; searches stay off disk.
func TestTMDBDiskStaleAndMiss(t *testing.T) {
	l := newTMDBLane(t)
	disk := &memCache{}
	p := l.client()
	p.Disk = disk
	ctx := context.Background()
	if _, err := p.Movie(ctx, "tmdb:693134"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Search(ctx, "private words", 0); err != nil {
		t.Fatal(err)
	}
	for k := range disk.rows {
		if strings.Contains(k, "/search") {
			t.Fatalf("a search was kept on disk: %s", k)
		}
	}
	// A restart: a new client over the same disk, with the source down.
	l.mode.Store("500")
	q := l.client()
	q.Disk = disk
	hits := l.hits.Load()
	m, err := q.Movie(ctx, "tmdb:693134")
	if err != nil || m.Name != "Dune: Part Two" || l.hits.Load() != hits {
		t.Fatalf("after restart: %+v %v (%d -> %d hits)", m, err, hits, l.hits.Load())
	}
	// Old copy: served at once, refreshed in the background.
	l.mode.Store("ok")
	r := l.client()
	r.Disk = disk
	r.TTL = time.Nanosecond
	time.Sleep(time.Millisecond)
	hits = l.hits.Load()
	if m, err := r.Movie(ctx, "tmdb:693134"); err != nil || m.Name != "Dune: Part Two" {
		t.Fatalf("stale: %+v %v", m, err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for l.hits.Load() == hits && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if l.hits.Load() == hits {
		t.Fatal("stale answer was not refreshed in the background")
	}
	// A 404 is remembered for an hour, on disk too.
	l.mode.Store("404")
	if _, err := p.Movie(ctx, "tmdb:7"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("404: %v", err)
	}
	s := l.client()
	s.Disk = disk
	hits = l.hits.Load()
	if _, err := s.Movie(ctx, "tmdb:7"); !errors.Is(err, ErrNotFound) || l.hits.Load() != hits {
		t.Fatalf("miss not remembered: %v (%d -> %d hits)", err, hits, l.hits.Load())
	}
}

// A failed call serves the old copy kept in memory.
func TestTMDBStaleOnFailure(t *testing.T) {
	l := newTMDBLane(t)
	p := l.client()
	p.TTL = time.Nanosecond
	ctx := context.Background()
	if _, err := p.Search(ctx, "dune", 0); err != nil {
		t.Fatal(err)
	}
	l.mode.Store("500")
	time.Sleep(time.Millisecond)
	if _, err := p.Search(ctx, "dune", 0); err != nil {
		t.Fatalf("no stale copy on failure: %v", err)
	}
}
