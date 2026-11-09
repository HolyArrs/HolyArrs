package backups

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

func TestOpenDirChecksKeyOnRestore(t *testing.T) {
	t.Setenv(secrets.EnvKeyFile, "")
	dir := t.TempDir()
	s, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	key, err := secrets.LoadKey(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := secrets.Open(context.Background(), s, key); err != nil {
		t.Fatal(err)
	}
	m := &Manager{Store: s, Dir: dir, Version: "1.0.0"}
	b, err := m.Create(context.Background(), KindManual)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	other := make([]byte, secrets.KeySize)
	_, _ = rand.Read(other)
	if err := os.WriteFile(filepath.Join(dir, secrets.KeyFile), []byte(base64.StdEncoding.EncodeToString(other)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	bm, unlock, err := OpenDir(dir, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if bm.CheckKey == nil {
		t.Fatal("CheckKey not set with a key file present")
	}
	if err := bm.StageNamed(context.Background(), b.Name); err == nil {
		t.Fatal("staged a backup made with a different key")
	}
	if Staged(dir) {
		t.Fatal("restore staged")
	}
	if _, _, err := OpenDir(dir, "1.0.0"); err == nil || !strings.Contains(err.Error(), "stop it first") {
		t.Fatalf("second OpenDir while locked: %v", err)
	}
}

func TestPutBackPicksNewestPreUpdateAndReportsKept(t *testing.T) {
	t.Setenv(secrets.EnvKeyFile, "")
	dir := t.TempDir()
	base := os.DirFS(filepath.Join("..", "core", "database", "migrations", "main"))
	start := func(v string, extra map[string]string) {
		t.Helper()
		mig := fstest.MapFS{}
		es, err := fs.ReadDir(base, ".")
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range es {
			b, _ := fs.ReadFile(base, e.Name())
			mig[e.Name()] = &fstest.MapFile{Data: b}
		}
		for n, q := range extra {
			mig[n] = &fstest.MapFile{Data: []byte(q)}
		}
		s, err := database.Open(dir, database.Options{MainMigrations: mig})
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		m := &Manager{Store: s, Dir: dir, Version: v}
		if _, err := m.AdoptMigrationCopies(); err != nil {
			t.Fatal(err)
		}
		if err := m.AfterStart(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	start("1.0.0", nil)
	start("1.1.0", map[string]string{"9998_one.sql": "CREATE TABLE one(x);"})
	start("1.2.0", map[string]string{"9998_one.sql": "CREATE TABLE one(x);", "9999_two.sql": "CREATE TABLE two(x);"})
	bm, unlock, err := OpenDir(dir, "1.2.0")
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	pre := 0
	all, _ := bm.List()
	for _, b := range all {
		if b.Kind == KindPreUpdate {
			pre++
		}
	}
	if pre < 2 {
		t.Fatalf("need two pre-update backups, have %d", pre)
	}
	newest, err := bm.NewestPreUpdate()
	if err != nil {
		t.Fatal(err)
	}
	name, kept, err := PutBack(bm, "")
	if err != nil {
		t.Fatal(err)
	}
	if name != newest.Name {
		t.Fatalf("restored %q, want the newest %q", name, newest.Name)
	}
	if !strings.HasPrefix(kept, filepath.Join(dir, "backups", "before-restore-")) {
		t.Fatalf("kept path %q", kept)
	}
	if _, err := os.Stat(kept); err != nil {
		t.Fatalf("kept folder missing: %v", err)
	}
	if Staged(dir) {
		t.Fatal("restore still staged after PutBack")
	}
}
