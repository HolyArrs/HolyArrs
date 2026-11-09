package database

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func TestEventsBySourceAndHideFeed(t *testing.T) {
	s, _ := openTest(t, Options{})
	ctx := context.Background()
	now := FormatTime(time.Now())
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		for _, q := range []string{
			"INSERT INTO sources (id, created_at, created_by, updated_at, updated_by, name, protocol, format, url) VALUES (1, ?1, 't', ?1, 't', 'one', 'torrent', 'torznab', 'http://a'), (2, ?1, 't', ?1, 't', 'two', 'torrent', 'torznab', 'http://b')",
			"INSERT INTO releases (id, created_at, created_by, updated_at, updated_by, source_id, name, link_ref, protocol, found_by) VALUES (10, ?1, 't', ?1, 't', 1, 'r1', 'x', 'torrent', 'feed'), (20, ?1, 't', ?1, 't', 2, 'r2', 'y', 'torrent', 'feed')",
		} {
			if _, err := tx.Exec(q, now); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	day := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	for i, e := range []Event{
		{Kind: "download_sent", Data: map[string]any{"release_id": 10}},
		{Kind: "release_chosen", Data: map[string]any{"release_id": 20}},
		{Kind: "source_paused", Data: map[string]any{"source_id": 1, "reason": "down"}},
		{Kind: "feed_checked"},
		{Kind: "title_added"},
	} {
		e.Summary, e.At = e.Kind, day.Add(time.Duration(i)*time.Hour)
		if err := s.AddEvent(ctx, &e); err != nil {
			t.Fatal(i, err)
		}
	}
	count := func(f EventFilter) (int, []string) {
		es, n, err := s.Events(ctx, f)
		if err != nil {
			t.Fatal(err)
		}
		var ks []string
		for _, e := range es {
			ks = append(ks, e.Kind)
		}
		return n, ks
	}
	if n, ks := count(EventFilter{SourceID: 1, SourceReleaseIDs: []int64{10}}); n != 2 || ks[0] != "source_paused" || ks[1] != "download_sent" {
		t.Errorf("source 1: %d %v", n, ks)
	}
	if n, ks := count(EventFilter{SourceID: 2, SourceReleaseIDs: []int64{20}}); n != 1 || ks[0] != "release_chosen" {
		t.Errorf("source 2: %d %v", n, ks)
	}
	if n, _ := count(EventFilter{SourceID: 3}); n != 0 {
		t.Errorf("source 3: %d", n)
	}
	if n, _ := count(EventFilter{HideFeed: true}); n != 4 {
		t.Errorf("hide feed: %d", n)
	}
	if n, _ := count(EventFilter{}); n != 5 {
		t.Errorf("all: %d", n)
	}
}

func TestAddEventRunsHookThroughGuard(t *testing.T) {
	ctx := context.Background()
	t.Cleanup(func() { eventGuard.Store(nil) })

	// With a guard set, every hook runs through it, and a crash inside the
	// hook does not fail AddEvent.
	var got []string
	SetEventGuard(func(name string, f func()) {
		got = append(got, name)
		defer func() { _ = recover() }()
		f()
	})
	s, _ := openTest(t, Options{})
	s.OnEvent(func(Event) { panic("boom") })
	e := &Event{Kind: "x", At: time.Now()}
	if err := s.AddEvent(ctx, e); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "events" {
		t.Fatalf("guard not used: %v", got)
	}

	// With no guard set, the built-in one catches the crash.
	eventGuard.Store(nil)
	e2 := &Event{Kind: "y", At: time.Now()}
	if err := s.AddEvent(ctx, e2); err != nil {
		t.Fatal(err)
	}
	es, _, err := s.Events(ctx, EventFilter{Kinds: []string{"y"}})
	if err != nil || len(es) != 1 {
		t.Fatalf("event row missing: %v %v", es, err)
	}
	if len(got) != 1 {
		t.Fatalf("old guard called after reset: %v", got)
	}
}
