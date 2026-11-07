package jobs

import (
	"sync/atomic"
	"testing"
	"time"
)

// Wait also waits for guarded one-off work started with GoGuard and for
// the waiters added with AlsoWait, so the store is not closed under them.
func TestWaitCoversGuardedWork(t *testing.T) {
	r := quiet(NewRegistry())
	var done, extra atomic.Bool
	r.GoGuard("health", func() { time.Sleep(50 * time.Millisecond); done.Store(true) })
	r.AlsoWait(func() { time.Sleep(20 * time.Millisecond); extra.Store(true) })
	r.Wait()
	if !done.Load() || !extra.Load() {
		t.Fatalf("Wait returned early: guarded %v, extra %v", done.Load(), extra.Load())
	}
}
