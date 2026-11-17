package discover

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/suggest"
)

type countLister struct {
	n   atomic.Int32
	err error
}

func (c *countLister) ExploreList(ctx context.Context, kind, path string, q url.Values) ([]metadata.DiscoverItem, error) {
	c.n.Add(1)
	return nil, c.err
}

func (c *countLister) PersonCredits(ctx context.Context, name string) ([]metadata.DiscoverItem, error) {
	c.n.Add(1)
	return nil, c.err
}

func (c *countLister) GenreID(ctx context.Context, kind, genre string) (int64, error) { return 0, nil }

func captureLog(t *testing.T) *bytes.Buffer {
	var b bytes.Buffer
	old := log.Writer()
	log.SetOutput(&b)
	t.Cleanup(func() { log.SetOutput(old) })
	return &b
}

func TestExploreOffAsksNoOne(t *testing.T) {
	l := &countLister{}
	e := &Explore{Store: noStore, LookupsOff: func() bool { return true }}
	s := ExploreSource{E: e, Lister: l, Calls: &engine.Calls{}}
	if _, err := s.List(context.Background(), engine.Request{List: "trending", Kind: "movie"}); err != nil {
		t.Fatal(err)
	}
	if l.n.Load() != 0 {
		t.Fatalf("lookups off made %d outside asks", l.n.Load())
	}
}

func TestExploreServerDownLogsOnceAndWaits(t *testing.T) {
	b := captureLog(t)
	l := &countLister{err: metadata.ErrServiceUnavailable}
	e := &Explore{Store: noStore, Paced: &engine.Paced{Source: ListerSource{L: l}, Burst: 100}}
	s := ExploreSource{E: e, Lister: l, Calls: &engine.Calls{}}
	for i, list := range []string{"trending", "best", "upcoming", "trending"} {
		_, _ = s.List(context.Background(), engine.Request{List: list, Kind: "movie", Ref: strconv.Itoa(i)})
	}
	if n := l.n.Load(); n != 1 {
		t.Fatalf("want 1 outside ask while the server is down, got %d", n)
	}
	if c := strings.Count(b.String(), "explore:"); c != 1 {
		t.Fatalf("want one log line, got %d:\n%s", c, b)
	}
	e.hold.mu.Lock()
	e.hold.until = time.Time{}
	e.hold.mu.Unlock()
	l.err = nil
	_, _ = s.List(context.Background(), engine.Request{List: "best", Kind: "movie"})
	if !strings.Contains(b.String(), "answers again") {
		t.Fatalf("want the way back logged:\n%s", b)
	}
}

func noStore() *database.Store { return nil }

// The wait doubles up to two minutes, a fail-fast refusal from our own
// guard does not lengthen it, and an answer ends it.
func TestExploreServerWaitIsCapped(t *testing.T) {
	_ = captureLog(t)
	var h serverHold
	now := time.Unix(1000, 0)
	for range 10 {
		h.note(metadata.ErrServiceUnavailable, now)
	}
	if d := h.until.Sub(now); d != downWaitMax || downWaitMax != 2*time.Minute {
		t.Fatalf("wait after many failures = %v", d)
	}
	until := h.until
	h.note(fmt.Errorf("TMDB: %w (%w)", metadata.ErrServiceUnavailable, outbound.ErrSuppressed), now.Add(time.Minute))
	if h.until != until {
		t.Fatalf("a fail-fast refusal moved the wait to %v", h.until)
	}
	h.note(nil, now)
	if h.held(now) || h.fails != 0 {
		t.Fatal("an answer should end the wait")
	}
	h.note(metadata.ErrServiceUnavailable, now)
	if d := h.until.Sub(now); d != downWaitMin {
		t.Fatalf("first wait after an answer = %v", d)
	}
}

// While the server asks to wait (a 429 with its reason), Explore asks no
// one until the wait ends, and logs nothing per list: the metadata side
// already logged the hold once.
func TestExploreBusyWaitsForRetryAfter(t *testing.T) {
	b := captureLog(t)
	metadata.ResetRefusal()
	t.Cleanup(metadata.ResetRefusal)
	until := time.Now().Add(20 * time.Minute)
	metadata.SetRefusal(metadata.Refusal{Code: "registration_limit", Until: until})
	l := &countLister{err: fmt.Errorf("TMDB: %w", metadata.ErrRateLimited)}
	e := &Explore{Store: noStore, Paced: &engine.Paced{Source: ListerSource{L: l}, Burst: 100}}
	s := ExploreSource{E: e, Lister: l, Calls: &engine.Calls{}}
	for i, list := range []string{"trending", "best", "upcoming", "trending", "best"} {
		_, _ = s.List(context.Background(), engine.Request{List: list, Kind: "movie", Ref: strconv.Itoa(i)})
	}
	if n := l.n.Load(); n != 1 {
		t.Fatalf("want 1 outside ask while the server says wait, got %d", n)
	}
	if !e.hold.held(until.Add(-time.Second)) || e.hold.held(until.Add(time.Second)) {
		t.Fatalf("hold should last exactly the server's wait, until=%v", e.hold.until)
	}
	if strings.Contains(b.String(), "explore:") {
		t.Fatalf("busy lists should not log per list:\n%s", b)
	}
}

// An empty pool built while the server is down is built again at 15 s, 30 s
// and 60 s, and at once when the server answers again.
func TestExploreEmptyPoolRefillsWhenServerReturns(t *testing.T) {
	captureLog(t)
	now := time.Date(2026, 11, 17, 8, 0, 0, 0, time.UTC)
	l := &countLister{err: metadata.ErrServiceUnavailable}
	e := &Explore{Store: noStore, Clock: func() time.Time { return now }, Paced: &engine.Paced{Source: ListerSource{L: l}, Burst: 100}}
	s := ExploreSource{E: e, Lister: l, Calls: &engine.Calls{}}
	_, _ = s.List(context.Background(), engine.Request{List: "trending", Kind: "movie"})
	k := PoolKey{1, "all"}
	for _, want := range []time.Duration{15 * time.Second, 30 * time.Second, time.Minute, time.Minute} {
		e.noteEmpty(k, true, now)
		e.mu.Lock()
		got := e.due[k]
		delete(e.due, k)
		e.mu.Unlock()
		if got.Sub(now) != want {
			t.Fatalf("want a rebuild in %s, got %s", want, got.Sub(now))
		}
	}
	e.hold.mu.Lock()
	e.hold.until = time.Time{}
	e.hold.mu.Unlock()
	l.err = nil
	_, _ = s.List(context.Background(), engine.Request{List: "best", Kind: "movie"})
	deadline := time.Now().Add(2 * time.Second)
	for {
		e.mu.Lock()
		got, ok := e.due[k]
		e.mu.Unlock()
		if ok && !got.After(now) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the empty pool was not queued when the server answered again")
		}
		time.Sleep(10 * time.Millisecond)
	}
	e.noteEmpty(k, false, now)
	e.mu.Lock()
	_, still := e.empty[k]
	e.mu.Unlock()
	if still {
		t.Fatal("a pool with seeds should clear its retries")
	}
}
