package library

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// Disks that sleep: the timed Library folder check must not write probe
// files on every run. Only start-up, Run now and folder changes run the
// full write and hardlink test.

// folderCheckQueue starts a queue with the library's handlers and the
// given Downloader save path.
func folderCheckQueue(t *testing.T, s *database.Store, savePath string) (*Service, *jobs.Queue) {
	t.Helper()
	q := jobs.New(s, jobs.Options{Workers: 1, PollInterval: 5 * time.Millisecond, BaseBackoff: 10 * time.Millisecond})
	svc := &Service{Store: s, Queue: q, SavePath: savePath, Now: func() time.Time { return now }}
	svc.Register(q)
	if err := q.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(q.Stop)
	return svc, q
}

// runFolderCheck queues one check_library_folders Job with params and
// waits for it to end.
func runFolderCheck(t *testing.T, s *database.Store, q *jobs.Queue, params map[string]any) {
	t.Helper()
	ctx := context.Background()
	id, _, err := q.Enqueue(ctx, KindCheckFolders, nil, params, "", domain.System)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var j domain.Job
		if err := database.Get(ctx, s.Read(), "jobs", id, &j); err != nil {
			t.Fatal(err)
		}
		switch j.State {
		case "done":
			return
		case "failed", "cancelled":
			t.Fatalf("folder check ended %s: %+v", j.State, j.Error)
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("folder check never ended")
}

func storedFolder(t *testing.T, s *database.Store, id int64) mediafiles.LibraryFolder {
	t.Helper()
	var lf mediafiles.LibraryFolder
	if err := database.Get(context.Background(), s.Read(), "library_folders", id, &lf); err != nil {
		t.Fatal(err)
	}
	return lf
}

func modTime(t *testing.T, p string) time.Time {
	t.Helper()
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	return st.ModTime()
}

var timedOnly = map[string]any{"scheduled": true}
var runNow = map[string]any{"scheduled": true, "by_hand": true}

// A timed run on a healthy folder reads only: no probe file is created in
// the Library folder or the Downloader folder, the stored health (here
// no_hardlink) is kept even though a full test would now say ok, and "last
// checked" still names the last full test.
func TestTimedFolderCheckDoesNotWrite(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	lib := t.TempDir()
	dl := filepath.Join(t.TempDir(), "downloads") // not there yet: no hardlinks
	lf, err := AddFolder(ctx, s, FolderInput{Path: lib, Kind: "movie", Label: "Movies"}, []string{dl}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	_, q := folderCheckQueue(t, s, dl)
	runFolderCheck(t, s, q, runNow) // a full test, so the path list is known
	full := storedFolder(t, s, lf.ID)
	if full.Health != "no_hardlink" || full.HealthCheckedAt == nil {
		t.Fatalf("after Run now: %+v", full)
	}

	if err := os.Mkdir(dl, 0o755); err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond)
	libBefore, dlBefore := modTime(t, lib), modTime(t, dl)
	runFolderCheck(t, s, q, timedOnly)

	got := storedFolder(t, s, lf.ID)
	if got.Health != "no_hardlink" || got.HealthReason != full.HealthReason {
		t.Errorf("timed run changed health to %q (%q); want the stored no_hardlink kept", got.Health, got.HealthReason)
	}
	if got.HealthCheckedAt == nil || !got.HealthCheckedAt.Equal(*full.HealthCheckedAt) {
		t.Errorf("timed run moved health_checked_at from %v to %v; it must keep the last full test", full.HealthCheckedAt, got.HealthCheckedAt)
	}
	if !modTime(t, lib).Equal(libBefore) {
		t.Error("timed run wrote in the Library folder (its folder time moved)")
	}
	if !modTime(t, dl).Equal(dlBefore) {
		t.Error("timed run wrote in the Downloader folder (its folder time moved)")
	}
	if ents, _ := os.ReadDir(lib); len(ents) != 0 {
		t.Errorf("probe files left in the Library folder: %v", ents)
	}
}

// Run now (scheduled and by hand) still runs the full write and hardlink
// test.
func TestRunNowFolderCheckStillProbes(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	lib := t.TempDir()
	dl := filepath.Join(t.TempDir(), "downloads")
	lf, err := AddFolder(ctx, s, FolderInput{Path: lib, Kind: "movie", Label: "Movies"}, []string{dl}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	_, q := folderCheckQueue(t, s, dl)
	runFolderCheck(t, s, q, runNow)
	first := storedFolder(t, s, lf.ID)
	if first.Health != "no_hardlink" {
		t.Fatalf("first: %+v", first)
	}
	if err := os.Mkdir(dl, 0o755); err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond)
	runFolderCheck(t, s, q, runNow)
	got := storedFolder(t, s, lf.ID)
	if got.Health != "ok" || got.HealthCheckedAt == nil || !got.HealthCheckedAt.After(*first.HealthCheckedAt) {
		t.Fatalf("Run now did not run the full test: %+v (before %v)", got, first.HealthCheckedAt)
	}
}

// A timed run still runs the full test when the stored health is missing
// or read_only, when the folder was never checked, and when the list of
// Downloader folders changed since the last full test. A folder that is
// gone shows as missing.
func TestTimedFolderCheckProbesWhenItMust(t *testing.T) {
	ctx := context.Background()
	setHealth := func(t *testing.T, s *database.Store, id int64, health string, checked bool) {
		t.Helper()
		q := "UPDATE library_folders SET health = ?, health_reason = 'set by the test' WHERE id = ?"
		if !checked {
			q = "UPDATE library_folders SET health = ?, health_reason = 'set by the test', health_checked_at = NULL WHERE id = ?"
		}
		if err := s.Write(context.Background(), func(tx *sql.Tx) error {
			_, err := tx.Exec(q, health, id)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	for _, health := range []string{"missing", "read_only"} {
		t.Run("stored "+health, func(t *testing.T) {
			s := open(t)
			lf := addFolder(t, s)
			_, q := folderCheckQueue(t, s, "")
			runFolderCheck(t, s, q, runNow)
			setHealth(t, s, lf.ID, health, true)
			runFolderCheck(t, s, q, timedOnly)
			if got := storedFolder(t, s, lf.ID); got.Health != "ok" {
				t.Fatalf("stored %s was not tested again: %+v", health, got)
			}
		})
	}
	t.Run("never checked", func(t *testing.T) {
		s := open(t)
		lf := addFolder(t, s)
		_, q := folderCheckQueue(t, s, "")
		setHealth(t, s, lf.ID, "no_hardlink", false)
		runFolderCheck(t, s, q, timedOnly)
		if got := storedFolder(t, s, lf.ID); got.Health != "ok" || got.HealthCheckedAt == nil {
			t.Fatalf("never-checked folder was not tested: %+v", got)
		}
	})
	t.Run("download folders changed", func(t *testing.T) {
		s := open(t)
		lf := addFolder(t, s)
		svc, q := folderCheckQueue(t, s, "")
		runFolderCheck(t, s, q, runNow)
		if got := storedFolder(t, s, lf.ID); got.Health != "ok" {
			t.Fatalf("first: %+v", got)
		}
		svc.SavePath = filepath.Join(t.TempDir(), "new-downloads") // a new Downloader folder, unreachable
		runFolderCheck(t, s, q, timedOnly)
		if got := storedFolder(t, s, lf.ID); got.Health != "no_hardlink" {
			t.Fatalf("a new Downloader folder got no hardlink test: %+v", got)
		}
	})
	t.Run("folder gone", func(t *testing.T) {
		s := open(t)
		parent := t.TempDir()
		lib := filepath.Join(parent, "movies")
		if err := os.Mkdir(lib, 0o755); err != nil {
			t.Fatal(err)
		}
		lf, err := AddFolder(ctx, s, FolderInput{Path: lib, Kind: "movie"}, nil, domain.System)
		if err != nil {
			t.Fatal(err)
		}
		_, q := folderCheckQueue(t, s, "")
		runFolderCheck(t, s, q, runNow)
		if err := os.Remove(lib); err != nil {
			t.Fatal(err)
		}
		runFolderCheck(t, s, q, timedOnly)
		if got := storedFolder(t, s, lf.ID); got.Health != "missing" {
			t.Fatalf("gone folder: %+v", got)
		}
	})
}
