package lists

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

func TestChartListAndGenreTiles(t *testing.T) {
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	s, err := Open(filepath.Join(t.TempDir(), "l.sqlite"), Options{Key: "k", Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	put := func(key, day string, es []Entry) {
		b, _ := json.Marshal(es)
		_, _ = s.db.Exec(`INSERT OR REPLACE INTO lists_day (key, day, body) VALUES (?, ?, ?)`, key, day, b)
	}
	match := func(kind, name, artist, id string) {
		_, _ = s.db.Exec(`INSERT OR REPLACE INTO matches (kind, name, artist, mbid, artist_mbid, matched_at) VALUES (?, ?, ?, ?, '', ?)`, kind, name, artist, id, now.Unix())
	}
	alb, art := fakeID("album"), fakeID("artist")
	match("album", "Big (Deluxe Edition)", "A", alb)
	match("artist", "A", "", art)
	put("albums:us", "2026-10-26", []Entry{{Rank: 1, Title: "Big (Deluxe Edition)", Artist: "A", ReleaseDate: "2026-10-23"}, {Rank: 2, Title: "Unmatched", Artist: "B"}})
	put("albums:us", "2026-10-19", []Entry{{Rank: 1, Title: "Unmatched", Artist: "B"}})
	c, ok := s.ChartList(context.Background(), "us", "albums")
	if !ok || len(c.Entries) != 2 || c.Entries[0].MBID != alb || c.Entries[0].ArtistMBID != art || c.Entries[0].Name != "Big" || c.Entries[1].MBID != "" {
		t.Fatalf("chart %+v", c)
	}
	if !c.HaveAgo || len(c.Ago) != 1 || c.Ago[0].Name != "Unmatched" {
		t.Fatalf("seven days ago %+v", c)
	}
	if _, ok := s.ChartList(context.Background(), "gb", "albums"); ok {
		t.Fatal("a region without a chart answered")
	}
	// Genre tiles: only genres with items, in plan order, with their artists.
	put("list:pop:0", "2026-10-26", []Entry{{Rank: 1, Title: "Song", Artist: "A", Album: "Big (Deluxe Edition)"}})
	put("list:chill:0", "2026-10-26", []Entry{{Rank: 1, Title: "Song", Artist: "A", Album: "Big (Deluxe Edition)"}})
	s.rebuild(context.Background(), false)
	g := s.GenreTiles()
	if len(g) != 1 || g[0].Slug != "pop" || g[0].Name != "Pop" || g[0].Count != 1 || g[0].Artists[0].ID != art {
		t.Fatalf("genre tiles %+v", g)
	}
	if s.Built().IsZero() {
		t.Fatal("built time missing")
	}
}

func TestChartListStaleGivesWayAndLookupMemo(t *testing.T) {
	now := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
	s, err := Open(filepath.Join(t.TempDir(), "l.sqlite"), Options{Key: "k", Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	put := func(key, day string) {
		b, _ := json.Marshal([]Entry{{Rank: 1, Title: "Big", Artist: "A"}})
		_, _ = s.db.Exec(`INSERT OR REPLACE INTO lists_day (key, day, body) VALUES (?, ?, ?)`, key, day, b)
	}
	put("albums:us", "2026-10-29")
	if _, ok := s.ChartList(context.Background(), "us", "albums"); ok {
		t.Fatal("a 12-day-old list answered instead of giving way to Apple")
	}
	put("albums:gb", "2026-11-05")
	if _, ok := s.ChartList(context.Background(), "gb", "albums"); !ok {
		t.Fatal("a 5-day-old list did not answer")
	}
	put("albums:de", "odd")
	if _, ok := s.ChartList(context.Background(), "de", "albums"); !ok {
		t.Fatal("an unreadable day did not count as fresh")
	}
	// The lookup keeps its answers (misses too) for the life of one call.
	look := s.lookup(context.Background())
	if _, _, ok := look("artist", "A", ""); ok {
		t.Fatal("unmatched artist found")
	}
	id := fakeID("artist")
	_, _ = s.db.Exec(`INSERT OR REPLACE INTO matches (kind, name, artist, mbid, artist_mbid, matched_at) VALUES ('artist', 'A', '', ?, '', ?)`, id, now.Unix())
	if _, _, ok := look("artist", "A", ""); ok {
		t.Fatal("the same lookup asked the store again")
	}
	if got, _, ok := s.lookup(context.Background())("artist", "A", ""); !ok || got != id {
		t.Fatalf("a new lookup did not see the match: %q", got)
	}
}
