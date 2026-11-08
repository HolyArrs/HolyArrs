package backups

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func setup(t *testing.T) (*Manager, *database.Store, *clock, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	c := &clock{t: time.Date(2026, 10, 13, 3, 0, 0, 0, time.UTC)}
	return &Manager{Store: s, Dir: dir, Version: "1.2.3", Now: c.now}, s, c, dir
}

func names(t *testing.T, z string) []string {
	t.Helper()
	r, err := zip.OpenReader(z)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	var out []string
	for _, f := range r.File {
		out = append(out, f.Name)
	}
	sort.Strings(out)
	return out
}

func TestCreateHoldsDatabaseAndVersionNeverKeyOrMedia(t *testing.T) {
	m, _, _, dir := setup(t)
	_ = os.WriteFile(filepath.Join(dir, "holyarrs.key"), []byte("key"), 0o600)
	_ = os.MkdirAll(filepath.Join(dir, "cache"), 0o750)
	_ = os.WriteFile(filepath.Join(dir, "cache", "poster.jpg"), []byte("img"), 0o600)
	info, err := m.Create(context.Background(), KindManual)
	if err != nil {
		t.Fatal(err)
	}
	got := names(t, info.Path)
	want := []string{"backup.json", database.EventsFile, database.MainFile}
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("zip holds %v", got)
	}
	r, _ := zip.OpenReader(info.Path)
	defer r.Close()
	for _, f := range r.File {
		if f.Name != "backup.json" {
			continue
		}
		rc, _ := f.Open()
		var meta Meta
		_ = json.NewDecoder(rc).Decode(&meta)
		rc.Close()
		if meta.App != "holyarrs" || meta.Version != "1.2.3" || meta.Kind != KindManual || meta.SchemaMain < 5 || !strings.Contains(meta.Note, "key") {
			t.Fatalf("meta %+v", meta)
		}
	}
}

func TestPresets(t *testing.T) {
	for _, p := range []Preset{Balanced, Light, Careful} {
		if _, ok := Rules[p]; !ok {
			t.Fatalf("no rule for %s", p)
		}
	}
	if Rules[Balanced].Every != 24*time.Hour || Rules[Light].Every != 7*24*time.Hour || Rules[Careful].Every != 24*time.Hour {
		t.Fatal("schedules wrong")
	}
	m, _, _, _ := setup(t)
	if p, _ := m.Preset(context.Background()); p != Balanced {
		t.Fatalf("default preset %s", p)
	}
	if err := m.SetPreset(context.Background(), Careful, domain.System); err != nil {
		t.Fatal(err)
	}
	if p, _ := m.Preset(context.Background()); p != Careful {
		t.Fatalf("preset %s", p)
	}
	if err := m.SetPreset(context.Background(), "weird", domain.System); err == nil {
		t.Fatal("unknown preset accepted")
	}
}

// makeAt writes empty backup files with times, for retention tests.
func makeAt(t *testing.T, m *Manager, kind string, times ...time.Time) {
	t.Helper()
	_ = os.MkdirAll(m.backupDir(), 0o750)
	for _, at := range times {
		if err := os.WriteFile(filepath.Join(m.backupDir(), fileName(kind, at)), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func kept(t *testing.T, m *Manager, kind string) int {
	t.Helper()
	l, err := m.List()
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, b := range l {
		if b.Kind == kind {
			n++
		}
	}
	return n
}

func nightly(now time.Time, days int) []time.Time {
	var out []time.Time
	for i := range days {
		out = append(out, now.Add(-time.Duration(i)*24*time.Hour))
	}
	return out
}

func TestBalancedKeepsSevenNightlyAndFourWeekly(t *testing.T) {
	m, _, c, _ := setup(t)
	makeAt(t, m, KindScheduled, nightly(c.t, 60)...)
	makeAt(t, m, KindManual, c.t.Add(-400*24*time.Hour))
	if err := m.Prune(Balanced); err != nil {
		t.Fatal(err)
	}
	if n := kept(t, m, KindScheduled); n != 11 {
		t.Fatalf("kept %d scheduled, want 7+4", n)
	}
	if kept(t, m, KindManual) != 1 {
		t.Fatal("manual backup deleted")
	}
}

func TestLightAndCarefulKeepByAge(t *testing.T) {
	for p, want := range map[Preset]int{Light: 5, Careful: 31} {
		m, _, c, _ := setup(t)
		times := nightly(c.t, 60)
		if p == Light {
			times = nil
			for i := range 9 {
				times = append(times, c.t.Add(-time.Duration(i)*7*24*time.Hour))
			}
		}
		makeAt(t, m, KindScheduled, times...)
		if err := m.Prune(p); err != nil {
			t.Fatal(err)
		}
		if n := kept(t, m, KindScheduled); n != want {
			t.Fatalf("%s kept %d, want %d", p, n, want)
		}
	}
}

func TestDueFollowsPreset(t *testing.T) {
	m, _, c, _ := setup(t)
	if !m.due(Balanced) {
		t.Fatal("no backups yet: due")
	}
	makeAt(t, m, KindScheduled, c.t.Add(-2*time.Hour))
	if m.due(Balanced) {
		t.Fatal("made two hours ago: not due")
	}
	c.t = c.t.Add(23 * time.Hour)
	if !m.due(Balanced) || m.due(Light) {
		t.Fatal("a day later: due nightly, not weekly")
	}
}

func TestUpdateBackupKeptUntilNextUpdateSucceeds(t *testing.T) {
	m, _, _, _ := setup(t)
	ctx := context.Background()
	if err := m.AfterStart(ctx); err != nil { // first start: nothing to back up
		t.Fatal(err)
	}
	if kept(t, m, KindUpdate) != 0 {
		t.Fatal("first start made an update backup")
	}
	m.Version = "1.2.4"
	if err := m.AfterStart(ctx); err != nil {
		t.Fatal(err)
	}
	if kept(t, m, KindUpdate) != 1 {
		t.Fatal("no backup for the update")
	}
	m.Now = func() time.Time { return time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC) }
	m.Version = "1.3.0"
	if err := m.AfterStart(ctx); err != nil {
		t.Fatal(err)
	}
	if kept(t, m, KindUpdate) != 1 {
		t.Fatalf("older update backup should go once the next update started: %d", kept(t, m, KindUpdate))
	}
}

func TestAdoptMigrationCopies(t *testing.T) {
	m, s, _, dir := setup(t)
	_ = os.MkdirAll(m.backupDir(), 0o750)
	raw := filepath.Join(m.backupDir(), "main-before-0009-20261013T030000.000Z.db")
	if _, err := s.Read().Exec("SELECT 1"); err != nil {
		t.Fatal(err)
	}
	src, _, err := s.Snapshot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(src)
	_ = os.WriteFile(raw, b, 0o600)
	pinned := filepath.Join(m.backupDir(), "events-pre-update-0001-to-0002-20261023T030000.000Z.db")
	_ = os.WriteFile(pinned, b, 0o600)
	sum := sha256.Sum256(b)
	_ = os.WriteFile(pinned+".sha256", []byte(hex.EncodeToString(sum[:])+"  x\n"), 0o600)
	bad := filepath.Join(m.backupDir(), "main-pre-update-0001-to-0002-20261023T040000.000Z.db")
	_ = os.WriteFile(bad, b, 0o600)
	_ = os.WriteFile(bad+".sha256", []byte("00  x\n"), 0o600)
	n, err := m.AdoptMigrationCopies()
	if err != nil || n != 2 {
		t.Fatalf("adopted %d %v", n, err)
	}
	for _, p := range []string{raw, pinned, pinned + ".sha256"} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("raw copy left behind: %s", p)
		}
	}
	if _, err := os.Stat(bad); err != nil {
		t.Fatal("a copy that fails its sha256 must stay put")
	}
	if kept(t, m, KindPreUpdate) != 1 {
		t.Fatal("the newest main and events copies should make one pre-update backup")
	}
	all, _ := m.List()
	var meta Meta
	zr, err := zip.OpenReader(all[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range zr.File {
		if f.Name == MetaFile {
			rc, _ := f.Open()
			_ = json.NewDecoder(rc).Decode(&meta)
			rc.Close()
		}
	}
	zr.Close()
	if meta.SchemaMain != 8 || meta.SchemaEvents != 1 || meta.Kind != KindPreUpdate || all[0].Detail == "" {
		t.Fatalf("meta %+v, detail %q", meta, all[0].Detail)
	}
	_ = dir
}

func zipBytes(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for n, b := range files {
		f, _ := w.Create(n)
		_, _ = f.Write(b)
	}
	_ = w.Close()
	return buf.Bytes()
}

func TestStageRefusesBadZips(t *testing.T) {
	m, _, _, _ := setup(t)
	ctx := context.Background()
	good, err := m.Create(ctx, KindManual)
	if err != nil {
		t.Fatal(err)
	}
	gb, _ := os.ReadFile(good.Path)
	meta, _ := json.Marshal(Meta{App: "holyarrs", SchemaMain: 999})
	for name, body := range map[string][]byte{
		"not a zip":     []byte("hello"),
		"no meta":       zipBytes(t, map[string][]byte{database.MainFile: []byte("x")}),
		"path escape":   zipBytes(t, map[string][]byte{"backup.json": []byte(`{"app":"holyarrs"}`), "../evil": []byte("x")}),
		"newer schema":  zipBytes(t, map[string][]byte{"backup.json": meta, database.MainFile: []byte("x")}),
		"broken db":     zipBytes(t, map[string][]byte{"backup.json": []byte(`{"app":"holyarrs","schema_main":1}`), database.MainFile: []byte("not sqlite")}),
		"other app zip": zipBytes(t, map[string][]byte{"backup.json": []byte(`{"app":"other"}`), database.MainFile: []byte("x")}),
	} {
		if err := m.Stage(ctx, bytes.NewReader(body)); err == nil {
			t.Fatalf("%s: accepted", name)
		}
		if _, err := os.Stat(filepath.Join(m.Dir, StagedDir)); !os.IsNotExist(err) {
			t.Fatalf("%s: left a staged folder", name)
		}
	}
	m.CheckKey = func(string) error { return errors.New("key mismatch") }
	if err := m.Stage(ctx, bytes.NewReader(gb)); err == nil || !strings.Contains(err.Error(), "key") || !strings.Contains(err.Error(), "Move to a new machine") {
		t.Fatalf("key check not applied, or no pointer to the new-machine steps: %v", err)
	}
	m.CheckKey = nil
	if err := m.Stage(ctx, bytes.NewReader(gb)); err != nil {
		t.Fatal(err)
	}
}

func TestRestoreSwapsOnRestart(t *testing.T) {
	m, s, _, dir := setup(t)
	ctx := context.Background()
	write := func(name string) {
		err := s.Write(ctx, func(tx *sql.Tx) error {
			_, err := database.Insert(tx, "library_folders", &mediafiles.LibraryFolder{Path: "/m/" + name, Kind: "movie", Label: name,
				Health: "ok", SameDiskAs: []int64{}}, domain.System)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	write("before")
	b, err := m.Create(ctx, KindManual)
	if err != nil {
		t.Fatal(err)
	}
	write("after")
	f, _ := os.Open(b.Path)
	if err := m.Stage(ctx, f); err != nil {
		t.Fatal(err)
	}
	f.Close()
	_ = s.Close()
	done, err := ApplyStaged(dir)
	if err != nil || !done {
		t.Fatalf("apply %v %v", done, err)
	}
	s2, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	var n int
	_ = s2.Read().QueryRow("SELECT count(*) FROM library_folders").Scan(&n)
	if n != 1 {
		t.Fatalf("restored database has %d folders, want 1", n)
	}
	if again, _ := ApplyStaged(dir); again {
		t.Fatal("staged restore applied twice")
	}
	old, _ := filepath.Glob(filepath.Join(dir, "backups", "before-restore-*", database.MainFile))
	if len(old) != 1 {
		t.Fatalf("replaced database not kept: %v", old)
	}
}

func TestApplyStagedRollsBackARecoverableSwapFailure(t *testing.T) {
	for _, failDB := range []string{database.MainFile, database.EventsFile} {
		t.Run(failDB, func(t *testing.T) {
			dir := t.TempDir()
			staged := filepath.Join(dir, StagedDir)
			if err := os.MkdirAll(staged, 0o750); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(staged, readyFile), []byte("ready"), 0o600); err != nil {
				t.Fatal(err)
			}
			for _, db := range []string{database.MainFile, database.EventsFile} {
				if err := os.WriteFile(filepath.Join(dir, db), []byte("old-"+db), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, db+"-wal"), []byte("old-wal-"+db), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(staged, db), []byte("new-"+db), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			original := applyRename
			failed := false
			applyRename = func(from, to string) error {
				if !failed && from == filepath.Join(staged, failDB) {
					failed = true
					return errors.New("injected rename failure")
				}
				return original(from, to)
			}
			t.Cleanup(func() { applyRename = original })

			done, err := ApplyStaged(dir)
			if done || err == nil {
				t.Fatalf("apply = %v, %v; want a rolled-back failure", done, err)
			}
			for _, db := range []string{database.MainFile, database.EventsFile} {
				for _, tc := range []struct{ path, want string }{
					{filepath.Join(dir, db), "old-" + db},
					{filepath.Join(dir, db+"-wal"), "old-wal-" + db},
					{filepath.Join(staged, db), "new-" + db},
				} {
					got, readErr := os.ReadFile(tc.path)
					if readErr != nil || string(got) != tc.want {
						t.Fatalf("%s = %q, %v; want %q", tc.path, got, readErr, tc.want)
					}
				}
			}
			if !Staged(dir) {
				t.Fatal("the staged restore is no longer retryable")
			}
		})
	}
}

func TestDeleteAndOpenRefuseOddNames(t *testing.T) {
	m, _, _, _ := setup(t)
	for _, n := range []string{"../holyarrs.db", "x.zip", "holyarrs-backup-manual-bad.zip/.."} {
		if err := m.Delete(n); err == nil {
			t.Fatalf("delete %q accepted", n)
		}
		if _, err := m.Open(n); err == nil {
			t.Fatalf("open %q accepted", n)
		}
	}
	b, _ := m.Create(context.Background(), KindManual)
	rc, err := m.Open(b.Name)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, rc)
	rc.Close()
	if err := m.Delete(b.Name); err != nil {
		t.Fatal(err)
	}
}

func TestStageCapsTotalUnpackedSize(t *testing.T) {
	m, _, _, _ := setup(t)
	old := maxUnpacked
	maxUnpacked = 1500
	defer func() { maxUnpacked = old }()
	big := bytes.Repeat([]byte("a"), 1000) // each entry is under the cap, the sum is not
	z := zipBytes(t, map[string][]byte{MetaFile: append([]byte(`{"app":"holyarrs"}`), bytes.Repeat([]byte(" "), 900)...),
		database.MainFile: big})
	err := m.Stage(context.Background(), bytes.NewReader(z))
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("total size not capped: %v", err)
	}
}

// crashAfterFirstMove stages a restore over old databases and stops
// ApplyStaged right after its first move, as a power cut would.
func crashAfterFirstMove(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	staged := filepath.Join(dir, StagedDir)
	if err := os.MkdirAll(staged, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staged, readyFile), []byte("ready"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, db := range []string{database.MainFile, database.EventsFile} {
		if err := os.WriteFile(filepath.Join(dir, db), []byte("old-"+db), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(staged, db), []byte("new-"+db), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	original := applyRename
	t.Cleanup(func() { applyRename = original })
	type crash struct{}
	applyRename = func(from, to string) error {
		if err := original(from, to); err != nil {
			return err
		}
		panic(crash{})
	}
	func() {
		defer func() {
			if _, ok := recover().(crash); !ok {
				t.Fatal("ApplyStaged did not reach its first move")
			}
		}()
		_, _ = ApplyStaged(dir)
	}()
	applyRename = original
	if _, err := os.Stat(filepath.Join(dir, JournalFile)); err != nil {
		t.Fatalf("no journal after the cut: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, database.MainFile)); err == nil {
		t.Fatal("first move did not happen")
	}
	return dir
}

func wantFiles(t *testing.T, dir, prefix string) {
	t.Helper()
	for _, db := range []string{database.MainFile, database.EventsFile} {
		got, err := os.ReadFile(filepath.Join(dir, db))
		if err != nil || string(got) != prefix+db {
			t.Fatalf("%s = %q, %v; want %q", db, got, err, prefix+db)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, JournalFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("journal left behind: %v", err)
	}
}

func TestRestoreCutOffAfterFirstMoveIsFinished(t *testing.T) {
	dir := crashAfterFirstMove(t)
	done, err := ApplyStaged(dir)
	if err != nil || done {
		t.Fatalf("apply after cut = %v, %v", done, err)
	}
	wantFiles(t, dir, "new-")
	if Staged(dir) {
		t.Fatal("staged restore still waiting")
	}
	old, _ := filepath.Glob(filepath.Join(dir, "backups", "before-restore-*", database.MainFile))
	if len(old) != 1 {
		t.Fatalf("replaced database not kept: %v", old)
	}
}

func TestRestoreCutOffThatCannotFinishIsUndone(t *testing.T) {
	dir := crashAfterFirstMove(t)
	// The staged copy and the ready mark went missing: finishing would
	// leave no main database, so the cut-off restore is undone.
	if err := os.RemoveAll(filepath.Join(dir, StagedDir)); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyStaged(dir); err != nil {
		t.Fatal(err)
	}
	wantFiles(t, dir, "old-")
}

func TestRestoreCutOffAfterStagedFolderRemovedStarts(t *testing.T) {
	dir := crashAfterFirstMove(t)
	jb, err := os.ReadFile(filepath.Join(dir, JournalFile))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyStaged(dir); err != nil {
		t.Fatal(err)
	}
	// Cut off after restore-staged was removed but before the journal was:
	// the next start must still open with the restored data.
	if err := os.WriteFile(filepath.Join(dir, JournalFile), jb, 0o600); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	log.SetOutput(&logs)
	_, err = ApplyStaged(dir)
	log.SetOutput(os.Stderr)
	if err != nil {
		t.Fatal(err)
	}
	wantFiles(t, dir, "new-")
	// The restored data is live, so the log must not claim it was undone.
	if l := logs.String(); strings.Contains(l, "undo") || !strings.Contains(l, "finished") {
		t.Fatalf("log after a finished cut-off restore: %q", l)
	}
	if _, err := os.Stat(filepath.Join(dir, JournalFile)); err == nil {
		t.Fatal("journal left behind")
	}
}
