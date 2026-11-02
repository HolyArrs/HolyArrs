package main

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"
	"github.com/CoderLuii/holyarrs-metadata/internal/srctag"
)

func TestMusicSourceCensus(t *testing.T) {
	st, err := imgstore.Open(imgstore.Options{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	st.KeepID("artist", "a1", srctag.Tag{Source: "mb-live", FetchedAt: time.Now()}, "mb-live", "a1")
	st.KeepID("artist", "a2", srctag.Tag{Source: "deezer"}, "x", "9")
	st.KeepID("artist", "a3", srctag.Tag{Source: "mb-live"}, "", "")
	n := 0
	mc := &musicSourceCensus{imgs: st, now: func() time.Time { n++; return time.Unix(int64(1000+n*300), 0) },
		dump: func(context.Context) (string, map[string]int64) {
			return "20261027-000000", map[string]int64{"artist": 5, "release_group": 7, "recording": 11}
		}}
	got, err := mc.compute(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	rows := got["sources"].([]sourceRow)
	if len(rows) != 2 || rows[0].Source != "mb-dump" || rows[0].Songs != 11 || rows[1].Source != "mb-live" || rows[1].Artists != 1 {
		t.Fatalf("rows %+v", rows)
	}
	if got["with_source_and_fetched_pct"].(float64) != 100 || got["total"].(int64) != 24 {
		t.Fatalf("%v", got)
	}
	if mc.get(context.Background())["total"] != int64(24) {
		t.Fatal("get")
	}
}

func TestStoreCountsTagged(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "discog.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`CREATE TABLE artists(mbid TEXT PRIMARY KEY, body BLOB NOT NULL DEFAULT '', walked INTEGER NOT NULL DEFAULT 0) WITHOUT ROWID;
CREATE TABLE albums(mbid TEXT PRIMARY KEY, body BLOB NOT NULL DEFAULT '', walked INTEGER NOT NULL DEFAULT 0) WITHOUT ROWID;
INSERT INTO artists(mbid, body, walked) VALUES('old', 'x', 5);`); err != nil {
		t.Fatal(err)
	}
	for _, tb := range []string{"artists", "albums"} {
		if err := srctag.Stamp(db, srctag.Spec{Table: tb, Key: []string{"mbid"}, Body: "body", Src: "'musicbrainz'", Back: "'" + srctag.Untagged + "'", Known: "walked"}); err != nil {
			t.Fatal(err)
		}
	}
	db.Exec(`INSERT INTO albums(mbid, body) VALUES('a', 'x'); INSERT INTO albums(mbid) VALUES('queued')`)
	db.Close()
	var a, u int64
	for _, c := range storeCounts(context.Background(), dir) {
		a += c.N
		u += c.Untagged
		if c.Kind == "albums" && (c.Source != "musicbrainz" || c.N != 1 || c.Untagged != 0) {
			t.Fatalf("%+v", c)
		}
	}
	if a != 2 || u != 1 {
		t.Fatalf("total %d untagged %d", a, u)
	}
}

// Wide pictures and logos count under their source in their own columns
// (audit 2026-10-29: about 14,700 were not shown on their own).
func TestPictureCountsWideAndLogos(t *testing.T) {
	var r sourceRow
	for _, c := range []imgstore.SourceCount{{Kind: "artist", Source: "fanart", N: 2}, {Kind: "wide", Source: "fanart", N: 5}, {Kind: "logo", Source: "fanart", N: 7}, {Kind: "album", Source: "fanart", N: 1}} {
		addPictureCount(&r, c)
	}
	if r.Photos != 2 || r.Wide != 5 || r.Logos != 7 || r.Covers != 1 {
		t.Fatalf("%+v", r)
	}
}
