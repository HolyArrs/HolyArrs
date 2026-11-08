package mediaservers

import (
	"context"
	"testing"
	"time"
)

// Stop sends a gathered batch now instead of after the delay, so a
// shutdown waits for it briefly and never after the store is closed.
func TestStopSendsTheBatchNow(t *testing.T) {
	w := newWorld(t)
	l := &Linker{Store: w.s, Vault: w.v, Delay: time.Hour}
	l.Queue(context.Background(), "/movies/a", Modified)
	start := time.Now()
	l.Stop()
	l.Wait()
	if time.Since(start) > 5*time.Second {
		t.Fatalf("Stop waited %s", time.Since(start))
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.pending != nil || l.timer != nil {
		t.Fatalf("batch left: %v", l.pending)
	}
}
