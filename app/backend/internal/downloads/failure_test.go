package downloads

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestDownloaderFailureTurnsDownRemovesAndSearches(t *testing.T) {
	w, m := setup(t)
	clientSaysFailed(m)
	id := grabbed(t, w, m)
	if n, _ := m.SyncOnce(context.Background()); n != 0 {
		t.Fatalf("still active: %d", n)
	}
	wantFailedPath(t, w, id, "the Downloader says", false)
	if len(w.fake.deleted) != 1 || w.fake.deleted[0] != hash+"|false" {
		t.Fatalf("a client-caused failure must leave the files: deleted %v", w.fake.deleted)
	}
	_, _ = m.SyncOnce(context.Background())
	if len(w.failedSlots) != 1 {
		t.Fatal("failed twice")
	}
}

func TestFailedKeptInTheDownloaderWhenSettingOff(t *testing.T) {
	w, m := setup(t)
	w.setting(t, SettingRemoveFailed, "false")
	id := grabbed(t, w, m)
	clientSaysFailed(m)
	_, _ = m.SyncOnce(context.Background())
	wantFailedPath(t, w, id, "the Downloader says", false)
	if len(w.fake.deleted) != 0 {
		t.Fatalf("kept in the Downloader with the setting off: deleted %v", w.fake.deleted)
	}
}

func TestDangerousFilesFailTheDownload(t *testing.T) {
	w, m := setup(t)
	letChange(t, m)
	id := grabbed(t, w, m)
	w.fake.set(hash, map[string]any{"progress": 1.0, "state": "stalledUP"})
	w.fake.files[hash] = []map[string]any{{"name": "Dune/Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS.mkv.exe", "size": 100}, {"name": "Dune/readme.txt", "size": 1}}
	_, _ = m.SyncOnce(context.Background())
	wantFailedPath(t, w, id, "dangerous files: Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS.mkv.exe", true)
	if len(w.finshed) != 0 {
		t.Fatal("a dangerous Download was imported")
	}
}

func TestDangerousFilesAllowedWhenSettingOff(t *testing.T) {
	w, m := setup(t)
	w.setting(t, SettingFailDangerous, "false")
	id := grabbed(t, w, m)
	w.fake.set(hash, map[string]any{"progress": 1.0, "state": "stalledUP"})
	w.fake.files[hash] = []map[string]any{{"name": "setup.exe", "size": 100}}
	_, _ = m.SyncOnce(context.Background())
	if d := w.download(t, id); d.State != "finished" {
		t.Fatalf("want finished with the setting off, got %s", d.State)
	}
}

func TestMarkFailedByHand(t *testing.T) {
	w, m := setup(t)
	letChange(t, m)
	id := grabbed(t, w, m)
	if err := m.MarkFailed(context.Background(), id, domain.Actor{Kind: "user", ID: 1}, "luis"); err != nil {
		t.Fatal(err)
	}
	wantFailedPath(t, w, id, "marked failed by luis", true)
	if err := m.MarkFailed(context.Background(), id, domain.Actor{Kind: "user", ID: 1}, "luis"); !errors.Is(err, ErrNotActive) {
		t.Fatalf("second mark: %v", err)
	}
	var n int
	_ = w.s.ReadEvents().QueryRow("SELECT count(*) FROM events WHERE kind = 'download_failed' AND download_id = ?", id).Scan(&n)
	if n != 1 {
		t.Fatalf("%d failure events", n)
	}
}

func TestMarkFailedWhileStalled(t *testing.T) {
	w, m := setup(t)
	letChange(t, m)
	id := grabbed(t, w, m)
	w.clock = w.clock.Add(11 * time.Minute)
	_, _ = m.SyncOnce(context.Background())
	if w.slot(t).State != mediafiles.SlotStalled {
		t.Fatalf("slot %s", w.slot(t).State)
	}
	if err := m.MarkFailed(context.Background(), id, domain.System, ""); err != nil {
		t.Fatal(err)
	}
	wantFailedPath(t, w, id, "marked failed", true)
}

func TestStalledThenActedOn(t *testing.T) {
	w, m := setup(t)
	letChange(t, m)
	m.StallAfter, m.ActAfter = 0, 0
	id := grabbed(t, w, m)
	ctx := context.Background()
	w.clock = w.clock.Add(5*time.Hour + 59*time.Minute)
	_, _ = m.SyncOnce(ctx)
	if d := w.download(t, id); d.State != "downloading" {
		t.Fatalf("stalled too early: %s", d.State)
	}
	w.clock = w.clock.Add(time.Minute)
	_, _ = m.SyncOnce(ctx)
	if d := w.download(t, id); d.State != "stalled" || *d.StateReason != "no progress for 6 hours" {
		t.Fatalf("want stalled after 6 hours, got %s %v", d.State, d.StateReason)
	}
	m2 := &Manager{Store: w.s, Now: w.now, OnFailed: m.OnFailed, Sleep: m.Sleep}
	w.clock = w.clock.Add(17*time.Hour + 59*time.Minute)
	_, _ = m2.SyncOnce(ctx)
	if d := w.download(t, id); d.State != "stalled" {
		t.Fatalf("acted too early: %s", d.State)
	}
	w.clock = w.clock.Add(time.Minute)
	_, _ = m2.SyncOnce(ctx)
	wantFailedPath(t, w, id, "no progress for 24 hours", true)
}

func TestStallTimesComeFromSettings(t *testing.T) {
	w, m := setup(t)
	m.StallAfter, m.ActAfter = 0, 0
	w.setting(t, SettingStallAfter, "1")
	w.setting(t, SettingActAfter, "2")
	id := grabbed(t, w, m)
	w.clock = w.clock.Add(time.Hour)
	_, _ = m.SyncOnce(context.Background())
	if d := w.download(t, id); d.State != "stalled" {
		t.Fatalf("want stalled after 1 hour, got %s", d.State)
	}
	w.clock = w.clock.Add(time.Hour)
	_, _ = m.SyncOnce(context.Background())
	if d := w.download(t, id); d.State != "failed" {
		t.Fatalf("want failed after 2 hours, got %s", d.State)
	}
}

func TestQueuedPausedAndFullProgressNeverStall(t *testing.T) {
	w, m := setup(t)
	id := grabbed(t, w, m)
	ctx := context.Background()
	for _, c := range []struct {
		state    string
		progress float64
		want     string
	}{{"queuedDL", 0.2, "queued"}, {"pausedDL", 0.2, "paused"}, {"downloading", 1.0, "downloading"}} {
		w.fake.set(hash, map[string]any{"state": c.state, "progress": c.progress})
		w.clock = w.clock.Add(48 * time.Hour)
		_, _ = m.SyncOnce(ctx)
		if d := w.download(t, id); d.State != c.want {
			t.Fatalf("%s at %.1f: got %s, want %s", c.state, c.progress, d.State, c.want)
		}
	}
}

func TestTimeWaitingOrPausedNeverCounts(t *testing.T) {
	w, m := setup(t)
	id := grabbed(t, w, m)
	ctx := context.Background()
	w.fake.set(hash, map[string]any{"state": "pausedDL"})
	_, _ = m.SyncOnce(ctx)
	w.clock = w.clock.Add(48 * time.Hour)
	_, _ = m.SyncOnce(ctx)
	w.fake.set(hash, map[string]any{"state": "downloading"})
	_, _ = m.SyncOnce(ctx)
	if d := w.download(t, id); d.State != "downloading" {
		t.Fatalf("after a long pause: %s", d.State)
	}
	w.clock = w.clock.Add(11 * time.Minute)
	_, _ = m.SyncOnce(ctx)
	if d := w.download(t, id); d.State != "stalled" {
		t.Fatalf("no progress after resuming: %s", d.State)
	}
}

func TestProgressGoingBackDoesNotResetTheTimer(t *testing.T) {
	w, m := setup(t)
	id := grabbed(t, w, m)
	before := w.download(t, id).ProgressAt
	w.clock = w.clock.Add(5 * time.Minute)
	w.fake.set(hash, map[string]any{"progress": 0.1})
	_, _ = m.SyncOnce(context.Background())
	d := w.download(t, id)
	if d.Progress != 0.2 || !d.ProgressAt.Equal(*before) {
		t.Fatalf("progress %.2f at %v (was %v)", d.Progress, d.ProgressAt, before)
	}
}

func TestVanishedIsReplaced(t *testing.T) {
	w, m := setup(t)
	id := grabbed(t, w, m)
	w.fake.drop(hash)
	for range missedSyncs {
		w.advance(missWindow)
		_, _ = m.SyncOnce(context.Background())
	}
	d := w.download(t, id)
	if d.State != "vanished" || !strings.Contains(*d.StateReason, "removed outside HolyArrs") {
		t.Fatalf("download %s %v", d.State, d.StateReason)
	}
	if w.release(t).State != "turned_down" || w.slot(t).State != mediafiles.SlotFailed || len(w.failedSlots) != 1 || len(w.fake.deleted) != 0 {
		t.Fatalf("release %s slot %s searched %v deleted %v", w.release(t).State, w.slot(t).State, w.failedSlots, w.fake.deleted)
	}
}

func TestVanishedNeedsADecisionWhenSettingOff(t *testing.T) {
	w, m := setup(t)
	w.setting(t, SettingReplaceVanished, "false")
	id := grabbed(t, w, m)
	w.fake.drop(hash)
	for range missedSyncs {
		w.advance(missWindow)
		_, _ = m.SyncOnce(context.Background())
	}
	if d := w.download(t, id); d.State != "vanished" {
		t.Fatalf("download %s", d.State)
	}
	if w.release(t).State == "turned_down" || w.slot(t).State != mediafiles.SlotNeedsADecision || len(w.failedSlots) != 0 {
		t.Fatalf("release %s slot %s searched %v", w.release(t).State, w.slot(t).State, w.failedSlots)
	}
}

func TestReadFinishedFolderAndFile(t *testing.T) {
	dir := t.TempDir()
	job := filepath.Join(dir, "Job.Name")
	if err := os.MkdirAll(filepath.Join(job, "Sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(job, "movie.mkv"), make([]byte, 10), 0o644)
	_ = os.WriteFile(filepath.Join(job, "Sub", "movie.srt"), make([]byte, 3), 0o644)
	pm := []domain.PathMap{{Theirs: "/dl", Ours: dir}}
	save, files := readFinished(Job{ContentPath: "/dl/Job.Name"}, pm, "")
	if save != "/dl" || len(files) != 2 || files[0].Name != "Job.Name/Sub/movie.srt" && files[1].Name != "Job.Name/Sub/movie.srt" {
		t.Fatalf("folder: %s %+v", save, files)
	}
	save, files = readFinished(Job{ContentPath: "/dl/Job.Name/movie.mkv"}, pm, "")
	if save != "/dl/Job.Name" || len(files) != 1 || files[0].Name != "movie.mkv" || files[0].Size != 10 {
		t.Fatalf("file: %s %+v", save, files)
	}
	save, files = readFinished(Job{ContentPath: "/far/Away/x.mkv", Size: 7}, nil, "")
	if save != "/far/Away" || len(files) != 1 || files[0].Size != 7 {
		t.Fatalf("unreadable: %s %+v", save, files)
	}
}

func TestDiscardAfterRejectedImport(t *testing.T) {
	w, m := setup(t)
	letChange(t, m)
	id := grabbed(t, w, m)
	if err := m.Discard(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if len(w.fake.deleted) != 1 || w.fake.deleted[0] != hash+"|true" {
		t.Fatalf("deleted %v", w.fake.deleted)
	}
	w.setting(t, SettingRemoveFailed, "false")
	_ = m.Discard(context.Background(), id)
	if len(w.fake.deleted) != 1 {
		t.Fatal("removed with the setting off")
	}
}

func (w *world) release(t *testing.T) domain.Release {
	t.Helper()
	var r domain.Release
	if err := database.Get(context.Background(), w.s.Read(), "releases", w.relID, &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func (w *world) setting(t *testing.T, key, value string) {
	t.Helper()
	if err := w.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE settings SET value = ? WHERE grp = 'links' AND key = ?", value, key)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func grabbed(t *testing.T, w *world, m *Manager) int64 {
	t.Helper()
	id, err := m.Grab(context.Background(), w.slotID, w.relID, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	w.fake.set(hash, map[string]any{"name": "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS", "category": "holyarrs", "progress": 0.2, "state": "downloading"})
	if _, err := m.SyncOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	return id
}

func wantFailedPath(t *testing.T, w *world, id int64, reason string, removed bool) {
	t.Helper()
	d := w.download(t, id)
	if d.State != "failed" || d.StateReason == nil || !strings.Contains(*d.StateReason, reason) {
		t.Fatalf("download %s %v, want failed with %q", d.State, d.StateReason, reason)
	}
	if r := w.release(t); r.State != "turned_down" || !strings.Contains(r.Verdict["turned_down_reason"].(string), reason) {
		t.Fatalf("release %s %v", r.State, r.Verdict)
	}
	if sl := w.slot(t); sl.State != mediafiles.SlotFailed {
		t.Fatalf("slot %s", sl.State)
	}
	if len(w.failedSlots) != 1 || w.failedSlots[0] != w.slotID {
		t.Fatalf("searched again: %v", w.failedSlots)
	}
	gone := len(w.fake.deleted) == 1 && w.fake.deleted[0] == hash+"|true"
	if gone != removed {
		t.Fatalf("removed from the Downloader: %v (deleted %v)", gone, w.fake.deleted)
	}
}

func letChange(t *testing.T, m *Manager) {
	t.Helper()
	if err := m.Store.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE downloaders SET writable = 1")
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
