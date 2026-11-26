package downloads

import (
	"context"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A search on a title whose finished download failed to import retries that
// import first and leaves the Slot out of the search.
func TestRetryImportFirstRetriesTheFinishedDownload(t *testing.T) {
	w, m := setup(t)
	id := grabbed(t, w, m)
	w.exec(t, "UPDATE downloads SET state = 'finished', progress = 1 WHERE id = ?", id)
	w.exec(t, "UPDATE slots SET state = 'needs_a_decision', last_reason = ?, active_download_id = ? WHERE id = ?", importer.SlowReadWhy, id, w.slotID)
	got := m.RetryImportFirst(context.Background(), []int64{w.slotID}, luis)
	if !got[w.slotID] {
		t.Fatalf("retried %v, want slot %d", got, w.slotID)
	}
	if sl := w.slot(t); sl.State != mediafiles.SlotImporting {
		t.Fatalf("slot %s", sl.State)
	}
	if len(w.finshed) != 1 || w.finshed[0] != id {
		t.Fatalf("import asked for %v", w.finshed)
	}
}

// A refusal Import again can't clear, or a download that is gone, leaves the
// Slot to the search.
func TestRetryImportFirstLeavesOthersToTheSearch(t *testing.T) {
	w, m := setup(t)
	id := grabbed(t, w, m)
	w.exec(t, "UPDATE downloads SET state = 'finished', progress = 1 WHERE id = ?", id)
	w.exec(t, "UPDATE slots SET state = 'needs_a_decision', last_reason = 'a different episode', active_download_id = ? WHERE id = ?", id, w.slotID)
	if got := m.RetryImportFirst(context.Background(), []int64{w.slotID}, luis); len(got) != 0 {
		t.Fatalf("retried %v for a refusal", got)
	}
	w.exec(t, "UPDATE slots SET last_reason = ? WHERE id = ?", importer.SlowReadWhy, w.slotID)
	w.exec(t, "UPDATE downloads SET state = 'vanished' WHERE id = ?", id)
	if got := m.RetryImportFirst(context.Background(), []int64{w.slotID}, luis); len(got) != 0 {
		t.Fatalf("retried %v for a download that is gone", got)
	}
	if len(w.finshed) != 0 {
		t.Fatalf("import asked for %v", w.finshed)
	}
}
