package importer

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestShowVersionsImportSideBySide(t *testing.T) {
	w := showSetup(t, []int{1, 2}, ep(1), ep(2))
	ctx := context.Background()
	im := &Importer{Store: w.s}

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
		var main mediafiles.Version
		if err := database.Get(ctx, tx, "versions", w.vID, &main); err != nil {
			return err
		}
		v := &mediafiles.Version{TitleID: main.TitleID, Label: "4K", PickRulesID: main.PickRulesID, LibraryFolderID: main.LibraryFolderID,
			FolderName: w.folder + " [4K]", KeepUpToDate: true}
		if _, err := database.Insert(tx, "versions", v, domain.System); err != nil {
			return err
		}
		uhdVer = v.ID
		for n := 1; n <= 4; n++ {
			var hd mediafiles.Slot
			if err := database.Get(ctx, tx, "slots", w.slots[n], &hd); err != nil {
				return err
			}
			state := mediafiles.SlotWanted
			if n <= 2 {
				state = mediafiles.SlotDownloading
			}
			sl := &mediafiles.Slot{ItemID: hd.ItemID, VersionID: v.ID, State: state}
			if _, err := database.Insert(tx, "slots", sl, domain.System); err != nil {
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
		if _, err := database.Insert(tx, "releases", r, domain.System); err != nil {
			return err
		}
		d := &domain.Download{DownloaderID: dlr, ReleaseID: &r.ID, SlotIDs: ids, ExternalID: "uhd", Progress: 1, State: "finished",
			FilesSeen: seen, SavePath: uhdDir, StartedBy: domain.System}
		if _, err := database.Insert(tx, "downloads", d, domain.System); err != nil {
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
	for n, id := range uhdSlots {
		var sl mediafiles.Slot
		_ = database.Get(ctx, w.s.Read(), "slots", id, &sl)
		if sl.FileID != nil || (n <= 2 && sl.State != mediafiles.SlotDownloading) || (n > 2 && sl.State != mediafiles.SlotWanted) {
			t.Fatalf("4K E%d touched by the HD pack: %+v", n, sl)
		}
	}
	if imp, err := im.Import(ctx, uhdDl, false, domain.System); err != nil || imp.State != "done" {
		t.Fatalf("4K import %+v %v", imp, err)
	}
	for _, n := range []int{1, 2} {
		hd := w.file(t, *w.slot(t, n).FileID)
		var usl mediafiles.Slot
		_ = database.Get(ctx, w.s.Read(), "slots", uhdSlots[n], &usl)
		if usl.FileID == nil {
			t.Fatalf("4K E%d not placed: %+v", n, usl)
		}
		uhd := w.file(t, *usl.FileID)
		if hd.State != "present" || uhd.State != "present" || hd.VersionID != w.vID || uhd.VersionID != uhdVer ||
			!strings.HasPrefix(hd.RelativePath, w.folder+"/") || !strings.HasPrefix(uhd.RelativePath, w.folder+" [4K]/") {
			t.Fatalf("E%d files HD %+v 4K %+v", n, hd, uhd)
		}
		for _, f := range []mediafiles.File{hd, uhd} {
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
