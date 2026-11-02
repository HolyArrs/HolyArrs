package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoderLuii/holyarrs-metadata/internal/imgstore"
)

func mk(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	must := func(path string, stmts ...string) {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, path)), 0o755); err != nil {
			t.Fatal(err)
		}
		db, err := sql.Open("sqlite", filepath.Join(dir, path))
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		for _, s := range stmts {
			if _, err := db.Exec(s); err != nil {
				t.Fatal(s, err)
			}
		}
	}
	must("img/images.sqlite",
		`CREATE TABLE images(kind TEXT, mbid TEXT, bytes INTEGER, PRIMARY KEY(kind,mbid))`,
		`CREATE TABLE misses(kind TEXT, mbid TEXT, until INTEGER, PRIMARY KEY(kind,mbid))`,
		`CREATE TABLE source_ids(kind TEXT, mbid TEXT, source TEXT, PRIMARY KEY(kind,mbid,source))`,
		`INSERT INTO images VALUES ('artist','a1',10),('album','b1',20),('wide','w1',5),('logo','l1',1),('title-banner','t1',100),('title-logo','t2',100),('title-wide','t3',100),('person','p1',50)`,
		`INSERT INTO misses VALUES ('artist','a2',1),('person','p2',1),('title-wide','t4',1)`,
		`INSERT INTO source_ids VALUES ('album','b1','deezer'),('person','p1','tmdb'),('title-logo','t2','fanart')`)
	must("fetchq-images.sqlite", `CREATE TABLE fetch_jobs(key TEXT PRIMARY KEY)`,
		`INSERT INTO fetch_jobs VALUES ('img:album:b1'),('big:logo:l1'),('img:person:p1'),('img:title-logo:t2')`)
	must("music.sqlite", `CREATE TABLE chart_days(day TEXT, store TEXT, body BLOB, PRIMARY KEY(day,store))`,
		`CREATE TABLE extra(x)`, `INSERT INTO chart_days VALUES ('d','s',x'0102')`, `INSERT INTO extra VALUES (1)`)
	must("cache.sqlite", `CREATE TABLE entries(key TEXT PRIMARY KEY)`, `INSERT INTO entries VALUES ('movie:1'),('tv:2')`)
	for _, k := range []string{"artist", "album", "wide", "logo", "title-banner", "title-logo", "title-wide", "person"} {
		p := filepath.Join(dir, "img", k, "ab")
		os.MkdirAll(p, 0o755)
		os.WriteFile(filepath.Join(p, "x.webp"), []byte("12345"), 0o644)
	}
	return dir
}

func count(t *testing.T, dir, file, query string) int {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(dir, file))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow(query).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAllowListIsExactlyMusicKinds(t *testing.T) {
	want := map[string]bool{imgstore.KindArtist: true, imgstore.KindAlbum: true, imgstore.KindWide: true, imgstore.KindLogo: true}
	if len(musicKinds) != 4 {
		t.Fatal(musicKinds)
	}
	for _, k := range musicKinds {
		if !want[k] {
			t.Fatalf("unexpected kind %q", k)
		}
	}
	for _, k := range []string{imgstore.KindTitleBanner, imgstore.KindTitleLogo, imgstore.KindTitleWide, imgstore.KindPerson, "title_logo", ""} {
		if isMusicKind(k) {
			t.Fatalf("%q must not be allowed", k)
		}
	}
	for _, tg := range targets() {
		for _, a := range tg.Args {
			s, _ := a.(string)
			if strings.Contains(s, "title") || strings.Contains(s, "person") {
				t.Fatalf("target %s/%s args touch a TV kind: %v", tg.File, tg.Table, a)
			}
		}
	}
}

func TestDryRunChangesNothing(t *testing.T) {
	dir := mk(t)
	p, err := plan(dir, 3)
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	p.print(&out, false)
	after, err := kept(dir)
	if err != nil {
		t.Fatal(err)
	}
	if ok, diff := p.Kept.equal(after); !ok {
		t.Fatal(diff)
	}
	if n := count(t, dir, "img/images.sqlite", `SELECT COUNT(*) FROM images`); n != 8 {
		t.Fatalf("images=%d", n)
	}
	for _, m := range p.Measures {
		if m.T.Table == "images" && (m.Rows != 4 || m.Bytes != 36) {
			t.Fatalf("images measure %+v", m)
		}
		if m.T.File == "fetchq-images.sqlite" && m.Rows != 2 {
			t.Fatalf("queue rows %d", m.Rows)
		}
	}
	if !strings.Contains(out.String(), "music.sqlite/extra") {
		t.Fatalf("unlisted table not reported:\n%s", out.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "img", "artist", "ab", "x.webp")); err != nil {
		t.Fatal("dry run removed a file")
	}
}

func TestApplyTouchesOnlyMusic(t *testing.T) {
	dir := mk(t)
	if err := apply(dir); err != nil {
		t.Fatal(err)
	}
	if n := count(t, dir, "img/images.sqlite", `SELECT COUNT(*) FROM images WHERE kind IN ('artist','album','wide','logo')`); n != 0 {
		t.Fatalf("music images left %d", n)
	}
	if n := count(t, dir, "img/images.sqlite", `SELECT COUNT(*) FROM images WHERE kind IN ('title-banner','title-logo','title-wide','person')`); n != 4 {
		t.Fatalf("TV images %d", n)
	}
	if n := count(t, dir, "img/images.sqlite", `SELECT COUNT(*) FROM misses WHERE kind='person'`); n != 1 {
		t.Fatalf("person misses %d", n)
	}
	if n := count(t, dir, "img/images.sqlite", `SELECT COUNT(*) FROM misses WHERE kind='title-wide'`); n != 1 {
		t.Fatalf("tv misses %d", n)
	}
	if n := count(t, dir, "img/images.sqlite", `SELECT COUNT(*) FROM source_ids`); n != 2 {
		t.Fatalf("source_ids %d", n)
	}
	if n := count(t, dir, "fetchq-images.sqlite", `SELECT COUNT(*) FROM fetch_jobs`); n != 2 {
		t.Fatalf("queue %d", n)
	}
	if n := count(t, dir, "music.sqlite", `SELECT COUNT(*) FROM chart_days`); n != 0 {
		t.Fatal("chart_days not wiped")
	}
	if n := count(t, dir, "music.sqlite", `SELECT COUNT(*) FROM extra`); n != 1 {
		t.Fatal("unlisted table touched")
	}
	if n := count(t, dir, "cache.sqlite", `SELECT COUNT(*) FROM entries`); n != 2 {
		t.Fatal("movie/TV cache touched")
	}
	for _, k := range musicKinds {
		if _, err := os.Stat(filepath.Join(dir, "img", k)); err == nil {
			t.Fatalf("%s dir left", k)
		}
	}
	for _, k := range []string{"title-banner", "title-logo", "title-wide", "person"} {
		if _, err := os.Stat(filepath.Join(dir, "img", k, "ab", "x.webp")); err != nil {
			t.Fatalf("%s files removed", k)
		}
	}
}
