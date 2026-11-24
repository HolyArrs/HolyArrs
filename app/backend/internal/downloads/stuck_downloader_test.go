package downloads

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestDownloaderStuckRule(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	old := now.Add(-7 * time.Hour)
	fresh := now.Add(-time.Hour)
	row := func(id, state string, p float64, at *time.Time) domain.Download {
		return domain.Download{ExternalID: id, State: state, Progress: p, ProgressAt: at}
	}
	job := func(state string, p float64) Job { return Job{State: state, Progress: p} }
	for _, c := range []struct {
		name string
		list []domain.Download
		have map[string]Job
		want bool
	}{
		{"two quiet", []domain.Download{row("a", "stalled", .2, &old), row("b", "downloading", .3, &old)},
			map[string]Job{"a": job(StateDownloading, .2), "b": job(StateDownloading, .3)}, true},
		{"one quiet is a torrent, not the connection", []domain.Download{row("a", "stalled", .2, &old)},
			map[string]Job{"a": job(StateDownloading, .2)}, false},
		{"one moved recently", []domain.Download{row("a", "stalled", .2, &old), row("b", "downloading", .3, &fresh)},
			map[string]Job{"a": job(StateDownloading, .2), "b": job(StateDownloading, .3)}, false},
		{"one moves now", []domain.Download{row("a", "stalled", .2, &old), row("b", "stalled", .3, &old)},
			map[string]Job{"a": job(StateDownloading, .2), "b": job(StateDownloading, .31)}, false},
		{"waiting and paused don't count", []domain.Download{row("a", "stalled", .2, &old), row("b", "stalled", .3, &old), row("c", "queued", 0, &fresh), row("d", "paused", 0, &fresh)},
			map[string]Job{"a": job(StateDownloading, .2), "b": job(StateDownloading, .3), "c": job(StateQueued, 0), "d": job(StatePaused, 0)}, true},
		{"complete rows don't count", []domain.Download{row("a", "stalled", .2, &old), row("b", "downloading", 1, &fresh)},
			map[string]Job{"a": job(StateDownloading, .2), "b": job(StateDownloading, 1)}, false},
		{"unlisted rows don't count", []domain.Download{row("a", "stalled", .2, &old), row("b", "stalled", .3, &old), row("c", "downloading", .1, &fresh)},
			map[string]Job{"a": job(StateDownloading, .2), "b": job(StateDownloading, .3)}, true},
		{"no progress time yet", []domain.Download{row("a", "stalled", .2, &old), row("b", "downloading", .3, nil)},
			map[string]Job{"a": job(StateDownloading, .2), "b": job(StateDownloading, .3)}, false},
		{"a fresh grab that never started proves nothing", []domain.Download{row("a", "stalled", .2, &old), row("b", "stalled", .3, &old), row("c", "downloading", 0, &fresh)},
			map[string]Job{"a": job(StateDownloading, .2), "b": job(StateDownloading, .3), "c": job(StateDownloading, 0)}, true},
		{"a fresh grab that starts moving does", []domain.Download{row("a", "stalled", .2, &old), row("b", "stalled", .3, &old), row("c", "downloading", 0, &fresh)},
			map[string]Job{"a": job(StateDownloading, .2), "b": job(StateDownloading, .3), "c": job(StateDownloading, .01)}, false},
		{"empty", nil, nil, false},
	} {
		if got := downloaderStuck(c.list, c.have, 6*time.Hour, now); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

// Two downloads on one Downloader stop together (a VPN that died quietly):
// neither fails while it lasts, health warns, and the clocks run on once one
// of them moves again.
func TestStuckDownloaderHoldsTheClocks(t *testing.T) {
	w, m := setup(t)
	letChange(t, m)
	m.StallAfter, m.ActAfter = 0, 0
	id := grabbed(t, w, m)
	ctx := context.Background()
	const hash2 = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	var id2 int64
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		d := w.download(t, id)
		d.ID, d.ExternalID, d.ReleaseID, d.SlotIDs = 0, hash2, nil, []int64{}
		_, err := database.Insert(tx, "downloads", &d, domain.System)
		id2 = d.ID
		return err
	}); err != nil {
		t.Fatal(err)
	}
	w.fake.set(hash2, map[string]any{"name": "Other.Movie.2020.1080p", "category": "holyarrs", "progress": 0.3, "state": "stalledDL"})
	_, _ = m.SyncOnce(ctx)

	w.clock = w.clock.Add(7 * time.Hour)
	_, _ = m.SyncOnce(ctx)
	for _, x := range []int64{id, id2} {
		if d := w.download(t, x); d.State != "stalled" || d.StateReason == nil || *d.StateReason != StuckReason {
			t.Fatalf("download %d: %s %v, want held as stuck", x, d.State, d.StateReason)
		}
	}
	for range 30 {
		w.clock = w.clock.Add(2 * time.Hour)
		_, _ = m.SyncOnce(ctx)
	}
	if d := w.download(t, id); d.State != "stalled" || len(w.fake.deleted) != 0 || w.release(t).State == "turned_down" {
		t.Fatalf("failed while the whole Downloader was stuck: %s, deleted %v", d.State, w.fake.deleted)
	}

	// The connection comes back: the second one moves, the first one's clock runs
	// on from the stall mark, so it fails only after the rest of its wait.
	w.fake.set(hash2, map[string]any{"name": "Other.Movie.2020.1080p", "category": "holyarrs", "progress": 0.5, "state": "downloading"})
	_, _ = m.SyncOnce(ctx)
	if d := w.download(t, id2); d.State != "downloading" {
		t.Fatalf("moved one: %s", d.State)
	}
	if d := w.download(t, id); d.State != "stalled" || !strings.HasPrefix(*d.StateReason, "no progress for") {
		t.Fatalf("first after the return: %s %v", d.State, d.StateReason)
	}
	// The second keeps moving, so this is one torrent's own stall again.
	for i := range 17 {
		w.clock = w.clock.Add(time.Hour)
		w.fake.set(hash2, map[string]any{"name": "Other.Movie.2020.1080p", "category": "holyarrs", "progress": 0.5 + float64(i+1)/100, "state": "downloading"})
		_, _ = m.SyncOnce(ctx)
	}
	if d := w.download(t, id); d.State != "stalled" {
		t.Fatalf("failed before its wait ran out: %s", d.State)
	}
	w.clock = w.clock.Add(2 * time.Hour)
	_, _ = m.SyncOnce(ctx)
	wantStallRetry(t, w, id, "no progress for")
}
