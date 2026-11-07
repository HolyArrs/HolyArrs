package api

import (
	"net/http/httptest"
	"testing"
	"time"

	musicscore "github.com/HolyArrs/holyarrs/app/backend/internal/music/score"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
)

func TestMusicAnswerCache(t *testing.T) {
	a := &API{}
	now := time.Now()
	k := musicAnswerKey(7, "rock", "global", 21)
	if k != "7|rock|global|21" {
		t.Fatalf("key %q", k)
	}
	if _, ok := a.cachedMusicAnswer(k, now); ok {
		t.Fatal("an empty cache answered")
	}
	// Only a complete answer is kept.
	a.writeMusicAnswer(httptest.NewRecorder(), k, map[string]int{"n": 1}, false, now)
	if _, ok := a.cachedMusicAnswer(k, now); ok {
		t.Fatal("an incomplete answer was kept")
	}
	rec := httptest.NewRecorder()
	a.writeMusicAnswer(rec, k, map[string]int{"n": 2}, true, now)
	if rec.Body.String() != "{\"n\":2}\n" || rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("written %q", rec.Body.String())
	}
	if b, ok := a.cachedMusicAnswer(k, now.Add(30*time.Second)); !ok || string(b) != "{\"n\":2}\n" {
		t.Fatalf("kept %q %v", b, ok)
	}
	if _, ok := a.cachedMusicAnswer(musicAnswerKey(8, "rock", "global", 21), now); ok {
		t.Fatal("another member read the answer")
	}
	if _, ok := a.cachedMusicAnswer(k, now.Add(musicAnswerTTL)); ok {
		t.Fatal("kept past its minute")
	}
	// An ask forgets every kept answer; other events keep them.
	a.musicOnEvent(domain.Event{Kind: "download_started"})
	if _, ok := a.cachedMusicAnswer(k, now); !ok {
		t.Fatal("an unrelated event forgot the answers")
	}
	a.musicOnEvent(domain.Event{Kind: domain.EventAskMade})
	if _, ok := a.cachedMusicAnswer(k, now); ok {
		t.Fatal("an ask did not forget the answers")
	}
}

func TestMusicTitleChangeClearsDays(t *testing.T) {
	old := musicDayDebounce
	musicDayDebounce = 20 * time.Millisecond
	defer func() { musicDayDebounce = old }()
	a := &API{}
	k1 := musicscore.DayKey{Member: 1, Date: "2026-10-22"}
	k2 := musicscore.DayKey{Member: 2, Date: "2026-10-22"}
	a.musicv2.days.Put(k1, 1)
	a.musicv2.days.Put(k2, 2)
	a.musicv2.cand.Put("chart:x", 1, time.Hour, time.Now())
	// A movie add leaves the music orders alone.
	a.musicOnEvent(domain.Event{Kind: domain.EventTitleAdded, Data: map[string]any{"why_added": "x"}})
	time.Sleep(60 * time.Millisecond)
	if _, ok := a.musicv2.days.Get(k1); !ok {
		t.Fatal("a non-music add cleared the day orders")
	}
	// A burst of artist adds clears every member's orders once.
	for i := 0; i < 5; i++ {
		a.musicOnEvent(domain.Event{Kind: domain.EventTitleAdded, Data: map[string]any{"album_types": []string{"album"}}})
	}
	if _, ok := a.musicv2.days.Get(k1); !ok {
		t.Fatal("cleared before the debounce")
	}
	time.Sleep(80 * time.Millisecond)
	if _, ok := a.musicv2.days.Get(k1); ok {
		t.Fatal("member 1's order kept after an artist add")
	}
	if _, ok := a.musicv2.days.Get(k2); ok {
		t.Fatal("the household's orders kept after an artist add")
	}
	if _, ok := a.musicv2.cand.Get("chart:x", time.Now()); !ok {
		t.Fatal("chart candidates dropped")
	}
	a.musicv2.days.Put(k1, 1)
	a.musicOnEvent(domain.Event{Kind: domain.EventTitleRemoved})
	time.Sleep(80 * time.Millisecond)
	if _, ok := a.musicv2.days.Get(k1); ok {
		t.Fatal("a removal kept the order")
	}
}
