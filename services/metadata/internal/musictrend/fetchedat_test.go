package musictrend

import (
	"path/filepath"
	"testing"
	"time"
)

// AC2: every music list row carries its source and fetch time, and a
// ListenBrainz chart row stored with fetched_at 0 takes the time its body
// carries.
func TestListsAndLBChartsCarryFetchedAt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m.sqlite")
	s, err := Open(path, Options{Search: &fakeSearch{}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO lists (kind, region, body) VALUES ('artists','global','{"source":"spotify81","updated":1800000000}')`); err != nil {
		t.Fatal(err)
	}
	var src string
	var at int64
	if err := s.db.QueryRow(`SELECT src, fetched_at FROM lists`).Scan(&src, &at); err != nil || src != "spotify81" || at == 0 {
		t.Fatalf("lists row %q %d %v", src, at, err)
	}
	// An lb_charts row from before, with no fetch time.
	_, _ = s.db.Exec(`INSERT OR REPLACE INTO lb_charts (id, body) VALUES (1, '{"at":1800000123}')`)
	_, _ = s.db.Exec(`UPDATE lb_charts SET fetched_at=0`)
	s.Close()
	s2, err := Open(path, Options{Search: &fakeSearch{}})
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if err := s2.db.QueryRow(`SELECT fetched_at FROM lb_charts WHERE id=1`).Scan(&at); err != nil || at != 1800000123 {
		t.Fatalf("lb_charts fetched_at %d %v", at, err)
	}
	// A new write is stamped with the write time.
	s2.saveLB()
	if err := s2.db.QueryRow(`SELECT fetched_at FROM lb_charts WHERE id=1`).Scan(&at); err != nil || at < time.Now().Unix()-60 {
		t.Fatalf("write not stamped: %d %v", at, err)
	}
}

// AC10: the gate opens only when every chart artist has a photo or a
// recorded miss (and every album a cover or a miss).
func TestGateNeedsPhotoOrRecordedMissForEveryArtist(t *testing.T) {
	c := Coverage{ChartArtists: 3, Artists: 3, ArtistPhotos: 2, ArtistMisses: 0, Albums: 1, AlbumCovers: 1}
	if Gate(c) {
		t.Fatal("open with an artist neither photographed nor missed")
	}
	c.ArtistMisses = 1
	if !Gate(c) {
		t.Fatal("closed with every artist a photo or a recorded miss")
	}
	c.AlbumCovers = 0
	if Gate(c) {
		t.Fatal("open with an album neither covered nor missed")
	}
}
