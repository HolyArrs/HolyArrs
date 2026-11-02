package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPermanentSnapshot(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "img"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"img/images.sqlite", "registered.sqlite", "cache.sqlite"} {
		db, err := sql.Open("sqlite", filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`PRAGMA journal_mode=WAL; CREATE TABLE t(v TEXT); INSERT INTO t VALUES('kept')`); err != nil {
			t.Fatal(err)
		}
		defer db.Close() // still open, as the running server's are
	}
	os.WriteFile(filepath.Join(dir, "control.json"), []byte(`{"providers":{}}`), 0o644)
	out := filepath.Join(t.TempDir(), "snap.tar.gz")
	n, err := writeSnapshot(context.Background(), dir, out)
	if err != nil || n != 3 {
		t.Fatalf("files %d err %v", n, err)
	}
	if _, err := os.Stat(out + ".part"); !os.IsNotExist(err) {
		t.Fatalf(".part left behind: %v", err)
	}
	f, _ := os.Open(out)
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	got := map[string]bool{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		got[h.Name] = true
		if h.Name == "registered.sqlite" {
			p := filepath.Join(t.TempDir(), "r.sqlite")
			b, _ := io.ReadAll(tr)
			os.WriteFile(p, b, 0o600)
			db, _ := sql.Open("sqlite", p)
			var v string
			if err := db.QueryRow(`SELECT v FROM t`).Scan(&v); err != nil || v != "kept" {
				t.Fatalf("copy unreadable: %v %q", err, v)
			}
			db.Close()
		}
	}
	if !got["img/images.sqlite"] || !got["registered.sqlite"] || !got["control.json"] || got["cache.sqlite"] {
		t.Fatalf("snapshot holds %v (the cache never)", got)
	}
}

func TestPruneKeepsNewest(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"permanent-20261024T000000.tar.gz", "permanent-20261025T000000.tar.gz", "permanent-20261026T000000.tar.gz", "permanent-20261027T000000.tar.gz"} {
		os.WriteFile(filepath.Join(dir, n), nil, 0o600)
	}
	prune(dir, 3)
	if m, _ := filepath.Glob(filepath.Join(dir, "permanent-*")); len(m) != 3 || filepath.Base(m[0]) != "permanent-20261025T000000.tar.gz" {
		t.Fatalf("kept %v", m)
	}
}

func TestCleanStaleKeepsFreshAndSnapshots(t *testing.T) {
	out := t.TempDir()
	old := time.Now().Add(-2 * time.Hour)
	for _, n := range []string{"a.tar.gz.part", ".snap-1", "permanent-x.tar.gz", "b.tar.gz.part"} {
		p := filepath.Join(out, n)
		os.WriteFile(p, nil, 0o600)
		if n != "b.tar.gz.part" {
			os.Chtimes(p, old, old)
		}
	}
	cleanStale(out, time.Now().Add(-time.Hour))
	for n, want := range map[string]bool{"a.tar.gz.part": false, ".snap-1": false, "permanent-x.tar.gz": true, "b.tar.gz.part": true} {
		if _, err := os.Stat(filepath.Join(out, n)); (err == nil) != want {
			t.Errorf("%s kept=%v", n, err == nil)
		}
	}
}
