package backups

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
