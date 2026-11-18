package downloads

import (
	"context"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// A folder read stuck inside one file-system call still ends at its limit, and its late answer changes nothing.
func TestImportAgainHungReadGivesUp(t *testing.T) {
	w, m, e, id := slowFolder(t, 20*time.Millisecond, 100*time.Millisecond)
	close(e.release)
	unblock, late := make(chan struct{}), make(chan struct{})
	old := rereadFolder
	rereadFolder = func(_ context.Context, j Job, _ []domain.PathMap, _ string) (string, []File) {
		<-unblock
		defer close(late)
		return j.SavePath, []File{{Name: "Dune.Part.Two.2024.1080p.WEB.H264-HOLYARRS.mkv", Size: 2048, Progress: 1}}
	}
	t.Cleanup(func() { rereadFolder = old })
	if err := m.ImportAgain(context.Background(), id, luis); err != nil {
		t.Fatal(err)
	}
	readDone(t, m)
	sl := w.slot(t)
	if sl.State != mediafiles.SlotNeedsADecision || sl.LastReason == nil || *sl.LastReason != importer.SlowReadWhy {
		t.Fatalf("slot %s reason %v", sl.State, sl.LastReason)
	}
	close(unblock)
	<-late
	time.Sleep(50 * time.Millisecond)
	if d := w.download(t, id); len(d.FilesSeen) != 0 {
		t.Fatalf("a late read stored files %v", d.FilesSeen)
	}
	if sl := w.slot(t); sl.State != mediafiles.SlotNeedsADecision {
		t.Fatalf("slot %s after the late read", sl.State)
	}
}
