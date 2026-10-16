package metadata

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// manyMB answers an artist with total albums, and an album with total
// Pressings, a page at a time.
func manyMB(t *testing.T, total int) *MusicBrainz {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		off, _ := strconv.Atoi(q.Get("offset"))
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/ws/2/artist/" + mbArtistID:
			fmt.Fprintf(w, `{"id":%q,"name":"Lab Band"}`, mbArtistID)
		case "/ws/2/release-group":
			var rows []string
			for i := off; i < min(off+pageSize, total); i++ {
				rows = append(rows, fmt.Sprintf(`{"id":"10000000-0000-0000-0000-%012d","title":"A%d","primary-type":"Album"}`, i, i))
			}
			fmt.Fprintf(w, `{"release-group-count":%d,"release-groups":[%s]}`, total, strings.Join(rows, ","))
		case "/ws/2/release":
			var rows []string
			for i := off; i < min(off+pageSize, total); i++ {
				rows = append(rows, fmt.Sprintf(`{"id":"20000000-0000-0000-0000-%012d","title":"P%d","status":"Official","media":[]}`, i, i))
			}
			fmt.Fprintf(w, `{"release-count":%d,"releases":[%s]}`, total, strings.Join(rows, ","))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	clock := newClock()
	return &MusicBrainz{BaseURL: func(context.Context) string { return srv.URL }, Limiter: &Limiter{Now: clock.Now, SleepUntil: clock.SleepUntil}}
}

// TestMusicBrainzSaysWhenListsAreCut: past maxPages pages the artist is
// marked Partial and Pressings answers ErrListCut with what it read.
func TestMusicBrainzSaysWhenListsAreCut(t *testing.T) {
	mb := manyMB(t, maxPages*pageSize+50)
	a, err := mb.Artist(context.Background(), mbArtistID)
	if err != nil || !a.Partial || len(a.Albums) != maxPages*pageSize {
		t.Fatalf("artist partial %v, %d albums, %v", a.Partial, len(a.Albums), err)
	}
	ps, err := mb.Pressings(context.Background(), mbAlbumA)
	if !errors.Is(err, ErrListCut) || len(ps) != maxPages*pageSize {
		t.Fatalf("pressings %d, %v", len(ps), err)
	}
	whole := manyMB(t, 150)
	if a, err := whole.Artist(context.Background(), mbArtistID); err != nil || a.Partial || len(a.Albums) != 150 {
		t.Fatalf("whole list partial %v, %d, %v", a.Partial, len(a.Albums), err)
	}
	if ps, err := whole.Pressings(context.Background(), mbAlbumA); err != nil || len(ps) != 150 {
		t.Fatalf("whole pressings %d, %v", len(ps), err)
	}
}

// answerMB answers every request with the handler given.
func answerMB(t *testing.T, h http.HandlerFunc) (*MusicBrainz, *memCache) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	clock := newClock()
	cache := &memCache{}
	return &MusicBrainz{BaseURL: func(context.Context) string { return srv.URL }, Limiter: &Limiter{Now: clock.Now, SleepUntil: clock.SleepUntil}, Cache: cache}, cache
}

// TestMusicBrainzBadRequestIsNotNotFound: 400 is a plain error, 404 means
// not found; other answers keep the status out of the words.
func TestMusicBrainzBadRequestIsNotNotFound(t *testing.T) {
	code := http.StatusBadRequest
	mb, _ := answerMB(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(code) })
	_, err := mb.Pressings(context.Background(), mbAlbumA)
	if err == nil || errors.Is(err, ErrArtistNotFound) || !errors.Is(err, ErrMusicUnavailable) {
		t.Fatalf("400: %v", err)
	}
	code = http.StatusInternalServerError
	_, err = mb.Artist(NoCache(context.Background()), mbArtistID)
	if !errors.Is(err, ErrMusicUnavailable) || strings.Contains(err.Error(), "500") {
		t.Fatalf("500: %v", err)
	}
	code = http.StatusNotFound
	if _, err = mb.Artist(NoCache(context.Background()), mbArtistID); !errors.Is(err, ErrArtistNotFound) {
		t.Fatalf("404: %v", err)
	}
}

// TestLimiterGivesBackCancelledSlots: a caller that gives up before its
// moment does not use up a slot.
func TestLimiterGivesBackCancelledSlots(t *testing.T) {
	clock := newClock()
	l := &Limiter{Now: clock.Now, SleepUntil: clock.SleepUntil}
	t0 := clock.Now()
	if err := l.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	gone, cancel := context.WithCancel(context.Background())
	cancel()
	for i := 0; i < 5; i++ {
		if err := l.Wait(gone); err == nil {
			t.Fatal("a cancelled caller was let through")
		}
	}
	if err := l.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := clock.Now().Sub(t0); got != time.Second {
		t.Fatalf("the next caller went %v after the first, want 1s", got)
	}
}

// TestLimiterMembersGetAShare: member lookups are spaced twice as far as
// the rest, and a refresh is not held back by the members' spacing.
func TestLimiterMembersGetAShare(t *testing.T) {
	clock := newClock()
	l := &Limiter{Now: clock.Now, SleepUntil: clock.SleepUntil}
	t0 := clock.Now()
	member := MemberLookup(context.Background())
	var at []time.Duration
	for i := 0; i < 3; i++ {
		if err := l.Wait(member); err != nil {
			t.Fatal(err)
		}
		at = append(at, clock.Now().Sub(t0))
	}
	if at[1]-at[0] < 2*time.Second || at[2]-at[1] < 2*time.Second {
		t.Fatalf("member slots %v", at)
	}
	before := clock.Now()
	if err := l.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if g := clock.Now().Sub(before); g > time.Second {
		t.Fatalf("the refresh waited %v behind the members' spacing", g)
	}
}

// gateClock blocks each sleeper until it is let go or its context ends.
type gateClock struct {
	*fakeClock
	mu       sync.Mutex
	sleeping int
	release  chan struct{}
}

func (g *gateClock) SleepUntil(ctx context.Context, t time.Time) error {
	g.mu.Lock()
	g.sleeping++
	g.mu.Unlock()
	defer func() { g.mu.Lock(); g.sleeping--; g.mu.Unlock() }()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-g.release:
		return g.fakeClock.SleepUntil(ctx, t)
	}
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	for i := 0; i < 5000; i++ {
		if ok() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out")
}

// TestLimiterQueueIsBoundedAndStaffGoFirst: member callers beyond the
// member queue are told MusicBrainz is busy; a refresh still gets in and
// goes before the members already waiting, even one already sleeping.
func TestLimiterQueueIsBoundedAndStaffGoFirst(t *testing.T) {
	g := &gateClock{fakeClock: newClock(), release: make(chan struct{})}
	l := &Limiter{Now: g.Now, SleepUntil: g.SleepUntil, MaxMemberQueue: 2}
	member := MemberLookup(context.Background())
	var mu sync.Mutex
	var order []string
	var wg sync.WaitGroup
	run := func(name string, ctx context.Context) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := l.Wait(ctx); err != nil {
				t.Errorf("%s: %v", name, err)
				return
			}
			mu.Lock()
			order = append(order, name)
			mu.Unlock()
		}()
	}
	run("m1", member)
	waitFor(t, func() bool { g.mu.Lock(); defer g.mu.Unlock(); return g.sleeping == 1 })
	run("m2", member)
	waitFor(t, func() bool { l.mu.Lock(); defer l.mu.Unlock(); return len(l.queue) == 2 })
	if err := l.Wait(member); !errors.Is(err, ErrMusicBusy) {
		t.Fatalf("third member: %v", err)
	}
	run("refresh", context.Background())
	waitFor(t, func() bool {
		l.mu.Lock()
		defer l.mu.Unlock()
		return len(l.queue) == 3 && !l.queue[0].low && l.sleeping != nil && !l.sleeping.low
	})
	close(g.release)
	wg.Wait()
	if strings.Join(order, ",") != "refresh,m1,m2" {
		t.Fatalf("order %v", order)
	}
}

// TestMusicBrainzRefusesHugeAndOddAnswers: an answer over 16 MB is refused,
// and an answer that is not JSON is not cached.
func TestMusicBrainzRefusesHugeAndOddAnswers(t *testing.T) {
	huge := true
	mb, cache := answerMB(t, func(w http.ResponseWriter, r *http.Request) {
		if huge {
			_, _ = w.Write([]byte(`{"artists":["`))
			_, _ = w.Write([]byte(strings.Repeat("x", 16<<20)))
			_, _ = w.Write([]byte(`"]}`))
			return
		}
		_, _ = w.Write([]byte(`not json`))
	})
	if _, err := mb.SearchArtists(context.Background(), "lab"); err == nil {
		t.Fatal("a huge answer was read")
	}
	huge = false
	if _, err := mb.SearchArtists(context.Background(), "lab"); err == nil {
		t.Fatal("odd answer accepted")
	}
	if len(cache.rows) != 0 {
		t.Fatalf("odd or huge answers cached: %d", len(cache.rows))
	}
}

// TestMusicBrainzCacheKeyNamesTheServer: the same path on two servers is
// two cache rows.
func TestMusicBrainzCacheKeyNamesTheServer(t *testing.T) {
	mb, _, _, cache := mbSetup(t)
	if _, err := mb.SearchArtists(context.Background(), "lab"); err != nil {
		t.Fatal(err)
	}
	mirror := &fakeMB{clock: newClock()}
	srv := httptest.NewServer(mirror)
	defer srv.Close()
	mb.BaseURL = func(context.Context) string { return srv.URL }
	if _, err := mb.SearchArtists(context.Background(), "lab"); err != nil || len(mirror.hits) != 1 {
		t.Fatalf("mirror answered from the other server's cache: %v", err)
	}
	if len(cache.rows) != 2 {
		t.Fatalf("cache rows %d", len(cache.rows))
	}
}

// TestMusicBrainzRedirectsStayOnTheServer: a redirect to another host is
// not followed; one on the same host is.
func TestMusicBrainzRedirectsStayOnTheServer(t *testing.T) {
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("followed a redirect to another host: %s", r.URL)
		_, _ = w.Write([]byte(`{"artists":[]}`))
	}))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ws/2/artist":
			http.Redirect(w, r, other.URL+"/ws/2/artist?fmt=json", http.StatusFound)
		case "/ws/2/release-group":
			http.Redirect(w, r, "/ws/2/release-group2?"+r.URL.RawQuery, http.StatusFound)
		case "/ws/2/release-group2":
			_, _ = w.Write([]byte(`{"release-group-count":0,"release-groups":[]}`))
		case "/ws/2/artist/" + mbArtistID:
			fmt.Fprintf(w, `{"id":%q,"name":"Lab Band"}`, mbArtistID)
		}
	}))
	defer srv.Close()
	clock := newClock()
	mb := &MusicBrainz{BaseURL: func(context.Context) string { return srv.URL }, Limiter: &Limiter{Now: clock.Now, SleepUntil: clock.SleepUntil}}
	if _, err := mb.SearchArtists(context.Background(), "lab"); err == nil {
		t.Fatal("cross-host redirect answered")
	}
	if a, err := mb.Artist(context.Background(), mbArtistID); err != nil || a.Name != "Lab Band" {
		t.Fatalf("same-host redirect: %v", err)
	}
}

// TestLimiterHoldDelaysWaitingCallers: a back-off that starts while a
// caller already waits for its moment delays that caller too.
func TestLimiterHoldDelaysWaitingCallers(t *testing.T) {
	clock := newClock()
	t0 := clock.Now()
	var l *Limiter
	first := true
	l = &Limiter{Now: clock.Now, SleepUntil: func(ctx context.Context, at time.Time) error {
		if first {
			first = false
			l.Hold(10 * time.Second) // another caller got a 503 meanwhile
		}
		return clock.SleepUntil(ctx, at)
	}}
	if err := l.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := clock.Now().Sub(t0); got < 10*time.Second {
		t.Fatalf("let through %v after the start, inside the back-off", got)
	}
}
