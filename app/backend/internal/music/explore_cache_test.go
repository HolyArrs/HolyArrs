package music

import (
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	musicscore "github.com/HolyArrs/holyarrs/app/backend/internal/music/score"
)

func TestMusicTitleChangeClearsDays(t *testing.T) {
	old := musicDayDebounce
	musicDayDebounce = 20 * time.Millisecond
	defer func() { musicDayDebounce = old }()
	a := newTestAPI(Server{})
	k1 := musicscore.DayKey{Member: 1, Date: "2026-10-22"}
	k2 := musicscore.DayKey{Member: 2, Date: "2026-10-22"}
	a.exploreCache.days.Put(k1, 1)
	a.exploreCache.days.Put(k2, 2)
	a.exploreCache.cand.Put("chart:x", 1, time.Hour, time.Now())
	// A movie add leaves the music orders alone.
	a.musicOnEvent(domain.Event{Kind: domain.EventTitleAdded, Data: map[string]any{"why_added": "x"}})
	time.Sleep(60 * time.Millisecond)
	if _, ok := a.exploreCache.days.Get(k1); !ok {
		t.Fatal("a non-music add cleared the day orders")
	}
	// A burst of artist adds clears every member's orders once.
	for i := 0; i < 5; i++ {
		a.musicOnEvent(domain.Event{Kind: domain.EventTitleAdded, Data: map[string]any{"album_types": []string{"album"}}})
	}
	if _, ok := a.exploreCache.days.Get(k1); !ok {
		t.Fatal("cleared before the debounce")
	}
	time.Sleep(80 * time.Millisecond)
	if _, ok := a.exploreCache.days.Get(k1); ok {
		t.Fatal("member 1's order kept after an artist add")
	}
	if _, ok := a.exploreCache.days.Get(k2); ok {
		t.Fatal("the household's orders kept after an artist add")
	}
	if _, ok := a.exploreCache.cand.Get("chart:x", time.Now()); !ok {
		t.Fatal("chart candidates dropped")
	}
	a.exploreCache.days.Put(k1, 1)
	a.musicOnEvent(domain.Event{Kind: domain.EventTitleRemoved})
	time.Sleep(80 * time.Millisecond)
	if _, ok := a.exploreCache.days.Get(k1); ok {
		t.Fatal("a removal kept the order")
	}
}
