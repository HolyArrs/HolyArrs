package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func addRoutedDownloader(t *testing.T, s *database.Store, name, ours string, tags []string, state string, removed bool) {
	t.Helper()
	pm, _ := json.Marshal([]domain.PathMap{{Theirs: ours, Ours: ours}})
	tg, _ := json.Marshal(tags)
	err := s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO downloaders (created_at, created_by, updated_at, updated_by, name, adapter, protocol, url, category, priority, path_map, after_import, state, last_error, tags, removed)
			VALUES ('2026-10-01T00:00:00Z', '{}', '2026-10-01T00:00:00Z', '{}', ?, 'qbittorrent', 'torrent', 'http://qbit:8080', ?, 1, ?, 'leave', ?, '', ?, ?)`, name, name, string(pm), state, string(tg), removed)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// A folder whose titles the tag routing sends to one Downloader is
// tested only against that Downloader's folder (and the save path), so an
// unrelated Downloader on another disk does not mark it as copying. The
// untagged folder still tests the untagged Downloader, and a picked
// Downloader that truly cannot hardlink still warns.
func TestFolderChecksOnlyRoutedDownloaders(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	usbDL := t.TempDir()
	hddDL := filepath.Join(t.TempDir(), "other-disk") // cannot be linked from
	addRoutedDownloader(t, s, "hdd", hddDL, nil, "ok", false)
	addRoutedDownloader(t, s, "usb", usbDL, []string{"holy-usb"}, "ok", false)
	all, err := DownloadPaths(ctx, s.Read())
	if err != nil || len(all) != 2 {
		t.Fatalf("paths %v %v", all, err)
	}

	lib := filepath.Join(filepath.Dir(usbDL), "lib")
	if err := os.Mkdir(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	usb, err := AddFolder(ctx, s, FolderInput{Path: lib, Kind: "movie", NewTitleTags: []string{"holy-usb"}}, all, domain.System)
	if err != nil || usb.Health != "ok" {
		t.Fatalf("usb folder: %v %q %q", err, usb.Health, usb.HealthReason)
	}
	got, _, err := RecheckFolder(ctx, s, usb.ID, all, domain.System, false)
	if err != nil || got.Health != "ok" {
		t.Fatalf("usb recheck: %v %q %q", err, got.Health, got.HealthReason)
	}

	hdd, err := AddFolder(ctx, s, FolderInput{Path: t.TempDir(), Kind: "movie"}, all, domain.System)
	if err != nil || hdd.Health != "no_hardlink" {
		t.Fatalf("untagged folder must still test the untagged Downloader: %v %q", err, hdd.Health)
	}

	// A save path no Downloader owns is always tested.
	save := filepath.Join(t.TempDir(), "gone")
	if got, _, _ := RecheckFolder(ctx, s, usb.ID, append([]string{save}, all...), domain.System, false); got.Health != "no_hardlink" {
		t.Fatalf("an unknown save path must still warn: %q", got.Health)
	}

	// The picked Downloader itself cannot link: still a warning.
	bad := filepath.Join(t.TempDir(), "usb-gone")
	addRoutedDownloader(t, s, "usb2", bad, []string{"holy-usb"}, "ok", false)
	all, _ = DownloadPaths(ctx, s.Read())
	if got, _, _ := RecheckFolder(ctx, s, usb.ID, all, domain.System, false); got.Health != "no_hardlink" {
		t.Fatalf("a picked Downloader that cannot link must warn: %q", got.Health)
	}
}
