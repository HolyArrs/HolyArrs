package library

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
)

// A Library file that is a link (placed from a streaming drive) is held as
// the link, listed in Recently deleted, and restored as the same link, even
// while the drive it points into is not mounted.
func TestRestoreHeldLink(t *testing.T) {
	svc, s, lf, id, fs := deleteWorld(t)
	ctx := context.Background()
	p := filepath.Join(lf.Path, filepath.FromSlash(fs[0].RelativePath))
	target := filepath.Join(t.TempDir(), "real.mkv")
	writeFile(t, target, "video")
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, p); err != nil {
		t.Skip("symlinks not available:", err)
	}
	if _, err := svc.Remove(ctx, id, RemoveChoice{DeleteFiles: true}, true, domain.System); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(target); err != nil { // the drive is gone for now
		t.Fatal(err)
	}
	held, err := svc.HeldFiles(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	listed := false
	for _, h := range held {
		listed = listed || h.ID == fs[0].ID && h.Path == fs[0].RelativePath
	}
	if n, _ := svc.HeldCount(ctx, 0); !listed || n != len(held) {
		t.Fatalf("held link not listed: %+v (count %d)", held, n)
	}
	res, _, err := svc.RestoreHeld(ctx, []int64{fs[0].ID}, "", domain.System)
	if err != nil || len(res) != 1 || res[0].Result != "restored" {
		t.Fatalf("restore %+v %v", res, err)
	}
	if got, err := os.Readlink(p); err != nil || got != target {
		t.Fatalf("want the link back at %s pointing to %s: %q %v", p, target, got, err)
	}
	if f := fileRow(t, s, fs[0].ID); f.State != "present" || f.RelativePath != fs[0].RelativePath {
		t.Fatalf("row %+v", f)
	}
}

// A held link goes once its keep time is up, like a held file: the link is
// removed and the file it points at is left alone.
func TestHoldingCleanUpRemovesHeldLink(t *testing.T) {
	svc, s, lf, _, fs := deleteWorld(t)
	ctx := context.Background()
	rules, err := importer.ReadFileRules(ctx, s.Read())
	if err != nil {
		t.Fatal(err)
	}
	day := now.AddDate(0, 0, -rules.HoldingDays).Truncate(24 * time.Hour)
	held := importer.HoldingDir + "/" + day.Format("2006-01-02") + "/Dune - Part Two (2024)/linked.mkv"
	at := database.FormatTime(now.AddDate(0, 0, -rules.HoldingDays).Add(-time.Hour))
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE files SET state = 'gone', relative_path = ?, updated_at = ? WHERE id = ?", held, at, fs[1].ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "real.mkv")
	writeFile(t, target, "video")
	p := filepath.Join(lf.Path, filepath.FromSlash(held))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, p); err != nil {
		t.Skip("symlinks not available:", err)
	}
	if n, err := svc.CleanHolding(ctx, domain.System); err != nil || n != 1 {
		t.Fatalf("cleaned %d %v", n, err)
	}
	if _, err := os.Lstat(p); !os.IsNotExist(err) {
		t.Fatal("the held link is still there")
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatal("the file the link pointed at was removed")
	}
}
