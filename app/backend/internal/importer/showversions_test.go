package importer

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/store"
)

// 4.8.11: a show with an HD and a 4K Version in the same Library folder.
// The HD pack fills only HD Slots, the 4K pack only 4K Slots; each copy is
// placed in its own Version's folder and neither replaces the other.
func TestShowVersionsImportSideBySide(t *testing.T) {
	w := showSetup(t, []int{1, 2}, ep(1), ep(2))
	ctx := context.Background()
	im := &Importer{Store: w.s}

	// The 4K Version: its own Slots, its folder name carries the label.
	const uhdPack = "Copper.Valley.S01.2160p.WEB.H265-HOLYARRS"
	uhdDir := t.TempDir()
	uhdSlots := map[int]int64{}
	var uhdVer, uhdDl int64
	var seen []domain.SeenFile
	for _, n := range []int{1, 2} {
		name := strings.Replace(ep(n), "1080p.WEB.H264", "2160p.WEB.H265", 1)
		write(t, filepath.Join(uhdDir, uhdPack, name), 4096)
		seen = append(seen, domain.SeenFile{Name: uhdPack + "/" + name, Size: 4096})
	}
	err := w.s.Write(ctx, func(tx *sql.Tx) error {
		var main domain.Version
		if err := store.Get(ctx, tx, "versions", w.vID, &main); err != nil {
			return err
		}
		v := &domain.Version{TitleID: main.TitleID, Label: "4K", PickRulesID: main.PickRulesID, LibraryFolderID: main.LibraryFolderID,
			FolderName: w.folder + " [4K]", KeepUpToDate: true}
		if _, err := store.Insert(tx, "versions", v, domain.System); err != nil {
			return err
		}
		uhdVer = v.ID
		for n := 1; n <= 4; n++ {
			var hd domain.Slot
			if err := store.Get(ctx, tx, "slots", w.slots[n], &hd); err != nil {
				return err
			}
			state := domain.SlotWanted
			if n <= 2 {
				state = domain.SlotDownloading
			}
			sl := &domain.Slot{ItemID: hd.ItemID, VersionID: v.ID, State: state}
			if _, err := store.Insert(tx, "slots", sl, domain.System); err != nil {
				return err
			}
			uhdSlots[n] = sl.ID
		}
		var src, dlr int64
		_ = tx.QueryRow("SELECT id FROM sources LIMIT 1").Scan(&src)
		_ = tx.QueryRow("SELECT id FROM downloaders LIMIT 1").Scan(&dlr)
		ids := []int64{uhdSlots[1], uhdSlots[2]}
		r := &domain.Release{SourceID: src, Name: uhdPack, LinkRef: "http://x/2.torrent", Protocol: "torrent", Flags: []string{}, Parsed: map[string]any{},
			MatchedSlotIDs: ids, Verdict: map[string]any{}, FoundBy: "auto_search", State: "chosen"}
		if _, err := store.Insert(tx, "releases", r, domain.System); err != nil {
			return err
		}
		d := &domain.Download{DownloaderID: dlr, ReleaseID: &r.ID, SlotIDs: ids, ExternalID: "uhd", Progress: 1, State: "finished",
			FilesSeen: seen, SavePath: uhdDir, StartedBy: domain.System}
		if _, err := store.Insert(tx, "downloads", d, domain.System); err != nil {
			return err
		}
		uhdDl = d.ID
		_, err := tx.Exec("UPDATE slots SET active_download_id = ? WHERE id IN (?, ?)", d.ID, ids[0], ids[1])
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	if imp, err := im.Import(ctx, w.dlID, false, domain.System); err != nil || imp.State != "done" {
		t.Fatalf("HD import %+v %v", imp, err)
	}
	// The HD pack left every 4K Slot alone.
	for n, id := range uhdSlots {
		var sl domain.Slot
		_ = store.Get(ctx, w.s.Read(), "slots", id, &sl)
		if sl.FileID != nil || (n <= 2 && sl.State != domain.SlotDownloading) || (n > 2 && sl.State != domain.SlotWanted) {
			t.Fatalf("4K E%d touched by the HD pack: %+v", n, sl)
		}
	}
	if imp, err := im.Import(ctx, uhdDl, false, domain.System); err != nil || imp.State != "done" {
		t.Fatalf("4K import %+v %v", imp, err)
	}
	for _, n := range []int{1, 2} {
		hd := w.file(t, *w.slot(t, n).FileID)
		var usl domain.Slot
		_ = store.Get(ctx, w.s.Read(), "slots", uhdSlots[n], &usl)
		if usl.FileID == nil {
			t.Fatalf("4K E%d not placed: %+v", n, usl)
		}
		uhd := w.file(t, *usl.FileID)
		if hd.State != "present" || uhd.State != "present" || hd.VersionID != w.vID || uhd.VersionID != uhdVer ||
			!strings.HasPrefix(hd.RelativePath, w.folder+"/") || !strings.HasPrefix(uhd.RelativePath, w.folder+" [4K]/") {
			t.Fatalf("E%d files HD %+v 4K %+v", n, hd, uhd)
		}
		for _, f := range []domain.File{hd, uhd} {
			if _, err := os.Stat(filepath.Join(w.lib, filepath.FromSlash(f.RelativePath))); err != nil {
				t.Fatalf("E%d %s: %v", n, f.RelativePath, err)
			}
		}
	}
	var replaced int
	_ = w.s.Read().QueryRow("SELECT count(*) FROM files WHERE state = 'replaced'").Scan(&replaced)
	if replaced != 0 {
		t.Fatalf("%d files replaced", replaced)
	}
}
