package downloads

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// clearRetryRig is an imported Usenet download on a test clock.
func clearRetryRig(t *testing.T) (*world, *Manager, int64, *removeSpy, *time.Time) {
	t.Helper()
	w, m, id, spy := usenetDownload(t)
	if err := execSQL(w, `UPDATE downloads SET state = 'finished'`); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 11, 3, 0, 0, 0, time.UTC)
	m.Now = func() time.Time { return now }
	return w, m, id, spy, &now
}

func clearWait(t *testing.T, w *world, id int64) (at sql.NullString, tries int) {
	t.Helper()
	if err := w.s.Read().QueryRow("SELECT clear_retry_at, clear_tries FROM downloads WHERE id = ?", id).Scan(&at, &tries); err != nil {
		t.Fatal(err)
	}
	return at, tries
}

// A busy Downloader means try later: no failure in History, and the clear
// runs again on a later sync once its wait is over, then never again.
func TestClearFinishedRetriesABusyDownloaderLater(t *testing.T) {
	w, m, id, spy, now := clearRetryRig(t)
	ctx := context.Background()
	spy.err = outbound.ErrSuppressed
	m.ClearFinished(ctx, id)
	if countEvents(t, w, "download_not_removed", id) != 0 {
		t.Fatal("a busy Downloader was logged as a failure")
	}
	if at, tries := clearWait(t, w, id); !at.Valid || tries != 1 {
		t.Fatalf("not waiting to clear: %v %d", at, tries)
	}
	spy.err = nil
	_, _ = m.SyncOnce(ctx)
	if len(spy.removed) != 0 {
		t.Fatal("retried before the wait was over")
	}
	*now = now.Add(time.Hour)
	_, _ = m.SyncOnce(ctx)
	if len(spy.removed) != 1 {
		t.Fatalf("removed %v", spy.removed)
	}
	if at, _ := clearWait(t, w, id); at.Valid {
		t.Fatal("still waiting after the clear worked")
	}
	if countEvents(t, w, "download_removed", id) != 1 {
		t.Fatal("no History entry for the clear")
	}
	*now = now.Add(24 * time.Hour)
	_, _ = m.SyncOnce(ctx)
	if len(spy.removed) != 1 {
		t.Fatalf("cleared twice %v", spy.removed)
	}
}

// Each failed try waits longer than the one before, up to a ceiling.
func TestClearRetryBacksOff(t *testing.T) {
	w, m, id, spy, now := clearRetryRig(t)
	ctx := context.Background()
	spy.err = context.DeadlineExceeded
	m.ClearFinished(ctx, id)
	var gaps []time.Duration
	for range 10 {
		at, _ := clearWait(t, w, id)
		next, err := time.Parse(time.RFC3339Nano, at.String)
		if err != nil {
			t.Fatal(err)
		}
		gaps = append(gaps, next.Sub(*now))
		*now = next
		_, _ = m.SyncOnce(ctx)
	}
	for i := 1; i < len(gaps); i++ {
		if gaps[i] < gaps[i-1] {
			t.Fatalf("waits shrink: %v", gaps)
		}
	}
	if gaps[0] > 5*time.Minute || gaps[1] <= gaps[0] || gaps[len(gaps)-1] > 2*time.Hour {
		t.Fatalf("waits %v", gaps)
	}
	if _, tries := clearWait(t, w, id); tries != 11 {
		t.Fatalf("tries %d", tries)
	}
}

// A real refusal is logged once, not on every retry, and is still retried.
func TestClearRetryLogsARefusalOnce(t *testing.T) {
	w, m, id, spy, now := clearRetryRig(t)
	ctx := context.Background()
	spy.err = errors.New("NZBGet said no")
	m.ClearFinished(ctx, id)
	for range 3 {
		*now = now.Add(3 * time.Hour)
		_, _ = m.SyncOnce(ctx)
	}
	if n := countEvents(t, w, "download_not_removed", id); n != 1 {
		t.Fatalf("%d failure entries", n)
	}
	if _, tries := clearWait(t, w, id); tries != 4 {
		t.Fatalf("tries %d", tries)
	}
}

// The wait ends when the job is gone from the Downloader.
func TestClearRetryEndsWhenTheJobIsGone(t *testing.T) {
	w, m, id, spy, now := clearRetryRig(t)
	ctx := context.Background()
	spy.err = outbound.ErrSuppressed
	m.ClearFinished(ctx, id)
	spy.err = ErrNotFound
	*now = now.Add(time.Hour)
	_, _ = m.SyncOnce(ctx)
	if at, _ := clearWait(t, w, id); at.Valid {
		t.Fatal("still waiting for a job the Downloader no longer has")
	}
}

// Turning the switch off, or the Downloader going away, ends the wait.
func TestClearRetryEndsWhenTheSwitchIsOff(t *testing.T) {
	w, m, id, spy, now := clearRetryRig(t)
	ctx := context.Background()
	spy.err = outbound.ErrSuppressed
	m.ClearFinished(ctx, id)
	spy.err = nil
	if err := execSQL(w, `UPDATE downloaders SET clear_finished = 0`); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(time.Hour)
	_, _ = m.SyncOnce(ctx)
	if len(spy.removed) != 0 {
		t.Fatalf("removed %v", spy.removed)
	}
	if at, _ := clearWait(t, w, id); at.Valid {
		t.Fatal("still waiting with the switch off")
	}
}

// The first try that fails for a known reason keeps that reason for the
// Downloads page.
func TestClearWaitsListsWaitingDownloads(t *testing.T) {
	w, m, id, spy, _ := clearRetryRig(t)
	ctx := context.Background()
	spy.err = outbound.ErrSuppressed
	m.ClearFinished(ctx, id)
	waits := ClearWaits(ctx, w.s.Read())
	cw, ok := waits[id]
	if !ok || cw.Tries != 1 || cw.NextTry.IsZero() || cw.Reason == "" {
		t.Fatalf("waits %+v", waits)
	}
}

// The Downloads view marks an imported download that is still waiting to be
// cleared, and only that one.
func TestDownloadsViewShowsAWaitingClear(t *testing.T) {
	t.Parallel()
	x := seed41(t, mediafiles.SlotInLibrary, "finished")
	pointQB(t, x, qbList(t, `[]`).URL)
	feedStore(t, x.dl)
	if v := view(t, x.srv.URL+"/api/v1/downloads/view"); len(v.Downloads) != 1 || v.Downloads[0].ClearWaiting != nil {
		t.Fatalf("before %+v", v.Downloads)
	}
	exec41(t, x.s, "UPDATE downloads SET clear_retry_at = '2026-11-11T03:05:00.000000000Z', clear_tries = 2, clear_error = 'outside service is retrying' WHERE id = ?", x.dlID)
	v := view(t, x.srv.URL+"/api/v1/downloads/view")
	cw := v.Downloads[0].ClearWaiting
	if cw == nil || cw.Tries != 2 || cw.Reason != "outside service is retrying" || !cw.NextTry.Equal(time.Date(2026, 11, 11, 3, 5, 0, 0, time.UTC)) {
		t.Fatalf("after %+v", cw)
	}
}

// A download row taken over by a new grab (same Downloader id) must never be
// cleared by a retry left over from the old download.
func TestClearRetryNeverRemovesANewDownloadOnAReusedRow(t *testing.T) {
	w, m, id, spy, now := clearRetryRig(t)
	ctx := context.Background()
	spy.err = outbound.ErrSuppressed
	m.ClearFinished(ctx, id)
	if at, _ := clearWait(t, w, id); !at.Valid {
		t.Fatal("not waiting to clear")
	}
	spy.err = nil
	// A new grab reuses the row: it is downloading again.
	if err := execSQL(w, `UPDATE downloads SET state = 'downloading'`); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(2 * time.Hour)
	m.retryClears(ctx)
	if len(spy.removed) != 0 {
		t.Fatalf("removed the new download %v", spy.removed)
	}
	if at, _ := clearWait(t, w, id); at.Valid {
		t.Fatal("old wait to clear kept on the reused row")
	}
}
