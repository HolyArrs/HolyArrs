package logs_test

import (
	"context"
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/logs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

func onDisk(dir string) string {
	a, _ := os.ReadFile(filepath.Join(dir, logs.FileName))
	b, _ := os.ReadFile(filepath.Join(dir, logs.OldName))
	return string(a) + string(b)
}

// The vault tells the log of a new secret before the save returns, and a
// changed or removed secret (by Set or by a direct DELETE in any package)
// stays masked in the files across a restart.
func TestVaultRotationNeverShowsOldValue(t *testing.T) {
	ctx := context.Background()
	cfg := t.TempDir()
	s, err := database.Open(cfg, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	key := make([]byte, secrets.KeySize)
	v, err := secrets.Open(ctx, s, key)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(cfg, "logs")
	l, err := logs.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	v.OnPut = l.AddSecret
	lg := log.New(l, "", log.LstdFlags)

	id, err := v.Put(ctx, "test", "first-value-aaaa1")
	if err != nil {
		t.Fatal(err)
	}
	// Logged right away, before any refresh loop.
	lg.Printf("site said first-value-aaaa1")
	if strings.Contains(onDisk(dir), "first-value-aaaa1") {
		t.Fatal("new secret reached the file unmasked")
	}
	// A line from before the masker knew it (an older version wrote it).
	_ = l.Close()
	f, _ := os.OpenFile(filepath.Join(dir, logs.FileName), os.O_APPEND|os.O_WRONLY, 0o600)
	_, _ = f.WriteString("2026/10/14 10:00:00 old first-value-aaaa1 and gone-value-bbbb2\n")
	_ = f.Close()
	l, _ = logs.Open(dir)
	v.OnPut = l.AddSecret

	gone, err := v.Put(ctx, "test", "gone-value-bbbb2")
	if err != nil {
		t.Fatal(err)
	}
	if err := v.Set(ctx, id, "second-value-cccc3"); err != nil {
		t.Fatal(err)
	}
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("DELETE FROM secrets WHERE id = ?", gone)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	_ = l.Close()

	// Restart: a fresh log that never saw the old values.
	l2, err := logs.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer l2.Close()
	if err := l2.Sync(ctx, v); err != nil {
		t.Fatal(err)
	}
	for _, old := range []string{"first-value-aaaa1", "gone-value-bbbb2"} {
		if strings.Contains(onDisk(dir), old) {
			t.Fatalf("%s on disk", old)
		}
		if strings.Contains(string(l2.Tail(1<<20)), old) {
			t.Fatalf("%s in download", old)
		}
		got, _ := l2.Read(logs.Filter{})
		for _, ln := range got {
			if strings.Contains(ln.Text, old) {
				t.Fatalf("%s in viewer", old)
			}
		}
	}
	if left, upTo, err := v.RetiredValues(ctx); err != nil || len(left) != 0 || upTo != 0 {
		t.Fatalf("retired rows not forgotten: %d %d %v", len(left), upTo, err)
	}
}
