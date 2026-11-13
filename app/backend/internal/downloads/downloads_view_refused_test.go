package downloads

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// planImport records one Import with a plan; download 0 is a by-hand import
// of a folder, which no download backs.
func planImport(t *testing.T, x *dl41, download int64, mode, state, plan string) {
	t.Helper()
	var dl any
	if download > 0 {
		dl = download
	}
	exec41(t, x.s, `INSERT INTO imports(created_at, created_by, updated_at, updated_by, download_id, from_path, kind, mode, state, plan)
		VALUES ('t', '{}', 't', '{}', ?, '/data/downloads', 'movie', ?, ?, ?)`, dl, mode, state, plan)
}

// A refused import says why on its row (the importer's own words, not a
// sample's or another file's) and offers the download's folder to import
// by hand.
func TestDownloadsViewRefusedRowSaysWhyAndGivesTheFolder(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotSearching, "finished")
	save := t.TempDir()
	dir := filepath.Join(save, "Salt.Choir.2020.1080p-GRP")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	exec41(t, x.s, `UPDATE downloads SET save_path = ?, files_seen = ? WHERE id = ?`, save,
		`[{"name":"Salt.Choir.2020.1080p-GRP/salt.mkv","size":4096},{"name":"Salt.Choir.2020.1080p-GRP/sample.mkv","size":10}]`, x.dlID)
	lie := "the file is 50 MB, too small to be the full video; likely a fake"
	planImport(t, x, x.dlID, "auto", "rejected", fmt.Sprintf(`[
		{"source_file":"/d/sample.mkv","target_slot_ids":[],"method":"refuse","why":["a sample, not the full video"]},
		{"source_file":"/d/salt.mkv","target_slot_ids":[%d],"method":"refuse","why":["title matches","%s"]}]`, x.slotID, lie))
	r := ownRow(t, x)
	if r.Phase != "import_refused" || r.RefusedWhy != lie || r.ImportFolder != dir {
		t.Fatalf("phase %q why %q folder %q", r.Phase, r.RefusedWhy, r.ImportFolder)
	}
}

// Importing a file by hand into the download's Slot after a refusal ends
// the refusal: the row (and Home, which reads it) stops saying import
// refused. A by-hand import into another Slot changes nothing.
func TestDownloadsViewByHandImportEndsARefusal(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotInLibrary, "finished")
	planImport(t, x, x.dlID, "auto", "rejected", fmt.Sprintf(`[{"source_file":"/d/salt.mkv","target_slot_ids":[%d],"method":"refuse","why":["file is 720p, name said 1080p"]}]`, x.slotID))
	planImport(t, x, 0, "by_hand", "done", fmt.Sprintf(`[{"source_file":"/x/other.mkv","target_slot_ids":[%d],"method":"hardlink","why":["chosen by hand"]}]`, x.slotID+100))
	if r := ownRow(t, x); r.Phase != "import_refused" {
		t.Fatalf("another Slot's import: phase %q", r.Phase)
	}
	planImport(t, x, 0, "by_hand", "done", fmt.Sprintf(`[{"source_file":"/d/salt.mkv","target_slot_ids":[%d],"method":"hardlink","why":["chosen by hand"]}]`, x.slotID))
	if r := ownRow(t, x); r.Phase != "done" || r.Text != "In the library." || r.RefusedWhy != "" || r.ImportFolder != "" {
		t.Fatalf("after the by-hand import: phase %q text %q why %q folder %q", r.Phase, r.Text, r.RefusedWhy, r.ImportFolder)
	}
}
