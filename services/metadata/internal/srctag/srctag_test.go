package srctag

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func open(t *testing.T, name string) (*sql.DB, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	db, err := sql.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db, p
}

func TestPutRefusesUntagged(t *testing.T) {
	db, _ := open(t, "slim.sqlite")
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := Put(context.Background(), db, "album", "a1", "cover", Tag{}, time.Now()); !errors.Is(err, ErrUntagged) {
		t.Fatalf("untagged write got %v", err)
	}
	if err := PutDump(context.Background(), db, DumpMeta{Table: "artist"}); err == nil {
		t.Fatal("dump tag without date or schema sequence was kept")
	}
}

func TestReportCountsEverySourceAndNoUntagged(t *testing.T) {
	ctx := context.Background()
	db, _ := open(t, "slim.sqlite")
	img, imgPath := open(t, "images.sqlite")
	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}
	if _, err := img.Exec(`CREATE TABLE source_ids(kind TEXT, mbid TEXT, source TEXT, store TEXT, ext TEXT, at INTEGER, fetched_at INTEGER, via_ip TEXT);
INSERT INTO source_ids VALUES('artist','a','applemusic','us','1',5,5,''),('album','b','applemusic','us','2',5,5,''),('artist','c','spotify','link','x',5,5,'')`); err != nil {
		t.Fatal(err)
	}
	for _, m := range []DumpMeta{{"artist", "20261028-002228", 31, 100}, {"release", "20261028-002228", 31, 50}} {
		if err := PutDump(ctx, db, m); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Unix(1000, 0)
	if err := Put(ctx, db, "album", "b", "cover", Tag{Source: "deezer", Ref: "302127"}, now); err != nil {
		t.Fatal(err)
	}
	if err := Put(ctx, db, "artist", "a", "id", Tag{Source: "mb-live", ViaIP: "10.0.0.2", FetchedAt: time.Unix(2000, 0)}, now); err != nil {
		t.Fatal(err)
	}
	got, err := Report(ctx, db, imgPath)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int64{"dump/musicbrainz-dump:20261028-002228": 150, "override/deezer": 1, "override/mb-live": 1,
		"picture-id/applemusic": 2, "picture-id/spotify": 1}
	if len(got) != len(want) {
		t.Fatalf("report %+v", got)
	}
	for _, c := range got {
		if want[c.Layer+"/"+c.Source] != c.N || c.Untagged != 0 {
			t.Fatalf("line %+v", c)
		}
	}
	var at int64
	var ip string
	if err := db.QueryRow(`SELECT fetched_at, via_ip FROM source_tags WHERE source='mb-live'`).Scan(&at, &ip); err != nil || at != 2000 || ip != "10.0.0.2" {
		t.Fatalf("mb-live tag %d %q %v", at, ip, err)
	}
}

// L7: re-stamping while another connection writes never leaves a stored
// row untagged (the triggers are dropped and made in one transaction).
func TestStampNoUntaggedWriteWhileRestamping(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "race.sqlite")+"?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`CREATE TABLE t (k TEXT PRIMARY KEY, body BLOB)`); err != nil {
		t.Fatal(err)
	}
	spec := Spec{Table: "t", Key: []string{"k"}, Body: "body", Src: "'x'"}
	if err := Stamp(db, spec); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 30; i++ {
			if err := Stamp(db, spec); err != nil {
				t.Error(err)
				return
			}
		}
	}()
	for i := 0; i < 300; i++ {
		if _, err := db.Exec(`INSERT INTO t(k, body) VALUES(?, 'v')`, i); err != nil {
			t.Fatal(err)
		}
	}
	<-done
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM t WHERE src<>'x'`).Scan(&n)
	if n != 0 {
		t.Fatalf("%d rows untagged", n)
	}
}

// A table whose empty rows are real answers ("the source has none") tags
// them too: on write and in the backfill.
func TestStampTagsEmptyRowsWhenAsked(t *testing.T) {
	db, _ := open(t, "s.sqlite")
	if _, err := db.Exec(`CREATE TABLE similar (mbid TEXT PRIMARY KEY, body BLOB, none INTEGER NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO similar VALUES ('old', NULL, 1)`); err != nil {
		t.Fatal(err)
	}
	if err := Stamp(db, Spec{Table: "similar", Key: []string{"mbid"}, Body: "body", Src: "'listenbrainz'", Empty: "'none-src'", Known: "1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO similar(mbid, body, none) VALUES ('new', NULL, 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO similar(mbid, body, none) VALUES ('full', x'7b7d', 0)`); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query(`SELECT mbid, src, fetched_at FROM similar ORDER BY mbid`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	want := map[string]string{"full": "listenbrainz", "new": "none-src", "old": "none-src"}
	for rows.Next() {
		var id, src string
		var at int64
		if err := rows.Scan(&id, &src, &at); err != nil {
			t.Fatal(err)
		}
		if src != want[id] || at <= 0 {
			t.Fatalf("%s: src %q at %d", id, src, at)
		}
	}
}
