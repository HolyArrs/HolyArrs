package downloader

import (
	"context"
	"reflect"
	"sync"
	"testing"

	"github.com/CoderLuii/holyarrs/internal/domain"
)

type countingEngine struct {
	Engine
	mu          sync.Mutex
	lists, gets int
	last        []Job
}

func (c *countingEngine) List(ctx context.Context, category string) ([]Job, error) {
	js, err := c.Engine.List(ctx, category)
	c.mu.Lock()
	c.lists++
	c.last = js
	c.mu.Unlock()
	return js, err
}

func (c *countingEngine) Get(ctx context.Context, id string) (Job, error) {
	c.mu.Lock()
	c.gets++
	c.mu.Unlock()
	return c.Engine.Get(ctx, id)
}

// The shadow hook gets exactly the list SyncOnce read, and SyncOnce makes
// the same client calls with the hook as without it.
func TestSyncOnceOnListAddsNoClientCall(t *testing.T) {
	run := func(hook bool) (*countingEngine, [][]Job, []uint64) {
		w, m := setup(t)
		ce := &countingEngine{}
		m.Engine = func(d domain.Downloader) (Engine, error) {
			e, err := (&Manager{}).EngineFor(context.Background(), d)
			ce.Engine = e
			return ce, err
		}
		var seen [][]Job
		var gens []uint64
		if hook {
			m.OnList = func(_ int64, gen uint64, category string, jobs []Job, err error) {
				if err != nil || category != "holyarrs" {
					t.Errorf("hook got %q %v", category, err)
				}
				seen = append(seen, jobs)
				gens = append(gens, gen)
				if !reflect.DeepEqual(jobs, ce.last) {
					t.Errorf("hook list differs from the list SyncOnce read")
				}
			}
		}
		ctx := context.Background()
		if _, err := m.Grab(ctx, w.slotID, w.relID, domain.System); err != nil {
			t.Fatal(err)
		}
		w.fake.set(hash, map[string]any{"name": "Dune", "category": "holyarrs", "progress": 0.4, "state": "downloading"})
		_, _ = m.SyncOnce(ctx)
		m.Forget(1)
		w.fake.set(hash, map[string]any{"progress": 1.0, "state": "stalledUP"})
		_, _ = m.SyncOnce(ctx)
		return ce, seen, gens
	}
	legacy, _, _ := run(false)
	shadow, seen, gens := run(true)
	if legacy.lists != shadow.lists || legacy.gets != shadow.gets {
		t.Fatalf("calls: legacy %d lists %d gets, shadow %d lists %d gets", legacy.lists, legacy.gets, shadow.lists, shadow.gets)
	}
	if len(seen) != shadow.lists || len(seen) == 0 {
		t.Fatalf("hook ran %d times for %d lists", len(seen), shadow.lists)
	}
	if gens[0] == gens[len(gens)-1] {
		t.Fatalf("generation did not move after Forget: %v", gens)
	}
}
