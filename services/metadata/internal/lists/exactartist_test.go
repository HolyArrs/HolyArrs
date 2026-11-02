package lists

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

// A chart row's own Spotify artist id, resolved exactly, beats the name
// match; an album whose name match is another artist's loses that id (the
// served chart finds it again under the right artist); an id that does not
// resolve leaves the name match.
func TestChartListUsesTheRowsArtistID(t *testing.T) {
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	exact := fakeID("exact")
	var asked []string
	s, err := Open(filepath.Join(t.TempDir(), "l.sqlite"), Options{Key: "k", Now: func() time.Time { return now },
		ExactArtist: func(_ context.Context, source, id, _ string, _ []string) string {
			asked = append(asked, source+":"+id)
			if id == "0OjRa6r1EIPF5RZPy1SGz9" {
				return exact
			}
			return ""
		}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	put := func(key string, es []Entry) {
		b, _ := json.Marshal(es)
		_, _ = s.db.Exec(`INSERT OR REPLACE INTO lists_day (key, day, body) VALUES (?, '2026-10-26', ?)`, key, b)
	}
	byName, alb := fakeID("byname"), fakeID("album")
	_, _ = s.db.Exec(`INSERT INTO matches (kind, name, artist, mbid, artist_mbid, matched_at) VALUES ('artist','Basta','',?,'',1),('album','Big','Basta',?,?,1),('artist','Other','',?,'',1)`, byName, alb, byName, byName)
	put("artists:ru", []Entry{{Rank: 1, Title: "Basta", Artist: "Basta", ExtArtist: "0OjRa6r1EIPF5RZPy1SGz9"}, {Rank: 2, Title: "Other", Artist: "Other", ExtArtist: "1111111111111111111111"}})
	put("albums:ru", []Entry{{Rank: 1, Title: "Big", Artist: "Basta", ExtArtist: "0OjRa6r1EIPF5RZPy1SGz9"}})
	c, _ := s.ChartList(context.Background(), "ru", "artists")
	if c.Entries[0].MBID != exact || c.Entries[1].MBID != byName {
		t.Fatalf("artists %+v", c.Entries)
	}
	c, _ = s.ChartList(context.Background(), "ru", "albums")
	if c.Entries[0].ArtistMBID != exact || c.Entries[0].MBID != "" {
		t.Fatalf("albums %+v", c.Entries)
	}
	if len(asked) == 0 || asked[0] != "spotify:0OjRa6r1EIPF5RZPy1SGz9" {
		t.Fatalf("asked %v", asked)
	}
}

// A spotify81 row without exact evidence whose name several artists carry
// links to none.
func TestChartListSharedNameLinksNoArtist(t *testing.T) {
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	s, err := Open(filepath.Join(t.TempDir(), "l.sqlite"), Options{Key: "k", Now: func() time.Time { return now },
		ExactArtist: func(context.Context, string, string, string, []string) string { return "" },
		SharedName:  func(_ context.Context, name string) bool { return name == "Omar" }})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	byName := fakeID("byname")
	_, _ = s.db.Exec(`INSERT INTO matches (kind, name, artist, mbid, artist_mbid, matched_at) VALUES ('artist','Omar','',?,'',1)`, byName)
	b, _ := json.Marshal([]Entry{{Rank: 1, Title: "Omar", Artist: "Omar", ExtArtist: "1GLzVjVa7bLnAcD1At98j3"}})
	_, _ = s.db.Exec(`INSERT INTO lists_day (key, day, body) VALUES ('artists:dk', '2026-10-26', ?)`, b)
	c, _ := s.ChartList(context.Background(), "dk", "artists")
	if len(c.Entries) != 1 || c.Entries[0].MBID != "" || c.Entries[0].Name != "Omar" {
		t.Fatalf("entries %+v", c.Entries)
	}
}
