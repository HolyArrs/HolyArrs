package database

import (
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"
)

func TestLoaderRejectsSessionStatements(t *testing.T) {
	for _, sql := range []string{
		"PRAGMA foreign_keys=OFF;",
		"CREATE TABLE a(x);\nVACUUM;",
		"BEGIN;\nCREATE TABLE a(x);\nCOMMIT;",
		"BEGIN TRANSACTION; CREATE TABLE a(x);",
		"CREATE TABLE a(x); commit;",
	} {
		if _, err := loadMigrations(fstest.MapFS{"0001_a.sql": {Data: []byte(sql)}}); err == nil {
			t.Errorf("accepted %q", sql)
		}
	}
	ok := "-- PRAGMA in a comment is fine\nCREATE TABLE a(x);\nCREATE TRIGGER t AFTER INSERT ON a\nBEGIN\n  UPDATE a SET x = 1;\nEND;"
	if _, err := loadMigrations(fstest.MapFS{"0001_a.sql": {Data: []byte(ok)}}); err != nil {
		t.Fatalf("trigger refused: %v", err)
	}
	for _, sub := range []string{"migrations/main", "migrations/events"} {
		src, _ := fs.Sub(embedded, sub)
		if _, err := loadMigrations(src); err != nil {
			t.Fatalf("%s: %v", sub, err)
		}
	}
}

func TestMigrationBreakingForeignKeysIsUndone(t *testing.T) {
	dir := t.TempDir()
	base := fstest.MapFS{"0001_a.sql": {Data: []byte("CREATE TABLE p(id INTEGER PRIMARY KEY); CREATE TABLE c(pid INTEGER REFERENCES p(id));")}}
	s, err := openAt(t, dir, base)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	bad := fstest.MapFS{
		"0001_a.sql": base["0001_a.sql"],
		"0002_b.sql": {Data: []byte("INSERT INTO c(pid) VALUES (42);")},
	}
	if _, err := Open(dir, Options{MainMigrations: bad}); !errors.Is(err, ErrMigration) {
		t.Fatalf("got %v, want ErrMigration", err)
	}
	s, err = openAt(t, dir, base)
	if err != nil {
		t.Fatal(err)
	}
	var fk int
	if err := s.main.db.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil || fk != 1 {
		t.Fatalf("foreign_keys after migrate = %d %v", fk, err)
	}
	if v, _ := s.Version(); v != 1 {
		t.Fatalf("version %d", v)
	}
}

func TestHeadroomScalesWithSize(t *testing.T) {
	if got := headroom(10 << 20); got != 50<<20 {
		t.Fatalf("small db headroom %d", got)
	}
	if got := headroom(10 << 30); got != 1<<30 {
		t.Fatalf("10 GB db headroom %d", got)
	}
}

func TestForeignKeysOnAfterMigrations(t *testing.T) {
	dir := t.TempDir()
	s, err := openAt(t, dir, v1)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	s, err = openAt(t, dir, v2)
	if err != nil {
		t.Fatal(err)
	}
	var fk int
	var sync int
	if err := s.main.db.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil || fk != 1 {
		t.Fatalf("foreign_keys = %d %v", fk, err)
	}
	if err := s.main.db.QueryRow("PRAGMA synchronous").Scan(&sync); err != nil || sync != 1 {
		t.Fatalf("synchronous = %d %v, want NORMAL", sync, err)
	}
}
