package database

import (
	"context"
	"database/sql"
	"io/fs"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
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

func TestEventsNotedKinds(t *testing.T) {
	s, _ := openTest(t, Options{})
	ctx := context.Background()
	for _, e := range []Event{
		{Kind: "search_done"},
		{Kind: "search_done", Data: map[string]any{"sources_note": "kept apart"}},
		{Kind: "search_done", Data: map[string]any{"sources_note": ""}},
		{Kind: "feed_checked", Data: map[string]any{"sources_note": "fallback"}},
		{Kind: "feed_checked"},
		{Kind: "title_added"},
	} {
		e.Summary = e.Kind
		if err := s.AddEvent(ctx, &e); err != nil {
			t.Fatal(err)
		}
	}
	es, n, err := s.Events(ctx, EventFilter{Kinds: []string{"title_added", "search_done:noted", "feed_checked:noted"}})
	if err != nil {
		t.Fatal(err)
	}
	var notes []string
	for _, e := range es {
		v, _ := e.Data["sources_note"].(string)
		notes = append(notes, e.Kind+"="+v)
	}
	if n != 3 || len(notes) != 3 || notes[0] != "title_added=" || notes[1] != "feed_checked=fallback" || notes[2] != "search_done=kept apart" {
		t.Errorf("noted: %d %v", n, notes)
	}
}

// The History filter must match what the plain LIKE and json_extract filter
// returned, and read the noted searches through their small index.
func TestEventsKindFilterSameAsLike(t *testing.T) {
	s, _ := openTest(t, Options{})
	ctx := context.Background()
	for i, k := range []string{"sign_in", "sign_in_failed", "signal", "sig", "ask_sent", "search_done", "search_done", "feed_checked", "feed_checked", "title_added", "library_folder_added", "library_folderx"} {
		e := Event{Kind: k, Summary: k}
		if i%2 == 1 {
			e.Data = map[string]any{"sources_note": "left out"}
		}
		if err := s.AddEvent(ctx, &e); err != nil {
			t.Fatal(err)
		}
	}
	kinds := []string{"title_added", "sign_in*", "ask_*", "library_folder_*", "search_done:noted", "feed_checked:noted"}
	got, n, err := s.Events(ctx, EventFilter{Kinds: kinds})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := s.readEv.QueryContext(ctx, `SELECT id FROM events WHERE kind = 'title_added' OR kind LIKE 'sign\_in%' ESCAPE '\' OR kind LIKE 'ask\_%' ESCAPE '\'
		OR kind LIKE 'library\_folder\_%' ESCAPE '\' OR (kind IN ('search_done', 'feed_checked') AND coalesce(json_extract(data, '$.sources_note'), '') <> '')
		ORDER BY at DESC, id DESC`)
	if err != nil {
		t.Fatal(err)
	}
	var want, have []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		want = append(want, id)
	}
	rows.Close()
	for _, e := range got {
		have = append(have, e.ID)
	}
	if n != len(want) || !reflect.DeepEqual(have, want) {
		t.Errorf("got %d %v, want %v", n, have, want)
	}
	var plan []string
	rows, err = s.readEv.QueryContext(ctx, "EXPLAIN QUERY PLAN SELECT count(*) FROM events WHERE (kind >= ? AND kind < ?) OR (kind = ? AND coalesce(json_extract(data, '$.sources_note'), '') <> '')", "sign", "sign\U0010FFFF", "search_done")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var a, b, c int
		var d string
		if err := rows.Scan(&a, &b, &c, &d); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, d)
	}
	rows.Close()
	if p := strings.Join(plan, " | "); !strings.Contains(p, "events_noted") || strings.Contains(p, "SCAN events") {
		t.Errorf("plan: %s", p)
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

// A new install and one whose events file stopped at version 2 both end at
// the newest events version with the noted index in place.
func TestEventsNotedIndexFreshAndUpgrade(t *testing.T) {
	check := func(t *testing.T, s *Store) {
		t.Helper()
		v, err := SchemaVersion(s.ReadEvents())
		if err != nil || v != 3 {
			t.Fatalf("events version %d, %v; want 3", v, err)
		}
		var n int
		if err := s.ReadEvents().QueryRow("SELECT count(*) FROM sqlite_master WHERE type='index' AND name='events_noted'").Scan(&n); err != nil || n != 1 {
			t.Fatalf("events_noted index: %d, %v", n, err)
		}
	}
	t.Run("fresh", func(t *testing.T) {
		s, _ := openTest(t, Options{})
		check(t, s)
	})
	t.Run("from version 2", func(t *testing.T) {
		src, _ := fs.Sub(embedded, "migrations/events")
		ms, err := loadMigrations(src)
		if err != nil {
			t.Fatal(err)
		}
		old := fstest.MapFS{}
		for _, m := range ms {
			if m.version <= 2 {
				old[m.name] = &fstest.MapFile{Data: []byte(m.sql)}
			}
		}
		dir := t.TempDir()
		s, err := Open(dir, Options{EventsMigrations: old})
		if err != nil {
			t.Fatal(err)
		}
		if v, _ := SchemaVersion(s.ReadEvents()); v != 2 {
			t.Fatalf("old events version %d, want 2", v)
		}
		_ = s.Close()
		s, err = Open(dir, Options{})
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		check(t, s)
	})
}
