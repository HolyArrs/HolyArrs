package downloader

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/CoderLuii/holyarrs/internal/domain"
)

// removeSpy is an engine that records what was removed from it.
type removeSpy struct {
	removed []string
	err     error
}

func (s *removeSpy) Health(context.Context) (Health, error)          { return Health{}, nil }
func (s *removeSpy) Add(context.Context, AddRequest) (string, error) { return "id", nil }
func (s *removeSpy) List(context.Context, string) ([]Job, error)     { return nil, nil }
func (s *removeSpy) Get(context.Context, string) (Job, error)        { return Job{}, ErrNotFound }
func (s *removeSpy) Remove(_ context.Context, id string, files bool) error {
	if s.err != nil {
		return s.err
	}
	s.removed = append(s.removed, id+"|"+map[bool]string{true: "files", false: "keep"}[files])
	return nil
}

// usenetDownload makes the test Download a Usenet one in a Downloader that
// may change things and clears finished jobs.
func usenetDownload(t *testing.T) (*world, *Manager, int64, *removeSpy) {
	t.Helper()
	w, m := setup(t)
	id := grabbed(t, w, m)
	if err := execSQL(w, `UPDATE downloaders SET protocol = 'usenet', writable = 1, clear_finished = 1`); err != nil {
		t.Fatal(err)
	}
	spy := &removeSpy{}
	m.Engine = func(domain.Downloader) (Engine, error) { return spy, nil }
	m.Forget(w.download(t, id).DownloaderID) // grabbing cached the qBittorrent fake
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
	m.ClearFinished(context.Background(), id+1000) // no Download row: a client-only item
	if err := execSQL(w, `UPDATE downloads SET release_id = NULL`); err != nil {
		t.Fatal(err)
	}
	m.ClearFinished(context.Background(), id) // not sent by a Release of ours
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
	m.ClearFinished(context.Background(), id) // returns nothing: the Import stays done
	if countEvents(t, w, "download_not_removed", id) != 1 || countEvents(t, w, "download_removed", id) != 0 {
		t.Fatal("the failed removal is not in History")
	}
}
