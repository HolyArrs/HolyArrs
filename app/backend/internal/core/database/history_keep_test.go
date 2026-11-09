package database

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestHistoryKeepSettings(t *testing.T) {
	s, _ := openTest(t, Options{})
	ctx := context.Background()
	if k := ReadHistoryKeep(ctx, s.Read()); k != DefaultHistoryKeep {
		t.Fatalf("default: %+v", k)
	}
	if err := s.SaveHistoryKeep(ctx, HistoryKeep{RoutineDays: -1}, System); err != ErrHistoryKeep {
		t.Fatalf("negative: %v", err)
	}
	if err := s.SaveHistoryKeep(ctx, HistoryKeep{RoutineDays: 7, OtherDays: 365}, System); err != nil {
		t.Fatal(err)
	}
	if k := ReadHistoryKeep(ctx, s.Read()); k.RoutineDays != 7 || k.OtherDays != 365 {
		t.Fatalf("saved: %+v", k)
	}
}

func TestPruneEvents(t *testing.T) {
	s, _ := openTest(t, Options{})
	ctx := context.Background()
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	add := func(kind string, age time.Duration) {
		e := Event{Kind: kind, Summary: kind, At: now.Add(-age)}
		if err := s.AddEvent(ctx, &e); err != nil {
			t.Fatal(err)
		}
	}
	day := 24 * time.Hour
	add("feed_checked", 40*day)  // routine, old: goes
	add("search_done", 10*day)   // routine, new: stays
	add("import_done", 400*day)  // other: kept forever by default
	add("settings_changed", day) // other, new
	for range pruneBatch + 5 {   // more than one batch
		add("title_refreshed", 31*day)
	}
	n, err := s.PruneEvents(ctx, DefaultHistoryKeep, now)
	if err != nil || n != pruneBatch+6 {
		t.Fatalf("pruned %d, %v", n, err)
	}
	kinds := func() map[string]int {
		es, _, err := s.Events(ctx, EventFilter{Limit: 500})
		if err != nil {
			t.Fatal(err)
		}
		m := map[string]int{}
		for _, e := range es {
			m[e.Kind]++
		}
		return m
	}
	if k := kinds(); len(k) != 3 || k["search_done"] != 1 || k["import_done"] != 1 || k["settings_changed"] != 1 {
		t.Fatalf("left: %v", k)
	}
	add("sign_in_failed", 900*day) // audit: past its fixed 365 days
	add("signed_in", 300*day)      // audit: kept
	if n, err := s.PruneEvents(ctx, HistoryKeep{OtherDays: 30}, now); err != nil || n != 2 {
		t.Fatalf("other: %d %v", n, err)
	}
	if k := kinds(); k["import_done"] != 0 || k["settings_changed"] != 1 || k["search_done"] != 1 || k["sign_in_failed"] != 0 || k["signed_in"] != 1 {
		t.Fatalf("left after other: %v", k)
	}
}

func TestEventIndexes(t *testing.T) {
	s, _ := openTest(t, Options{})
	for _, name := range []string{"events_download", "events_kind_at"} {
		var n int
		if err := s.ReadEvents().QueryRow("SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name = ?", name).Scan(&n); err != nil || n != 1 {
			t.Errorf("%s: %d %v", name, n, err)
		}
	}
	var plan string
	_ = s.ReadEvents().QueryRow("EXPLAIN QUERY PLAN SELECT at FROM events WHERE download_id = 5").Scan(new(int), new(int), new(int), &plan)
	if plan == "" || !strings.Contains(plan, "events_download") {
		t.Errorf("download_id lookup plan: %q", plan)
	}
}
