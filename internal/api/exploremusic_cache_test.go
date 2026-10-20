package api

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
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
