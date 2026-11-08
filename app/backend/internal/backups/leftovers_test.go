package backups

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

func put(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return string(b)
}

// A backup or restore upload cut off by a stop or kill leaves its working
// folder and a .zip.part; they are never valid after a restart, so the
// start removes them. Before-restore folders and a staged restore stay.
func TestCleanLeftoversRemovesOnlyCutOffWork(t *testing.T) {
	dir := t.TempDir()
	put(t, filepath.Join(dir, ".backup-123", database.MainFile), strings.Repeat("x", 1000))
	put(t, filepath.Join(dir, ".restore-456", "upload.zip"), strings.Repeat("y", 500))
	put(t, filepath.Join(dir, "backups", "holyarrs-backup-manual-20261101T000000.000Z.zip.part"), strings.Repeat("z", 100))
	keep := []string{
		filepath.Join(dir, database.MainFile),
		filepath.Join(dir, StagedDir, readyFile),
		filepath.Join(dir, "backups", "before-restore-20261101T000000.000Z", database.MainFile),
		filepath.Join(dir, "backups", "holyarrs-backup-manual-20261101T000000.000Z.zip"),
		filepath.Join(dir, ".backup-note"), // a file, not a folder we made
	}
	for _, p := range keep {
		put(t, p, "keep")
	}
	freed, err := CleanLeftovers(dir)
	if err != nil {
		t.Fatal(err)
	}
	if freed != 1600 {
		t.Errorf("freed %d bytes, want 1600", freed)
	}
	for _, p := range []string{".backup-123", ".restore-456", filepath.Join("backups", "holyarrs-backup-manual-20261101T000000.000Z.zip.part")} {
		if present(filepath.Join(dir, p)) {
			t.Errorf("%s still there", p)
		}
	}
	for _, p := range keep {
		if !present(p) {
			t.Errorf("%s was removed", p)
		}
	}
	// Nothing to do, no backups folder: fine.
	if n, err := CleanLeftovers(t.TempDir()); err != nil || n != 0 {
		t.Fatalf("empty dir: %d %v", n, err)
	}
}

// A backup asked to stop (shutdown) stops and leaves nothing behind.
func TestCreateStopsWhenCanceled(t *testing.T) {
	m, _, _, dir := setup(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.Create(ctx, KindManual); !errors.Is(err, context.Canceled) {
		t.Fatalf("want canceled, got %v", err)
	}
	es, _ := os.ReadDir(dir)
	for _, e := range es {
		if strings.HasPrefix(e.Name(), ".backup-") {
			t.Errorf("left %s", e.Name())
		}
	}
	bs, _ := os.ReadDir(filepath.Join(dir, "backups"))
	for _, e := range bs {
		t.Errorf("left backups/%s", e.Name())
	}
}

// A manual backup that clearly cannot fit says so plainly.
func TestManualBackupSaysWhenTheDiskIsFull(t *testing.T) {
	m, _, _, _ := setup(t)
	m.FreeSpace = func(string) int64 { return 1024 }
	mux := http.NewServeMux()
	m.Routes(mux, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/backups", nil))
	var body map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != http.StatusInsufficientStorage || !strings.Contains(body["error"], "Not enough free space") || strings.Contains(body["error"], "ten minutes") {
		t.Fatalf("%d %q", rec.Code, body["error"])
	}
	if all, _ := m.List(); len(all) != 0 {
		t.Fatalf("no backup should be made: %v", all)
	}
}

// A disk that fills up during the backup also gets the plain message.
func TestDiskFullErrorIsPlain(t *testing.T) {
	for _, e := range []error{errors.New("snapshot: database or disk is full (13)"), errors.New("write x: no space left on device")} {
		if !diskFull(e) {
			t.Errorf("not seen as disk full: %v", e)
		}
	}
	if diskFull(errors.New("permission denied")) || diskFull(nil) {
		t.Error("other errors are not disk full")
	}
}

// restoreState makes a config folder with old databases and a staged
// restore, and the journal ApplyStaged writes, with the first k moves done.
func restoreState(t *testing.T, k int) (string, journal) {
	t.Helper()
	dir := t.TempDir()
	put(t, filepath.Join(dir, database.MainFile), "old-main")
	put(t, filepath.Join(dir, database.MainFile+"-wal"), "old-main-wal")
	put(t, filepath.Join(dir, database.EventsFile), "old-events")
	put(t, filepath.Join(dir, StagedDir, database.MainFile), "new-main")
	put(t, filepath.Join(dir, StagedDir, database.EventsFile), "new-events")
	put(t, filepath.Join(dir, StagedDir, readyFile), "x")
	keepRel := filepath.Join("backups", "before-restore-20261101T000000.000Z")
	if err := os.MkdirAll(filepath.Join(dir, keepRel), 0o750); err != nil {
		t.Fatal(err)
	}
	j := journal{Keep: keepRel, Moves: []journalMove{
		{From: database.MainFile, To: filepath.Join(keepRel, database.MainFile)},
		{From: database.MainFile + "-wal", To: filepath.Join(keepRel, database.MainFile+"-wal")},
		{From: filepath.Join(StagedDir, database.MainFile), To: database.MainFile},
		{From: database.EventsFile, To: filepath.Join(keepRel, database.EventsFile)},
		{From: filepath.Join(StagedDir, database.EventsFile), To: database.EventsFile},
	}}
	if err := writeJournal(dir, j); err != nil {
		t.Fatal(err)
	}
	for _, mv := range j.Moves[:k] {
		if err := os.Rename(filepath.Join(dir, mv.From), filepath.Join(dir, mv.To)); err != nil {
			t.Fatal(err)
		}
	}
	return dir, j
}

func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev, flags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev); log.SetFlags(flags) })
	return &buf
}

// A restore cut off after any number of moves is finished on the next
// start, says so, and keeps every replaced database in the one folder the
// message names.
func TestCutOffRestoreIsFinishedAtEveryStep(t *testing.T) {
	for k := 0; k <= 5; k++ {
		dir, j := restoreState(t, k)
		buf := captureLog(t)
		done, err := ApplyStaged(dir)
		if err != nil {
			t.Fatalf("k=%d: %v", k, err)
		}
		if done {
			t.Errorf("k=%d: restore applied a second time", k)
		}
		out := buf.String()
		if strings.Contains(out, "undo") || !strings.Contains(out, "finished") || !strings.Contains(out, j.Keep) {
			t.Errorf("k=%d: log %q", k, out)
		}
		if got := read(t, filepath.Join(dir, database.MainFile)); got != "new-main" {
			t.Errorf("k=%d: main %q", k, got)
		}
		if got := read(t, filepath.Join(dir, database.EventsFile)); got != "new-events" {
			t.Errorf("k=%d: events %q", k, got)
		}
		keep := filepath.Join(dir, j.Keep)
		for f, want := range map[string]string{database.MainFile: "old-main", database.MainFile + "-wal": "old-main-wal", database.EventsFile: "old-events"} {
			if got := read(t, filepath.Join(keep, f)); got != want {
				t.Errorf("k=%d: kept %s %q", k, f, got)
			}
		}
		es, _ := os.ReadDir(filepath.Join(dir, "backups"))
		if len(es) != 1 {
			t.Errorf("k=%d: want one before-restore folder, got %d", k, len(es))
		}
		if present(filepath.Join(dir, StagedDir)) || present(filepath.Join(dir, JournalFile)) {
			t.Errorf("k=%d: staged folder or journal left", k)
		}
	}
}

// A restore that cannot be finished (a staged file is gone) is undone and
// says so; the old databases are back.
func TestCutOffRestoreUndoneWhenItCannotFinish(t *testing.T) {
	dir, _ := restoreState(t, 3)
	if err := os.Remove(filepath.Join(dir, StagedDir, database.EventsFile)); err != nil {
		t.Fatal(err)
	}
	buf := captureLog(t)
	if err := recoverJournal(dir); err != nil {
		t.Fatal(err)
	}
	if out := buf.String(); !strings.Contains(out, "undone") || strings.Contains(out, "has been finished") {
		t.Errorf("log %q", out)
	}
	for f, want := range map[string]string{database.MainFile: "old-main", database.MainFile + "-wal": "old-main-wal", database.EventsFile: "old-events"} {
		if got := read(t, filepath.Join(dir, f)); got != want {
			t.Errorf("%s %q", f, got)
		}
	}
	if got := read(t, filepath.Join(dir, StagedDir, database.MainFile)); got != "new-main" {
		t.Errorf("staged main not put back: %q", got)
	}
}

// A manual backup keeps going when the browser or a proxy drops the request;
// only a stop of HolyArrs cuts it off.
func TestManualBackupFinishesWhenTheClientLeaves(t *testing.T) {
	m, _, _, _ := setup(t)
	mux := http.NewServeMux()
	m.Routes(mux, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // the client is already gone
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/backups", nil).WithContext(ctx))
	if rec.Code != http.StatusCreated {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	if all, _ := m.List(); len(all) != 1 {
		t.Fatalf("want one backup, got %v", all)
	}
}

func TestManualBackupStopsOnShutdown(t *testing.T) {
	m, _, _, _ := setup(t)
	stop, cancel := context.WithCancel(context.Background())
	cancel()
	m.Stop = stop
	mux := http.NewServeMux()
	m.Routes(mux, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/backups", nil))
	if rec.Code == http.StatusCreated {
		t.Fatalf("backup should stop on shutdown")
	}
	if all, _ := m.List(); len(all) != 0 {
		t.Fatalf("no backup should be made: %v", all)
	}
}
