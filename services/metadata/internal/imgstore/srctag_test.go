package imgstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/srctag"
)

func TestSourceIDsMigrationAddsFetchAndAddress(t *testing.T) {
	dir := t.TempDir()
	old, err := sql.Open("sqlite", filepath.Join(dir, "images.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(`CREATE TABLE source_ids(kind TEXT NOT NULL, mbid TEXT NOT NULL, source TEXT NOT NULL, store TEXT NOT NULL, ext TEXT NOT NULL, at INTEGER NOT NULL, PRIMARY KEY(kind, mbid, source));
INSERT INTO source_ids VALUES('artist','a','applemusic','us','1',777)`); err != nil {
		t.Fatal(err)
	}
	old.Close()
	now := time.Unix(5000, 0)
	s, err := Open(Options{Dir: dir, Logf: func(string, ...any) {}, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var at int64
	var ip string
	if err := s.db.QueryRow(`SELECT fetched_at, via_ip FROM source_ids WHERE mbid='a'`).Scan(&at, &ip); err != nil || at != 777 || ip != "" {
		t.Fatalf("old row %d %q %v", at, ip, err)
	}
	s.KeepID(KindArtist, "b", srctag.Tag{Source: "spotify", ViaIP: "10.0.0.3", FetchedAt: time.Unix(4000, 0)}, "link", "x")
	s.KeepID(KindArtist, "c", srctag.Tag{}, "link", "y")
	if err := s.db.QueryRow(`SELECT fetched_at, via_ip FROM source_ids WHERE mbid='b'`).Scan(&at, &ip); err != nil || at != 4000 || ip != "10.0.0.3" {
		t.Fatalf("tagged row %d %q %v", at, ip, err)
	}
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM source_ids WHERE mbid='c'`).Scan(&n)
	if n != 0 {
		t.Fatal("a match without a source was kept")
	}
}

func TestMusicBytesSkipsTitleAndPersonKinds(t *testing.T) {
	s, err := Open(Options{Dir: t.TempDir(), Logf: func(string, ...any) {}})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.db.Exec(`INSERT INTO images(kind,mbid,source,source_url,version,bytes,fetched,checked) VALUES
('artist','1','x','u','v',10,0,0),('album','2','x','u','v',20,0,0),('wide','3','x','u','v',30,0,0),('logo','4','x','u','v',40,0,0),
('title-banner','5','x','u','v',1000,0,0),('person','6','x','u','v',2000,0,0)`); err != nil {
		t.Fatal(err)
	}
	if n, err := s.MusicBytes(context.Background()); err != nil || n != 100 {
		t.Fatalf("music bytes %d %v", n, err)
	}
}

// A picture's public record carries its source and fetch date.
func TestInfoCarriesSourceAndFetchDate(t *testing.T) {
	at := time.Date(2026, 10, 30, 1, 2, 3, 0, time.UTC)
	i := Record{Provider: "applemusic", Version: "v1", Fetched: at}.Info()
	if i.Source != "applemusic" || i.FetchedAt != "2026-10-30T01:02:03Z" {
		t.Fatalf("%+v", i)
	}
	b, _ := json.Marshal(i)
	if !strings.Contains(string(b), `"src":"applemusic"`) || !strings.Contains(string(b), `"fetched_at":"2026-10-30T01:02:03Z"`) {
		t.Fatal(string(b))
	}
}
