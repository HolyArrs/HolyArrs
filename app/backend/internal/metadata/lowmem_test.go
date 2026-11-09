package metadata

import (
	"fmt"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
	"testing"
	"time"
)

// Low-memory mode keeps at most 150 TMDB answers in memory (the Disk copy
// answers the rest); off, the cap stays 1000.
func TestTMDBMemoryCapLowMemory(t *testing.T) {
	t.Cleanup(func() { startup.SetLowMemory(false) })
	for _, c := range []struct {
		low bool
		cap int
	}{{false, maxCache}, {true, 150}} {
		startup.SetLowMemory(c.low)
		tm := &TMDB{}
		for i := range c.cap + 1 {
			tm.remember(fmt.Sprint("u", i), cached{body: []byte("x"), at: time.Now()})
		}
		if n := len(tm.cache); n > c.cap {
			t.Fatalf("low %v: %d answers kept, cap %d", c.low, n, c.cap)
		}
	}
}

// Expired show and music answers are dropped at most once a minute when a
// new one is kept, never left until the cache is full.
func TestGetterDropsExpiredAnswers(t *testing.T) {
	t.Cleanup(func() { startup.SetLowMemory(false) })
	g := &getter{}
	now := time.Now()
	g.put("old", []byte("o"), time.Hour, now.Add(-2*time.Hour))
	g.put("fresh", []byte("f"), time.Hour, now.Add(-10*time.Minute))
	g.swept = now.Add(-2 * time.Minute)
	g.put("new", []byte("n"), time.Hour, now)
	if _, ok := g.cache["old"]; ok {
		t.Fatal("an expired answer was kept")
	}
	if _, ok := g.cache["fresh"]; !ok {
		t.Fatal("a fresh answer was dropped")
	}
	if _, ok := g.cache["new"]; !ok {
		t.Fatal("the new answer was not kept")
	}
	startup.SetLowMemory(true)
	g = &getter{}
	for i := range 251 {
		g.put(fmt.Sprint("k", i), []byte("x"), time.Hour, now)
	}
	if len(g.cache) > 250 {
		t.Fatalf("low-memory cap: %d answers", len(g.cache))
	}
}
