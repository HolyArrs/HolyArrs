package downloads

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// slowEngine answers Get once release is closed or the read's time is up, counting the asks.
type slowEngine struct {
	Engine
	job     Job
	release chan struct{}
	asked   atomic.Int32
}

func (e *slowEngine) Get(ctx context.Context, _ string) (Job, error) {
	e.asked.Add(1)
	select {
	case <-e.release:
	case <-ctx.Done():
	}
	return e.job, nil
}

// slowFolder is an unseen job whose folder is there now and healthy, read by a slow Downloader.
func slowFolder(t *testing.T, wait, limit time.Duration) (*world, *Manager, *slowEngine, int64) {
	t.Helper()
	w, m, _, id, root := unseenJob(t)
	job := filepath.Join(root, "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS")
	if err := os.MkdirAll(job, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(job, "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS.mkv"), make([]byte, 2048), 0o644); err != nil {
		t.Fatal(err)
	}
	e := &slowEngine{job: Job{ID: hash, State: StateFinished, SavePath: root, ContentPath: job}, release: make(chan struct{})}
	useEngine(m, e)
	oldWait, oldLimit := rereadWait, rereadTimeout
	rereadWait, rereadTimeout = wait, limit
	t.Cleanup(func() { rereadWait, rereadTimeout = oldWait, oldLimit })
	return w, m, e, id
}

// readDone waits until no folder read of Import again runs.
func readDone(t *testing.T, m *Manager) {
	t.Helper()
	for end := time.Now().Add(30 * time.Second); time.Now().Before(end); time.Sleep(10 * time.Millisecond) {
		m.mu.Lock()
		n := len(m.rereading)
		m.mu.Unlock()
		if n == 0 {
			return
		}
	}
	t.Fatal("the folder read did not end")
}

// A read slower than the click waits goes on after the click and places the movie when it ends.
func TestImportAgainSlowReadFinishesAfterTheClick(t *testing.T) {
	w, m, e, id := slowFolder(t, 20*time.Millisecond, time.Minute)
	if err := m.ImportAgain(context.Background(), id, luis); err != nil {
		t.Fatal(err)
	}
	close(e.release)
	readDone(t, m)
	if sl := w.slot(t); sl.State != mediafiles.SlotInLibrary {
		t.Fatalf("slot %s reason %v", sl.State, sl.LastReason)
	}
}

// A read still cut short by its limit leaves a card that says the read took too long, and Import again stays on offer.
func TestImportAgainReadTooSlowSaysSo(t *testing.T) {
	w, m, _, id := slowFolder(t, 20*time.Millisecond, 100*time.Millisecond)
	if err := m.ImportAgain(context.Background(), id, luis); err != nil {
		t.Fatal(err)
	}
	readDone(t, m)
	sl := w.slot(t)
	if sl.State != mediafiles.SlotNeedsADecision || sl.LastReason == nil || *sl.LastReason != importer.SlowReadWhy || !importer.CanImportAgain(*sl.LastReason) {
		t.Fatalf("slot %s reason %v", sl.State, sl.LastReason)
	}
	if d := w.download(t, id); len(d.FilesSeen) != 0 {
		t.Fatalf("files %v", d.FilesSeen)
	}
}

// A second click while the folder is read starts no second read.
func TestImportAgainTwoClicksReadOnce(t *testing.T) {
	w, m, e, id := slowFolder(t, 20*time.Millisecond, time.Minute)
	for range 2 {
		if err := m.ImportAgain(context.Background(), id, luis); err != nil {
			t.Fatal(err)
		}
	}
	close(e.release)
	readDone(t, m)
	if n := e.asked.Load(); n != 1 {
		t.Fatalf("the Downloader was asked %d times", n)
	}
	if sl := w.slot(t); sl.State != mediafiles.SlotInLibrary {
		t.Fatalf("slot %s reason %v", sl.State, sl.LastReason)
	}
}

// On a mount stuck inside the walk, pressing again starts no second walk and says the read took too long.
func TestImportAgainStuckWalkNoPileUp(t *testing.T) {
	w, m, e, id := slowFolder(t, 20*time.Millisecond, 100*time.Millisecond)
	close(e.release)
	unblock := make(chan struct{})
	var walks atomic.Int32
	old := rereadFolder
	rereadFolder = func(_ context.Context, j Job, _ []domain.PathMap, _ string) (string, []File) {
		walks.Add(1)
		<-unblock
		return j.SavePath, nil
	}
	t.Cleanup(func() { close(unblock); rereadFolder = old })
	for range 5 {
		_ = m.ImportAgain(context.Background(), id, luis)
		readDone(t, m)
		sl := w.slot(t)
		if sl.State != mediafiles.SlotNeedsADecision || sl.LastReason == nil || *sl.LastReason != importer.SlowReadWhy {
			t.Fatalf("slot %s reason %v", sl.State, sl.LastReason)
		}
	}
	if n := walks.Load(); n > 1 {
		t.Fatalf("%d walks stuck after 5 presses", n)
	}
}
