package downloads

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// goneImport records one Import of the rig's download with a result.
func goneImport(t *testing.T, x *dl41, state, result string) {
	t.Helper()
	exec41(t, x.s, `INSERT INTO imports(created_at, created_by, updated_at, updated_by, download_id, from_path, kind, mode, state, result)
		VALUES ('t', '{}', 't', '{}', ?, '/data/downloads', 'movie', 'auto', ?, ?)`, x.dlID, state, result)
}

// A download whose files another app took before the import says so, seeding
// or finished, never "In the library."; one that did import still does.
func TestDownloadsViewFilesGoneIsNotInTheLibrary(t *testing.T) {
	t.Parallel()
	for _, state := range []string{"seeding", "finished"} {
		x := seed41(t, mediafiles.SlotWanted, state)
		goneImport(t, x, "failed", `[{"source_file":"/d/salt.mkv","outcome":"gone"},{"source_file":"/d/salt.nfo","outcome":"skipped"}]`)
		if r := ownRow(t, x); r.Text != FilesGoneText {
			t.Fatalf("%s: phase %q text %q", state, r.Phase, r.Text)
		}
		y := seed41(t, mediafiles.SlotInLibrary, state)
		goneImport(t, y, "done", `[{"source_file":"/d/salt.mkv","outcome":"placed"}]`)
		if r := ownRow(t, y); r.Phase != "done" || r.Text != "In the library." {
			t.Fatalf("%s imported: phase %q text %q", state, r.Phase, r.Text)
		}
	}
}

// A pack whose first episodes were placed by an earlier import is not called
// taken when a later import finds the rest gone.
func TestDownloadsViewPackPlacedEarlierNotTaken(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotWanted, "seeding")
	goneImport(t, x, "done", `[{"source_file":"/d/e01.mkv","outcome":"placed"}]`)
	goneImport(t, x, "failed", `[{"source_file":"/d/e06.mkv","outcome":"gone"}]`)
	if r := ownRow(t, x); r.Text == FilesGoneText {
		t.Fatalf("phase %q text %q", r.Phase, r.Text)
	}
}
