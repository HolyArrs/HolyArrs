package sources

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
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// TestChooseByHand_GoesToTheVersionItFits checks that a 1080p Release picked by hand for a movie with a 4K main Version and a 1080p Version goes to the 1080p one,
// imports there, and leaves the 4K copy in place.
func TestChooseByHand_GoesToTheVersionItFits(t *testing.T) {
	f := seed(t, "http://127.0.0.1:1/api")
	ctx := context.Background()
	const name = "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS"
	const old = "Dune.Part.Two.2024.2160p.UHD.BluRay.x265-HOLYARRS"
	var lib, fourKPath string
	var oldID, slot2, relID int64
	if err := f.s.Write(ctx, func(tx *sql.Tx) error {
		var main mediafiles.Version
		if err := tx.QueryRow("SELECT v.id, v.title_id, v.library_folder_id, v.folder_name, v.pick_rules_id FROM versions v JOIN slots s ON s.version_id = v.id WHERE s.id = ?", f.slotID).
			Scan(&main.ID, &main.TitleID, &main.LibraryFolderID, &main.FolderName, &main.PickRulesID); err != nil {
			return err
		}
		if err := tx.QueryRow("SELECT path FROM library_folders WHERE id = ?", main.LibraryFolderID).Scan(&lib); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE versions SET label = '4K', pick_rules_id = (SELECT id FROM pick_rules WHERE name = ?) WHERE id = ?", mediafiles.FourKName, main.ID); err != nil {
			return err
		}
		rp := main.FolderName + "/" + main.FolderName + " Bluray-2160p.mkv"
		fourKPath = filepath.Join(lib, filepath.FromSlash(rp))
		if err := os.MkdirAll(filepath.Dir(fourKPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(fourKPath, make([]byte, 100), 0o644); err != nil {
			return err
		}
		oldName := old
		fl := &mediafiles.File{SlotIDs: []int64{f.slotID}, VersionID: main.ID, RelativePath: rp, Size: 100, AddedAt: time.Now().Add(-time.Hour),
			Languages: []string{}, MediaInfo: map[string]any{}, ReleaseName: &oldName, Origin: "imported", PlacedBy: "hardlink", Role: "main", State: "present"}
		var err error
		if oldID, err = database.Insert(tx, "files", fl, domain.System); err != nil {
			return err
		}
		if _, err := tx.Exec("UPDATE slots SET file_id = ?, state = 'in_library' WHERE id = ?", oldID, f.slotID); err != nil {
			return err
		}
		v := &mediafiles.Version{TitleID: main.TitleID, Label: "1080p", PickRulesID: main.PickRulesID, LibraryFolderID: main.LibraryFolderID,
			FolderName: main.FolderName + " [1080p]", KeepUpToDate: true, AudioLanguages: []string{}}
		if _, err := database.Insert(tx, "versions", v, domain.System); err != nil {
			return err
		}
		var itemID int64
		if err := tx.QueryRow("SELECT item_id FROM slots WHERE id = ?", f.slotID).Scan(&itemID); err != nil {
			return err
		}
		sl := &mediafiles.Slot{ItemID: itemID, VersionID: v.ID, State: mediafiles.SlotWanted}
		if slot2, err = database.Insert(tx, "slots", sl, domain.System); err != nil {
			return err
		}
		// The row the movie page shows: found by the main Version's search, refused by its 4K rules.
		r := &domain.Release{SourceID: f.srcID, Name: name, LinkRef: "http://x/1.torrent", Protocol: "torrent", Flags: []string{}, Parsed: map[string]any{},
			MatchedSlotIDs: []int64{f.slotID}, FoundBy: "hand_search", State: "refused",
			Verdict: map[string]any{"outcome": "refused", "passed": false, "reasons": []any{map[string]any{"step": "quality", "rule": "step_not_allowed", "effect": "fail", "detail": "1080p web is not allowed"}}}}
		relID, err = database.Insert(tx, "releases", r, domain.System)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	ch, err := (&Searcher{Store: f.s}).Choose(ctx, relID, 0, true, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if ch.SlotID != slot2 {
		t.Fatalf("the 1080p Release went to Slot %d, want the 1080p Version's Slot %d", ch.SlotID, slot2)
	}

	// Download it for that Slot and import it.
	dl := t.TempDir()
	if err := os.WriteFile(filepath.Join(dl, name+".mkv"), make([]byte, 2048), 0o644); err != nil {
		t.Fatal(err)
	}
	var dlID int64
	if err := f.s.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('what_to_pick', 'import_size_floors', 'false', '2026-01-01T00:00:00Z', '{}', '2026-01-01T00:00:00Z', '{}')
			ON CONFLICT (grp, key) DO UPDATE SET value = excluded.value`); err != nil {
			return err
		}
		d := &domain.Downloader{Name: "q", Adapter: "qbittorrent", Protocol: "torrent", URL: "http://q", Category: "holyarrs", PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok"}
		if _, err := database.Insert(tx, "downloaders", d, domain.System); err != nil {
			return err
		}
		x := &domain.Download{DownloaderID: d.ID, ReleaseID: &relID, SlotIDs: []int64{ch.SlotID}, ExternalID: "abc", Progress: 1, State: "finished",
			FilesSeen: []domain.SeenFile{{Name: name + ".mkv", Size: 2048}}, SavePath: dl, StartedBy: domain.System}
		var err error
		if dlID, err = database.Insert(tx, "downloads", x, domain.System); err != nil {
			return err
		}
		_, err = tx.Exec("UPDATE slots SET state = 'downloading', active_download_id = ? WHERE id = ?", dlID, ch.SlotID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	imp, err := (&importer.Importer{Store: f.s}).Import(ctx, dlID, false, domain.System)
	if err != nil || imp.State != "done" {
		t.Fatalf("import %v %+v", err, imp)
	}
	if _, err := os.Stat(fourKPath); err != nil {
		t.Fatalf("the 4K file is gone: %v", err)
	}
	var fl mediafiles.File
	if err := database.Get(ctx, f.s.Read(), "files", oldID, &fl); err != nil || fl.State != "present" {
		t.Fatalf("4K file row %s %v", fl.State, err)
	}
	var s2 mediafiles.Slot
	if err := database.Get(ctx, f.s.Read(), "slots", slot2, &s2); err != nil || s2.FileID == nil {
		t.Fatalf("the 1080p Version has no file: %+v %v", s2, err)
	}
}
