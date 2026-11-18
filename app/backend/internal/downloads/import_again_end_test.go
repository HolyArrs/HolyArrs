package downloads

import (
	"context"
	"sync"
	"testing"
	"time"
)

// endRec keeps every "slot" event and whether the read still ran when it came.
type endRec struct {
	mu      sync.Mutex
	m       *Manager
	id      int64
	reading []bool
}

func (r *endRec) Publish(topic string, _ any) uint64 {
	if topic == "slot" {
		still := r.m.Rereading(r.id)
		r.mu.Lock()
		r.reading = append(r.reading, still)
		r.mu.Unlock()
	}
	return 0
}

// A read that ends after the click, here too slow, tells the page to fetch the card again, and that fetch no longer sees the read.
func TestImportAgainSlowReadEndTellsThePage(t *testing.T) {
	_, m, _, id := slowFolder(t, 20*time.Millisecond, 150*time.Millisecond)
	rec := &endRec{m: m, id: id}
	m.Live = rec
	if err := m.ImportAgain(context.Background(), id, luis); err != nil {
		t.Fatal(err)
	}
	readDone(t, m)
	for end := time.Now().Add(10 * time.Second); time.Now().Before(end); time.Sleep(10 * time.Millisecond) {
		rec.mu.Lock()
		got := append([]bool(nil), rec.reading...)
		rec.mu.Unlock()
		if len(got) > 0 && !got[len(got)-1] {
			return
		}
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	t.Fatalf("no slot event after the read ended, or the read still showed: %v", rec.reading)
}
