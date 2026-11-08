package migrate

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestBackupReadHardening: links in db/ and config/, config.xml, one read
// at a time.
func TestBackupReadHardening(t *testing.T) {
	if DefaultLimits.MaxFile > 1<<30 {
		t.Fatalf("copy cap %d", DefaultLimits.MaxFile)
	}
	root := t.TempDir()
	db := dbBytes(t, radarrDB()...)
	if err := os.WriteFile(filepath.Join(root, "radarr.db"), db, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config.xml"), []byte("<Config><ApiKey>XMLSECRET</ApiKey></Config>"), 0o600); err != nil {
		t.Fatal(err)
	}
	b, err := openBackup(Source{Kind: Radarr, Path: root}, arrReader{kind: Radarr}.backupFiles())
	if err != nil {
		t.Fatal(err)
	}
	if b.has("config.xml") {
		t.Error("config.xml (the arr key) was copied")
	}
	if _, err := openBackup(Source{Kind: Radarr, Path: root}, []string{"radarr.db"}); !errors.Is(err, ErrBackupBusy) {
		t.Errorf("second read at once: %v", err)
	}
	b.close()
	b2, err := openBackup(Source{Kind: Radarr, Path: root}, []string{"radarr.db"})
	if err != nil {
		t.Fatalf("after the first closed: %v", err)
	}
	b2.close()

	// A db/ sub-folder that is a link refuses the backup.
	other := t.TempDir()
	if err := os.WriteFile(filepath.Join(other, "db.sqlite3"), db, 0o600); err != nil {
		t.Fatal(err)
	}
	seerrRoot := t.TempDir()
	if err := os.Symlink(other, filepath.Join(seerrRoot, "db")); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}
	if _, err := openBackup(Source{Kind: Seerr, Path: seerrRoot}, seerrFiles); !errors.Is(err, ErrUnsafeBackup) {
		t.Fatalf("linked db/ folder: %v", err)
	}
}
