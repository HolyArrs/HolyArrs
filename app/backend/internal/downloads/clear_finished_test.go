package downloads

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

type removeSpy struct {
	removed []string
	err     error
	content string
}

func (s *removeSpy) Health(context.Context) (Health, error) { return Health{}, nil }

func (s *removeSpy) Add(context.Context, AddRequest) (string, error) { return "id", nil }

func (s *removeSpy) List(context.Context, string) ([]Job, error) { return nil, nil }

func (s *removeSpy) Get(_ context.Context, id string) (Job, error) {
	if s.content == "" {
		return Job{}, ErrNotFound
	}
	return Job{ID: id, ContentPath: s.content}, nil
}

func (s *removeSpy) Remove(_ context.Context, id string, files bool) error {
	if s.err != nil {
		return s.err
	}
	s.removed = append(s.removed, id+"|"+map[bool]string{true: "files", false: "keep"}[files])
	return nil
}

func TestClearFinishedRemovesJobWithFiles(t *testing.T) {
	w, m, id, spy := usenetDownload(t)
	m.ClearFinished(context.Background(), id)
	if len(spy.removed) != 1 || !strings.HasSuffix(spy.removed[0], "|files") {
		t.Fatalf("removed %v", spy.removed)
	}
	if countEvents(t, w, "download_removed", id) != 1 {
		t.Fatal("no History entry for the removal")
	}
}

func TestClearFinishedLeavesTorrentsAlone(t *testing.T) {
	w, m, id, spy := usenetDownload(t)
	if err := execSQL(w, `UPDATE downloaders SET protocol = 'torrent'`); err != nil {
		t.Fatal(err)
	}
	m.ClearFinished(context.Background(), id)
	if len(spy.removed) != 0 {
		t.Fatalf("removed %v", spy.removed)
	}
}

func TestClearFinishedLeavesOutsideItemsAlone(t *testing.T) {
	w, m, id, spy := usenetDownload(t)
	m.ClearFinished(context.Background(), id+1000)
	if err := execSQL(w, `UPDATE downloads SET release_id = NULL`); err != nil {
		t.Fatal(err)
	}
	m.ClearFinished(context.Background(), id)
	if len(spy.removed) != 0 {
		t.Fatalf("removed %v", spy.removed)
	}
}

func TestClearFinishedLeavesReadOnlyDownloaderAlone(t *testing.T) {
	w, m, id, spy := usenetDownload(t)
	if err := execSQL(w, `UPDATE downloaders SET writable = 0`); err != nil {
		t.Fatal(err)
	}
	m.ClearFinished(context.Background(), id)
	t.Setenv(ReadOnlyEnv, "http://elsewhere")
	if err := execSQL(w, `UPDATE downloaders SET writable = 1, url = 'http://elsewhere'`); err != nil {
		t.Fatal(err)
	}
	m.ClearFinished(context.Background(), id)
	if len(spy.removed) != 0 {
		t.Fatalf("removed %v", spy.removed)
	}
}

func TestClearFinishedSettingOffLeavesJobAlone(t *testing.T) {
	w, m, id, spy := usenetDownload(t)
	if err := execSQL(w, `UPDATE downloaders SET clear_finished = 0`); err != nil {
		t.Fatal(err)
	}
	m.ClearFinished(context.Background(), id)
	if len(spy.removed) != 0 {
		t.Fatalf("removed %v", spy.removed)
	}
}

func TestClearFinishedClientErrorIsLoggedNotFatal(t *testing.T) {
	w, m, id, spy := usenetDownload(t)
	spy.err = errors.New("SABnzbd said no")
	m.ClearFinished(context.Background(), id)
	if countEvents(t, w, "download_not_removed", id) != 1 || countEvents(t, w, "download_removed", id) != 0 {
		t.Fatal("the failed removal is not in History")
	}
}

func TestClearFinishedDeletesTheJobFolderOnDisk(t *testing.T) {
	share := t.TempDir()
	job := filepath.Join(share, "complete", "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS.1")
	if err := os.MkdirAll(job, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(job, "Dune.Part.Two.2024.mkv"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	w, m, id, spy := usenetDownload(t)
	if err := execSQL(w, `UPDATE downloaders SET path_map = '[{"theirs":"/downloads","ours":`+jsonStr(share)+`}]'`); err != nil {
		t.Fatal(err)
	}
	m.Forget(w.download(t, id).DownloaderID)
	spy.content = "/downloads/complete/Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS.1"
	m.ClearFinished(context.Background(), id)
	if _, err := os.Stat(job); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the job folder is still on disk: %v", err)
	}
	if _, err := os.Stat(share); err != nil {
		t.Fatalf("the shared folder itself went: %v", err)
	}
	if countEvents(t, w, "download_removed", id) != 1 {
		t.Fatal("no History entry")
	}
}

func TestClearFinishedDeletesTheFoldersOfAJobNamedByItsFile(t *testing.T) {
	share := t.TempDir()
	movies := filepath.Join(share, "complete", "movies")
	job := filepath.Join(movies, "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS")
	other := filepath.Join(movies, "Other.Movie.2020.1080p-GRP")
	for _, d := range []string{job, other} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, filepath.Base(d)+".mkv"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	w, m, id, spy := usenetDownload(t)
	if err := execSQL(w, `UPDATE downloaders SET path_map = '[{"theirs":"/downloads","ours":`+jsonStr(share)+`}]'`); err != nil {
		t.Fatal(err)
	}
	m.Forget(w.download(t, id).DownloaderID)
	spy.content = "/downloads/complete/movies/Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS/Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS.mkv"
	m.ClearFinished(context.Background(), id)
	if _, err := os.Stat(job); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the job folder is still on disk: %v", err)
	}
	for _, p := range []string{movies, other} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("%s was deleted: %v", p, err)
		}
	}
	if countEvents(t, w, "download_removed", id) != 1 {
		t.Fatal("no History entry")
	}
}

func TestClearFinishedNeverDeletesOutsideOrTheShareItself(t *testing.T) {
	share, outside := t.TempDir(), t.TempDir()
	w, m, id, spy := usenetDownload(t)
	if err := execSQL(w, `UPDATE downloaders SET path_map = '[{"theirs":"/downloads","ours":`+jsonStr(share)+`}]'`); err != nil {
		t.Fatal(err)
	}
	m.Forget(w.download(t, id).DownloaderID)
	for _, c := range []string{"/downloads", outside} {
		spy.content = c
		m.ClearFinished(context.Background(), id)
	}
	for _, p := range []string{share, outside} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("%s was deleted: %v", p, err)
		}
	}
}

func TestClearFinishedNeverDeletesAFolderNamedOtherwise(t *testing.T) {
	share := t.TempDir()
	cat := filepath.Join(share, "complete", "movies")
	other := filepath.Join(cat, "Another.Movie.2023")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	w, m, id, spy := usenetDownload(t)
	if err := execSQL(w, `UPDATE downloaders SET path_map = '[{"theirs":"/downloads","ours":`+jsonStr(share)+`}]'`); err != nil {
		t.Fatal(err)
	}
	m.Forget(w.download(t, id).DownloaderID)
	spy.content = "/downloads/complete/movies"
	m.ClearFinished(context.Background(), id)
	if _, err := os.Stat(other); err != nil {
		t.Fatalf("another job's folder was deleted: %v", err)
	}
	if !sameJobName("Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS.2", "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS") || sameJobName("movies", "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS") {
		t.Fatal("sameJobName")
	}
}

// A job folder in a Library folder, or one that holds a Library folder, is
// never deleted (the same rule a torrent's files follow); the job still
// leaves the Downloader.
func TestClearFinishedNeverDeletesInOrAroundALibraryFolder(t *testing.T) {
	const name = "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS"
	for _, lib := range []string{"complete", filepath.Join("complete", name, "Movies")} {
		share := t.TempDir()
		job := filepath.Join(share, "complete", name)
		if err := os.MkdirAll(job, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(job, name+".mkv"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		w, m, id, spy := usenetDownload(t)
		if err := execSQL(w, `UPDATE downloaders SET path_map = '[{"theirs":"/downloads","ours":`+jsonStr(share)+`}]'`); err != nil {
			t.Fatal(err)
		}
		exec41(t, w.s, "UPDATE library_folders SET path = ?", filepath.Join(share, lib))
		m.Forget(w.download(t, id).DownloaderID)
		spy.content = "/downloads/complete/" + name
		m.ClearFinished(context.Background(), id)
		if _, err := os.Stat(filepath.Join(job, name+".mkv")); err != nil {
			t.Fatalf("library folder %s: the job's files were deleted: %v", lib, err)
		}
		if len(spy.removed) != 1 {
			t.Fatalf("library folder %s: removed %v", lib, spy.removed)
		}
	}
}

func TestClearFinishedNeverFollowsALinkedParentOutside(t *testing.T) {
	share, outside := t.TempDir(), t.TempDir()
	victim := filepath.Join(outside, "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS")
	if err := os.MkdirAll(victim, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(share, "complete")); err != nil {
		t.Skipf("symlinks not available here: %v", err)
	}
	w, m, id, spy := usenetDownload(t)
	if err := execSQL(w, `UPDATE downloaders SET path_map = '[{"theirs":"/downloads","ours":`+jsonStr(share)+`}]'`); err != nil {
		t.Fatal(err)
	}
	m.Forget(w.download(t, id).DownloaderID)
	spy.content = "/downloads/complete/Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS"
	m.ClearFinished(context.Background(), id)
	if _, err := os.Stat(victim); err != nil {
		t.Fatalf("a folder outside the share was deleted through a link: %v", err)
	}
}

func usenetDownload(t *testing.T) (*world, *Manager, int64, *removeSpy) {
	t.Helper()
	w, m := setup(t)
	id := grabbed(t, w, m)
	if err := execSQL(w, `UPDATE downloaders SET protocol = 'usenet', writable = 1, clear_finished = 1`); err != nil {
		t.Fatal(err)
	}
	spy := &removeSpy{}
	m.Engine = func(domain.Downloader) (Engine, error) { return spy, nil }
	m.Forget(w.download(t, id).DownloaderID)
	return w, m, id, spy
}

func execSQL(w *world, q string) error {
	return w.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec(q)
		return err
	})
}

func countEvents(t *testing.T, w *world, kind string, id int64) int {
	t.Helper()
	var n int
	if err := w.s.ReadEvents().QueryRow("SELECT count(*) FROM events WHERE kind = ? AND download_id = ?", kind, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func jsonStr(s string) string { b, _ := json.Marshal(s); return string(b) }
