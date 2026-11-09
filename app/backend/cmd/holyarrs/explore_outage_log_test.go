package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"

	engine "github.com/HolyArrs/holyarrs/app/backend/internal/suggest"
)

// downSource fails while down is set, and always for Ref "gone" (a
// removed title).
type downSource struct{ down atomic.Bool }

func (d *downSource) List(_ context.Context, q engine.Request) ([]engine.Item, error) {
	if d.down.Load() || q.Ref == "gone" {
		return nil, errors.New("metadata server unreachable")
	}
	return []engine.Item{{TMDBID: "1", Kind: q.Kind, Name: "One"}}, nil
}

func (s *syncBuf) lines(part string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.Count(s.b.String(), part)
}

// A failing list logs once until it answers, not on every ask; a key that
// fails on its own stays quiet while other keys of its list answer.
func TestExploreListFailureLoggedOncePerOutage(t *testing.T) {
	t.Parallel()
	a, _, f := exploreRig(t)
	src := &downSource{}
	src.down.Store(true)
	a.explore.Paced = &engine.Paced{Source: src, Gap: time.Millisecond, Burst: 100}
	b := catchLog(t)
	ask := func(ctx context.Context, list, ref string) error {
		q := engine.Request{List: list, Kind: "movie", Ref: ref, Scope: engine.ScopeShared}
		_, err := discover.ExploreSource{E: &a.explore, Lister: f.provider()}.List(ctx, q)
		return err
	}
	bg := context.Background()
	for i := 0; i < 5; i++ {
		if ask(bg, "trending", "") == nil || ask(bg, "similar", "a") == nil || ask(bg, "similar", "b") == nil {
			t.Fatal("a down list answered")
		}
	}
	if n := b.lines("explore: list trending failed"); n != 1 {
		t.Fatalf("trending logged its failure %d times in one outage, want 1", n)
	}
	if n := b.lines("explore: list similar failed"); n != 2 {
		t.Fatalf("2 similar lists logged %d failures in one outage, want 2", n)
	}
	src.down.Store(false)
	for i := 0; i < 5; i++ {
		_ = ask(bg, "similar", "gone")
		if err := ask(bg, "similar", fmt.Sprint("ok", i)); err != nil {
			t.Fatal(err)
		}
	}
	if n := b.lines("explore: list similar failed"); n != 3 {
		t.Fatalf("a removed title among good ones logged %d failures in all, want 3", n)
	}
	if err := ask(bg, "trending", ""); err != nil {
		t.Fatal(err)
	}
	src.down.Store(true)
	if ask(bg, "trending", "x") == nil {
		t.Fatal("a down list answered")
	}
	if n := b.lines("explore: list trending failed"); n != 2 {
		t.Fatalf("a second outage logged %d trending failures in all, want 2", n)
	}
	// A cancelled ask (the page closed) is not an outage.
	ctx, cancel := context.WithCancel(bg)
	cancel()
	_ = ask(ctx, "upcoming", "")
	if n := b.lines("explore: list upcoming"); n != 0 {
		t.Fatalf("a cancelled ask logged %d lines", n)
	}
}
