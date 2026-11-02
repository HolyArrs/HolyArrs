package api

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/model"
	"github.com/CoderLuii/holyarrs-metadata/internal/refresh"
)

func refreshSetup(t *testing.T) (*Server, *memoryStore, *atomic.Int64, *refresh.Tracker) {
	tr, err := refresh.Open(filepath.Join(t.TempDir(), "refresh.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tr.Close() })
	s, m, calls := setup(Options{Burst: 100, Refresh: tr})
	return s, m, calls, tr
}

func waitCalls(t *testing.T, calls *atomic.Int64, want int64) {
	t.Helper()
	for i := 0; i < 200 && calls.Load() < want; i++ {
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond)
	if calls.Load() != want {
		t.Fatalf("calls %d, want %d", calls.Load(), want)
	}
}

func lookupKey() string {
	return model.Query{Provider: "tmdb", Kind: "movie", Operation: "detail", ID: "42", Page: 1, Limit: 20}.Key()
}

func TestRefreshServesStoredAndRevalidatesInBackground(t *testing.T) {
	s, m, calls, tr := refreshSetup(t)
	if w := req(s, lookup, "synthetic-test-token"); w.Code != 200 || w.Header().Get("X-Metadata-Cache") != "miss" {
		t.Fatalf("miss %d %s", w.Code, w.Header().Get("X-Metadata-Cache"))
	}
	key := lookupKey()
	if _, _, ok := tr.State(context.Background(), key); !ok {
		t.Fatal("fetch not tracked")
	}
	// TMDB answers verify after the 14-day backstop, not the 24 h TTL.
	m.mu.Lock()
	e := m.entries[key]
	m.mu.Unlock()
	if d := e.ExpiresAt.Sub(e.FetchedAt); d != refresh.DefaultBackstop {
		t.Fatalf("verify-after %v", d)
	}
	if w := req(s, lookup, "synthetic-test-token"); w.Header().Get("X-Metadata-Cache") != "hit" || calls.Load() != 1 {
		t.Fatal("fresh answer refetched")
	}
	// A feed marks it changed: served at once from the store, one refresh behind.
	tr.MarkRefs(context.Background(), []string{"tmdb:movie:42"})
	for range 3 {
		if w := req(s, lookup, "synthetic-test-token"); w.Code != 200 {
			t.Fatalf("stale status %d", w.Code)
		}
	}
	waitCalls(t, calls, 2)
	if _, stale, _ := tr.State(context.Background(), key); stale {
		t.Fatal("still stale after refresh")
	}
	// Past verify-after: also served, then refreshed.
	m.mu.Lock()
	e = m.entries[key]
	e.ExpiresAt = time.Now().Add(-time.Minute)
	m.entries[key] = e
	m.mu.Unlock()
	if w := req(s, lookup, "synthetic-test-token"); w.Code != 200 || w.Header().Get("X-Metadata-Cache") != "stale" {
		t.Fatalf("expired %d %s", w.Code, w.Header().Get("X-Metadata-Cache"))
	}
	waitCalls(t, calls, 3)
}

func TestRefreshNudgesTitlePageAfterSixHours(t *testing.T) {
	s, _, calls, tr := refreshSetup(t)
	req(s, lookup, "synthetic-test-token")
	key := lookupKey()
	// Seven hours since verification: answered as a hit, refreshed behind.
	now := time.Now()
	tr.Now = func() time.Time { return now.Add(-7 * time.Hour) }
	tr.Record(context.Background(), key, `{}`, "tmdb:movie:42")
	tr.Now = time.Now
	if w := req(s, lookup, "synthetic-test-token"); w.Header().Get("X-Metadata-Cache") != "hit" {
		t.Fatal("nudge did not answer from the store")
	}
	waitCalls(t, calls, 2)
}

func TestRefreshCountsSavedCallsAndShowsStats(t *testing.T) {
	s, _, calls, tr := refreshSetup(t)
	s.opts.TTL = time.Hour
	s.opts.NudgeAfter = 48 * time.Hour
	req(s, lookup, "synthetic-test-token")
	now := time.Now()
	tr.Now = func() time.Time { return now.Add(-2 * time.Hour) }
	tr.Record(context.Background(), lookupKey(), `{}`, "tmdb:movie:42")
	tr.Now = time.Now
	req(s, lookup, "synthetic-test-token")
	if calls.Load() != 1 {
		t.Fatal("refetched")
	}
	st := tr.Snapshot(context.Background())
	if st.UpstreamCallsSaved != 1 {
		t.Fatalf("saved %d", st.UpstreamCallsSaved)
	}
	raw, _ := json.Marshal(st)
	for _, f := range []string{"store_items", "stale", "changes_applied", "upstream_calls_saved", "median_change_to_applied_seconds"} {
		var m map[string]any
		json.Unmarshal(raw, &m)
		if _, ok := m[f]; !ok {
			t.Fatalf("stats missing %s", f)
		}
	}
}

func TestRevalidateRefetchesTrackedRow(t *testing.T) {
	s, _, calls, tr := refreshSetup(t)
	req(s, lookup, "synthetic-test-token")
	items, _ := tr.Backstop(context.Background(), time.Now().Add(time.Hour), 10)
	if len(items) != 1 {
		t.Fatalf("items %d", len(items))
	}
	if err := s.Revalidate(context.Background(), items[0]); err != nil || calls.Load() != 2 {
		t.Fatalf("revalidate %v calls %d", err, calls.Load())
	}
}

func TestMusicTTLWithoutFeed(t *testing.T) {
	s, _, _, _ := refreshSetup(t)
	q := model.Query{Provider: "musicbrainz", Kind: "artist", Operation: "detail", ID: "x"}
	if d := s.expiry(q, model.Result{}); d != 30*24*time.Hour {
		t.Fatalf("music ttl %v", d)
	}
	s.opts.MusicFeed = true
	if d := s.expiry(q, model.Result{}); d != refresh.DefaultBackstop {
		t.Fatalf("music feed ttl %v", d)
	}
}
