package importer

import (
	"context"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// A finished Usenet job whose folder HolyArrs may not read (no save folder
// and no path pair set up) has no file list: the decision says to set the
// Downloader's save folder, not to look at an empty import plan.
func TestShowImportWithNoSaveFolderSaysWhereToSetIt(t *testing.T) {
	w := showSetup(t, []int{1})
	if err := execSQL(w.s, "UPDATE downloaders SET name = 'SABnzbd', adapter = 'sabnzbd', protocol = 'usenet'"); err != nil {
		t.Fatal(err)
	}
	if _, err := (&Importer{Store: w.s}).Import(context.Background(), w.dlID, false, domain.System); err != nil {
		t.Fatal(err)
	}
	sl := w.slot(t, 1)
	if sl.State != mediafiles.SlotNeedsADecision || sl.LastReason == nil {
		t.Fatalf("slot %+v", sl)
	}
	why := *sl.LastReason
	// The save folder is HolyArrs's own setting, not one in the Downloader: the
	// line names both ways to let HolyArrs read the folder.
	if !strings.Contains(why, "SABnzbd has no path pair") || !strings.Contains(why, "HOLYARRS_SAVE_PATH") || !strings.Contains(why, "Settings > Downloaders") ||
		strings.Contains(why, "has no save folder") || strings.Contains(why, "import plan") {
		t.Fatalf("reason %q", why)
	}
}

// With a save folder set, an empty download is still the plain "nothing
// matched" decision: the folder was read and held nothing.
func TestShowImportWithSaveFolderKeepsNothingMatched(t *testing.T) {
	w := showSetup(t, []int{1})
	if err := execSQL(w.s, "UPDATE downloaders SET adapter = 'sabnzbd', protocol = 'usenet'"); err != nil {
		t.Fatal(err)
	}
	if _, err := (&Importer{Store: w.s, SavePath: w.dl}).Import(context.Background(), w.dlID, false, domain.System); err != nil {
		t.Fatal(err)
	}
	if sl := w.slot(t, 1); sl.LastReason == nil || !strings.Contains(*sl.LastReason, "could be matched to an episode") {
		t.Fatalf("slot %+v", sl)
	}
}

// The same for a movie: no file list and no save folder or path pair.
func TestMovieImportWithNoSaveFolderSaysWhereToSetIt(t *testing.T) {
	w := usenetWorld(t)
	ctx := context.Background()
	if err := execSQL(w.s, "UPDATE downloads SET files_seen = '[]'"); err != nil {
		t.Fatal(err)
	}
	if _, err := (&Importer{Store: w.s}).Import(ctx, w.dlID, false, domain.System); err != nil {
		t.Fatal(err)
	}
	var why string
	if err := w.s.Read().QueryRowContext(ctx, "SELECT last_reason FROM slots WHERE id = ?", w.slotID).Scan(&why); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(why, "HOLYARRS_SAVE_PATH") || !strings.Contains(why, "Settings > Downloaders") || strings.Contains(why, "import plan") {
		t.Fatalf("reason %q", why)
	}
	// A path pair is set up: the folder was read, so the plan is the answer.
	if err := execSQL(w.s, "UPDATE downloaders SET path_map = ?", `[{"theirs":"/downloads","ours":"`+strings.ReplaceAll(w.dl, `\`, `\\`)+`"}]`); err != nil {
		t.Fatal(err)
	}
	if err := execSQL(w.s, "UPDATE slots SET state = 'downloading' WHERE id = ?", w.slotID); err != nil {
		t.Fatal(err)
	}
	if _, err := (&Importer{Store: w.s}).Import(ctx, w.dlID, false, domain.System); err != nil {
		t.Fatal(err)
	}
	if err := w.s.Read().QueryRowContext(ctx, "SELECT last_reason FROM slots WHERE id = ?", w.slotID).Scan(&why); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(why, "import plan") {
		t.Fatalf("with a path pair, reason %q", why)
	}
}
