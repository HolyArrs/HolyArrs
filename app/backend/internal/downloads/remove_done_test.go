package downloads

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// diskEngine is a torrent client stand-in that deletes the content folder
// from disk when asked to remove with files, as a real client does.
type diskEngine struct {
	*seedEngine
	fail  error
	quiet bool // drops the torrent although its files could not be deleted
}

func (d *diskEngine) Remove(ctx context.Context, id string, files bool) error {
	if d.fail != nil {
		return d.fail
	}
	if files {
		for _, j := range d.jobs {
			if strings.EqualFold(j.ID, id) {
				if err := os.RemoveAll(j.ContentPath); err != nil && !d.quiet {
					return err
				}
			}
		}
	}
	return d.seedEngine.Remove(ctx, id, files)
}

// doneOnDisk is a seeding, imported download whose files are real: the
// download in root/downloads and the Library folder root/media/movies. link
// says how the Library copy was made: "hardlink", "copy" or "inplace" (the
// download sits inside the Library folder).
func doneOnDisk(t *testing.T, link string) (w *world, m *Manager, id int64, eng *diskEngine, dlFile, libFile string) {
	t.Helper()
	root := t.TempDir()
	lib := filepath.Join(root, "media", "movies")
	content := filepath.Join(root, "downloads", "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS")
	if link == "inplace" {
		content = filepath.Join(lib, "Dune Part Two (2024)")
	}
	dlFile = filepath.Join(content, "Dune.Part.Two.2024.mkv")
	libFile = filepath.Join(lib, "Dune Part Two (2024)", "Dune Part Two (2024).mkv")
	for _, d := range []string{filepath.Dir(dlFile), filepath.Dir(libFile)} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(dlFile, []byte("the movie"), 0o644); err != nil {
		t.Fatal(err)
	}
	switch link {
	case "hardlink":
		if err := os.Link(dlFile, libFile); err != nil {
			t.Skipf("no hard links here: %v", err)
		}
	case "copy":
		if err := os.WriteFile(libFile, []byte("the movie"), 0o644); err != nil {
			t.Fatal(err)
		}
	case "inplace":
		libFile = dlFile
	}
	w, m, id, se := seededDownload(t, "qbittorrent", `{}`, Job{Detail: "uploading", Ratio: 0.4, ContentPath: content})
	if err := execSQL(w, `UPDATE library_folders SET path = `+jsonStr(lib)); err != nil {
		t.Fatal(err)
	}
	if err := execSQL(w, `UPDATE downloads SET state = 'seeding'`); err != nil {
		t.Fatal(err)
	}
	eng = &diskEngine{seedEngine: se}
	m.Engine = func(domain.Downloader) (Engine, error) { return eng, nil }
	m.Forget(w.download(t, id).DownloaderID)
	return w, m, id, eng, dlFile, libFile
}

func stillThere(t *testing.T, path, why string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(b, []byte("the movie")) {
		t.Fatalf("%s: %s is gone or changed (%v)", why, path, err)
	}
}

func lastSummary(t *testing.T, w *world, kind string, id int64) string {
	t.Helper()
	var s string
	if err := w.s.ReadEvents().QueryRow("SELECT summary FROM events WHERE kind = ? AND download_id = ? ORDER BY id DESC LIMIT 1", kind, id).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

// A seeding, imported torrent goes on request; its Release is not turned down
// and nothing is searched again, whatever was asked, and History says so.
func TestRemoveDoneStopsSeedingAndKeepsTheImport(t *testing.T) {
	w, m, id, eng := seededDownload(t, "qbittorrent", `{}`, Job{Detail: "uploading", Ratio: 0.4})
	if err := m.Remove(context.Background(), id, RemoveChoice{TurnDown: true, SearchAgain: true}, luis, "luis"); err != nil {
		t.Fatal(err)
	}
	if len(eng.removed) != 1 || eng.removed[0] != hash+"|keep" {
		t.Fatalf("removed %v", eng.removed)
	}
	if d := w.download(t, id); d.State != "cleaned" || d.StateReason == nil || *d.StateReason != "removed by luis" {
		t.Fatalf("download %s %v", d.State, d.StateReason)
	}
	var rel string
	if err := w.s.Read().QueryRow("SELECT state FROM releases WHERE id = ?", w.relID).Scan(&rel); err != nil || rel == "turned_down" {
		t.Fatalf("release %s %v", rel, err)
	}
	if len(w.failedSlots) != 0 {
		t.Fatalf("searched again for %v", w.failedSlots)
	}
	if countEvents(t, w, "download_removed", id) != 1 || lastSummary(t, w, "download_removed", id) != "Download removed by luis" {
		t.Fatal("no History entry")
	}
}

// Seeding goal met or not, a person may remove it: the automatic remove waits
// for the goal, a hand remove does not (as the older apps do).
func TestRemoveDoneGoalMetOrNot(t *testing.T) {
	for _, j := range []Job{{Detail: "uploading", Ratio: 0.1}, {Detail: "stoppedUP", Ratio: 2}} {
		_, m, id, eng := seededDownload(t, "qbittorrent", `{"ratio":1}`, j)
		if err := m.Remove(context.Background(), id, RemoveChoice{}, luis, "luis"); err != nil || len(eng.removed) != 1 {
			t.Fatalf("%s: %v removed %v", j.Detail, err, eng.removed)
		}
	}
}

// A Library copy that is a hard link of the download: deleting the
// download's files leaves the Library file whole; keeping them keeps both.
func TestRemoveDoneHardLinkLibraryStays(t *testing.T) {
	_, m, id, eng, dlFile, libFile := doneOnDisk(t, "hardlink")
	if err := m.Remove(context.Background(), id, RemoveChoice{DeleteFiles: true}, luis, "luis"); err != nil {
		t.Fatal(err)
	}
	if len(eng.removed) != 1 || eng.removed[0] != hash+"|files" {
		t.Fatalf("removed %v", eng.removed)
	}
	if _, err := os.Stat(dlFile); !os.IsNotExist(err) {
		t.Fatalf("download file still there: %v", err)
	}
	stillThere(t, libFile, "hard link, delete files on")

	_, m, id, eng, dlFile, libFile = doneOnDisk(t, "hardlink")
	if err := m.Remove(context.Background(), id, RemoveChoice{}, luis, "luis"); err != nil || eng.removed[0] != hash+"|keep" {
		t.Fatalf("%v %v", err, eng.removed)
	}
	stillThere(t, dlFile, "hard link, delete files off")
	stillThere(t, libFile, "hard link, delete files off")
}

// A Library copy that was copied: the same, the Library file stays.
func TestRemoveDoneCopiedLibraryStays(t *testing.T) {
	_, m, id, _, dlFile, libFile := doneOnDisk(t, "copy")
	if err := m.Remove(context.Background(), id, RemoveChoice{DeleteFiles: true}, luis, "luis"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dlFile); !os.IsNotExist(err) {
		t.Fatalf("download file still there: %v", err)
	}
	stillThere(t, libFile, "copy, delete files on")
}

// The download is the Library copy (imported in place, or a Library folder
// inside the download folder): delete files is refused, History says why.
func TestRemoveDoneInPlaceNeverDeletesTheLibrary(t *testing.T) {
	w, m, id, eng, _, libFile := doneOnDisk(t, "inplace")
	if err := m.Remove(context.Background(), id, RemoveChoice{DeleteFiles: true}, luis, "luis"); err != nil {
		t.Fatal(err)
	}
	if len(eng.removed) != 1 || eng.removed[0] != hash+"|keep" {
		t.Fatalf("removed %v", eng.removed)
	}
	stillThere(t, libFile, "in place")
	if s := lastSummary(t, w, "download_removed", id); !strings.Contains(s, "its files were kept") || !strings.Contains(s, "Library folder") {
		t.Fatalf("History %q", s)
	}
	// The download folder holds the Library folder.
	w, m, id, se := seededDownload(t, "qbittorrent", `{}`, Job{Detail: "uploading", ContentPath: "/data/media"})
	if err := m.Remove(context.Background(), id, RemoveChoice{DeleteFiles: true}, luis, "luis"); err != nil || se.removed[0] != hash+"|keep" {
		t.Fatalf("%v %v", err, se.removed)
	}
	if !strings.Contains(lastSummary(t, w, "download_removed", id), "its files were kept") {
		t.Fatal("History does not say the files were kept")
	}
}

// Another torrent sharing the files keeps them too.
func TestRemoveDoneSharedFilesKept(t *testing.T) {
	_, m, id, eng := seededDownload(t, "qbittorrent", `{}`, Job{Detail: "uploading"})
	twin := eng.jobs[0]
	twin.ID, twin.Category = strings.Repeat("b", 40), "cross-seed"
	eng.jobs = append(eng.jobs, twin)
	if err := m.Remove(context.Background(), id, RemoveChoice{DeleteFiles: true}, luis, "luis"); err != nil || eng.removed[0] != hash+"|keep" {
		t.Fatalf("%v %v", err, eng.removed)
	}
}

// A finished Usenet job still listed: removing clears it, with its files when asked.
func TestRemoveDoneUsenet(t *testing.T) {
	w, m, id, eng := seededDownload(t, "sabnzbd", `{}`, Job{})
	if err := execSQL(w, `UPDATE downloaders SET protocol = 'usenet', after_import = 'leave'`); err != nil {
		t.Fatal(err)
	}
	m.Forget(w.download(t, id).DownloaderID)
	if err := m.Remove(context.Background(), id, RemoveChoice{DeleteFiles: true}, luis, "luis"); err != nil {
		t.Fatal(err)
	}
	if len(eng.removed) != 1 || eng.removed[0] != hash+"|files" {
		t.Fatalf("removed %v", eng.removed)
	}
}

// A season pack of which only some episodes went in: the ones in the
// library stay, the others keep their own state (looking again), and the
// import counts as settled so nothing is turned down.
func TestRemoveDonePartPack(t *testing.T) {
	w, m, id, eng := seededDownload(t, "qbittorrent", `{}`, Job{Detail: "uploading"})
	for _, q := range []string{
		`INSERT INTO items (title_id, kind, created_at, created_by, updated_at, updated_by) SELECT title_id, kind, created_at, created_by, updated_at, updated_by FROM items LIMIT 1`,
		`INSERT INTO slots (item_id, version_id, state, created_at, created_by, updated_at, updated_by)
			SELECT (SELECT MAX(id) FROM items), version_id, 'wanted', created_at, created_by, updated_at, updated_by FROM slots LIMIT 1`,
		`UPDATE slots SET state = 'in_library', active_download_id = NULL WHERE id = (SELECT MIN(id) FROM slots)`,
		`UPDATE downloads SET slot_ids = (SELECT json_group_array(id) FROM slots)`,
	} {
		if err := execSQL(w, q); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.Remove(context.Background(), id, RemoveChoice{TurnDown: true, SearchAgain: true}, luis, "luis"); err != nil || len(eng.removed) != 1 {
		t.Fatalf("%v %v", err, eng.removed)
	}
	rows, err := w.s.Read().Query("SELECT state FROM slots ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var s string
		_ = rows.Scan(&s)
		got = append(got, s)
	}
	if strings.Join(got, ",") != "in_library,wanted" || len(w.failedSlots) != 0 {
		t.Fatalf("slots %v, searched %v", got, w.failedSlots)
	}
}

// A finished download whose import waits for a person may be removed, and
// then Turn down is honoured (the Release was never placed).
func TestRemoveDoneNeedsDecisionMayTurnDown(t *testing.T) {
	w, m, id, _ := seededDownload(t, "qbittorrent", `{}`, Job{Detail: "uploading"})
	if err := execSQL(w, `UPDATE slots SET state = 'needs_a_decision'`); err != nil {
		t.Fatal(err)
	}
	if err := m.Remove(context.Background(), id, RemoveChoice{TurnDown: true}, luis, "luis"); err != nil {
		t.Fatal(err)
	}
	var rel string
	if err := w.s.Read().QueryRow("SELECT state FROM releases WHERE id = ?", w.relID).Scan(&rel); err != nil || rel != "turned_down" {
		t.Fatalf("release %s %v", rel, err)
	}
}

// While HolyArrs places the files, the remove waits.
func TestRemoveDoneRefusedWhileImporting(t *testing.T) {
	for _, q := range []string{`UPDATE imports SET state = 'placing'`, `UPDATE slots SET state = 'importing'`} {
		w, m, id, eng := seededDownload(t, "qbittorrent", `{}`, Job{Detail: "uploading"})
		if err := execSQL(w, q); err != nil {
			t.Fatal(err)
		}
		if err := m.Remove(context.Background(), id, RemoveChoice{DeleteFiles: true}, luis, "luis"); !errors.Is(err, ErrImporting) || len(eng.removed) != 0 {
			t.Fatalf("%s: %v %v", q, err, eng.removed)
		}
		if d := w.download(t, id); d.State != "finished" {
			t.Fatalf("%s: download %s", q, d.State)
		}
	}
}

// A read-only Downloader keeps its finished downloads.
func TestRemoveDoneReadOnlyDownloader(t *testing.T) {
	w, m, id, eng := seededDownload(t, "qbittorrent", `{}`, Job{Detail: "uploading"})
	if err := execSQL(w, `UPDATE downloaders SET writable = 0`); err != nil {
		t.Fatal(err)
	}
	m.Forget(w.download(t, id).DownloaderID)
	if err := m.Remove(context.Background(), id, RemoveChoice{}, luis, "luis"); !errors.Is(err, ErrDownloaderReadOnly) || len(eng.removed) != 0 {
		t.Fatalf("%v %v", err, eng.removed)
	}
	if d := w.download(t, id); d.State != "finished" {
		t.Fatalf("download %s", d.State)
	}
}

// The Downloader refuses: the person is told at once, History says it, and
// the remove is asked again on a later sync.
func TestRemoveDoneClientRefusalSaidAndRetried(t *testing.T) {
	_, m, id, se := seededDownload(t, "qbittorrent", `{}`, Job{Detail: "uploading"})
	eng := &diskEngine{seedEngine: se, fail: errors.New("qBittorrent said no")}
	m.Engine = func(domain.Downloader) (Engine, error) { return eng, nil }
	note, err := m.RemoveSaid(context.Background(), id, RemoveChoice{}, luis, "luis")
	if err != nil || !strings.Contains(note, "qBittorrent said no") || !strings.Contains(note, "tries again") {
		t.Fatalf("%q %v", note, err)
	}
	var pending *bool
	if err := m.Store.Read().QueryRow("SELECT removal_pending FROM downloads WHERE id = ?", id).Scan(&pending); err != nil || pending == nil {
		t.Fatalf("not waiting to retry: %v", err)
	}
	eng.fail = nil
	m.retryRemovals(context.Background())
	if len(se.removed) != 1 {
		t.Fatalf("retry removed %v", se.removed)
	}
}

// The Downloads view offers Remove on a finished row only to people who
// manage the Library, on a Downloader HolyArrs may change, and not while
// the files are being placed.
func TestDownloadsViewDoneRowRemovable(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotInLibrary, "seeding")
	exec41(t, x.s, `UPDATE downloaders SET writable = 1`)
	owner := webserver.Caller{UserID: 1, Role: webserver.RoleOwner, Via: "session"}
	if r := viewAs(t, x, owner); len(r) != 1 || !r[0].CanAct || r[0].ClientLocked {
		t.Fatalf("owner %+v", r)
	}
	exec41(t, x.s, `UPDATE downloaders SET writable = 0`)
	if r := viewAs(t, x, owner); len(r) != 1 || r[0].CanAct {
		t.Fatalf("read-only Downloader %+v", r)
	}
	exec41(t, x.s, `UPDATE downloaders SET writable = 1`)
	exec41(t, x.s, `UPDATE slots SET state = 'importing'`)
	if r := viewAs(t, x, owner); len(r) != 1 || r[0].CanAct || r[0].Phase != "importing" {
		t.Fatalf("importing %+v", r)
	}
}
