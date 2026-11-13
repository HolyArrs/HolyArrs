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
