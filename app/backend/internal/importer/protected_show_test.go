package importer

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestProtectedMultiEpisodeCopyCannotBeReplaced(t *testing.T) {
	w := showSetup(t, []int{1, 2}, "Copper.Valley.S01E01E02.1080p.WEB.H264-HOLYARRS.mkv")
	ctx := context.Background()
	oldName := "Copper.Valley.S01E01E02.720p.HDTV.H264-OLD"
	rel := "Copper Valley (2025)/Season 1/" + oldName + ".mkv"
	oldPath := filepath.Join(w.lib, filepath.FromSlash(rel))
	write(t, oldPath, 4096)
	before, err := os.ReadFile(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	var oldID int64
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		f := &mediafiles.File{Protected: true, SlotIDs: []int64{w.slots[1], w.slots[2]}, VersionID: w.vID, RelativePath: rel, Size: int64(len(before)), AddedAt: time.Now().Add(-time.Hour), Languages: []string{}, MediaInfo: map[string]any{}, ReleaseName: &oldName, Origin: "imported", PlacedBy: "hardlink", Role: "main", State: "present"}
		id, err := database.Insert(tx, "files", f, domain.System)
		oldID = id
		if err != nil {
			return err
		}
		_, err = tx.Exec("UPDATE slots SET file_id=? WHERE id IN (?,?)", oldID, w.slots[1], w.slots[2])
		return err
	}); err != nil {
		t.Fatal(err)
	}

	im := &Importer{Store: w.s}
	preview, err := im.Import(ctx, w.dlID, true, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	if hasMain(preview.Plan) {
		t.Fatalf("preview would replace protected pack: %+v", preview.Plan)
	}
	imp, err := im.Import(ctx, w.dlID, false, domain.System)
	if err != nil || imp.State != "failed" {
		t.Fatalf("import %+v err %v", imp, err)
	}
	after, err := os.ReadFile(oldPath)
	if err != nil || string(after) != string(before) {
		t.Fatalf("protected pack bytes changed: %v", err)
	}
	for _, episode := range []int{1, 2} {
		sl := w.slot(t, episode)
		if sl.FileID == nil || *sl.FileID != oldID || sl.State != mediafiles.SlotNeedsADecision || sl.ActiveDownloadID == nil || *sl.ActiveDownloadID != w.dlID {
			t.Fatalf("episode %d slot %+v", episode, sl)
		}
	}
	if release := w.release(t); release.State == "turned_down" {
		t.Fatalf("protected copy globally turned down Release: %+v", release)
	}
}
