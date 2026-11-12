package downloads

import (
	"context"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// deadDownload is a grabbed torrent still downloading, read from a stand-in
// client that reports j.
func deadDownload(t *testing.T, j Job, rules ...string) (*world, *Manager, int64, *seedEngine) {
	t.Helper()
	w, m := setup(t)
	id := grabbed(t, w, m)
	for _, q := range rules {
		if err := execSQL(w, q); err != nil {
			t.Fatal(err)
		}
	}
	j.ID, j.Category = hash, "holyarrs"
	if j.State == "" {
		j.State = StateDownloading
	}
	eng := &seedEngine{jobs: []Job{j}}
	m.Engine = func(domain.Downloader) (Engine, error) { return eng, nil }
	m.Forget(w.download(t, id).DownloaderID)
	return w, m, id, eng
}

func setRule(key, value string) string {
	return `INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by)
		VALUES ('links', '` + key + `', '` + value + `', '2026-01-01T00:00:00Z', '{}', '2026-01-01T00:00:00Z', '{}')`
}

func TestDeadRuleOffByDefault(t *testing.T) {
	w, m, id, _ := deadDownload(t, Job{Detail: "metaDL"})
	w.advance(48 * time.Hour)
	m.deadRule(context.Background())
	if d := w.download(t, id); d.State == "failed" {
		t.Fatalf("failed with the rule off: %v", d.StateReason)
	}
}

func TestDeadRuleStuckOnMetadata(t *testing.T) {
	w, m, id, _ := deadDownload(t, Job{Detail: "metaDL"}, setRule(SettingDeadMetadata, "30"))
	w.advance(29 * time.Minute)
	m.deadRule(context.Background())
	if d := w.download(t, id); d.State == "failed" {
		t.Fatal("failed too early")
	}
	w.advance(2 * time.Minute)
	m.deadRule(context.Background())
	wantFailedPath(t, w, id, "metadata", false)
}

func TestDeadRuleMetadataNeedsNoMetadata(t *testing.T) {
	w, m, id, _ := deadDownload(t, Job{Detail: "stalledDL", Size: 1000, Files: []File{{Name: "a.mkv", Size: 1000}}}, setRule(SettingDeadMetadata, "30"))
	w.advance(2 * time.Hour)
	m.deadRule(context.Background())
	if d := w.download(t, id); d.State == "failed" {
		t.Fatalf("failed with metadata known: %v", d.StateReason)
	}
}

func TestDeadRuleCrawling(t *testing.T) {
	const size = 10 << 30
	w, m, id, eng := deadDownload(t, Job{Detail: "downloading", Size: size, Progress: 0.1},
		setRule(SettingCrawlKBps, "100"), setRule(SettingCrawlHours, "1"))
	m.deadRule(context.Background()) // first look: the window starts
	// 50 KB/s for an hour.
	w.advance(time.Hour)
	eng.jobs[0].Progress = 0.1 + float64(50*1024*3600)/size
	m.deadRule(context.Background())
	wantFailedPath(t, w, id, "slower than 100 KB/s", false)
}

func TestDeadRuleCrawlingFastEnoughResets(t *testing.T) {
	const size = 10 << 30
	w, m, id, eng := deadDownload(t, Job{Detail: "downloading", Size: size, Progress: 0.1},
		setRule(SettingCrawlKBps, "100"), setRule(SettingCrawlHours, "1"))
	m.deadRule(context.Background())
	w.advance(time.Hour)
	eng.jobs[0].Progress = 0.1 + float64(500*1024*3600)/size
	m.deadRule(context.Background())
	// Paused time never counts.
	eng.jobs[0].State, eng.jobs[0].Detail = StatePaused, "pausedDL"
	w.advance(5 * time.Hour)
	m.deadRule(context.Background())
	if d := w.download(t, id); d.State == "failed" {
		t.Fatalf("failed: %v", d.StateReason)
	}
}

func TestDeadRuleMetadataPausedTimeNeverCounts(t *testing.T) {
	w, m, id, eng := deadDownload(t, Job{Detail: "metaDL"}, setRule(SettingDeadMetadata, "30"))
	// Paused by hand for two hours, then started again.
	eng.jobs[0].State, eng.jobs[0].Detail = StatePaused, "pausedDL"
	if _, err := m.SyncOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	w.advance(2 * time.Hour)
	eng.jobs[0].State, eng.jobs[0].Detail = StateDownloading, "metaDL"
	if _, err := m.SyncOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	m.deadRule(context.Background())
	if d := w.download(t, id); d.State == "failed" {
		t.Fatalf("failed as soon as it started again: %v", *d.StateReason)
	}
	w.advance(31 * time.Minute)
	m.deadRule(context.Background())
	wantFailedPath(t, w, id, "metadata", false)
}
