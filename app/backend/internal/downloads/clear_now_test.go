package downloads

import (
	"context"
	"testing"
)

// Clear now tries every waiting clear at once instead of at its next try.
func TestClearNowTriesEveryWaitingClear(t *testing.T) {
	w, m, id, spy, _ := clearRetryRig(t)
	ctx := context.Background()
	spy.err = context.DeadlineExceeded
	m.ClearFinished(ctx, id)
	if at, _ := clearWait(t, w, id); !at.Valid {
		t.Fatal("not waiting to clear")
	}
	spy.err = nil
	before := len(spy.removed)
	if left := m.ClearNow(ctx); left != 0 {
		t.Fatalf("%d still waiting", left)
	}
	if len(spy.removed) != before+1 {
		t.Fatalf("removed %v", spy.removed)
	}
	if at, _ := clearWait(t, w, id); at.Valid {
		t.Fatal("still waiting after Clear now")
	}
}

// A clear that fails again keeps waiting and is counted.
func TestClearNowCountsWhatStillWaits(t *testing.T) {
	_, m, id, spy, _ := clearRetryRig(t)
	ctx := context.Background()
	spy.err = context.DeadlineExceeded
	m.ClearFinished(ctx, id)
	if left := m.ClearNow(ctx); left != 1 {
		t.Fatalf("%d still waiting", left)
	}
}
