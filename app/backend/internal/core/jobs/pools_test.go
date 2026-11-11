package jobs

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// Each pool keeps working while every worker of another pool hangs: two hung
// searches do not delay a health check; hung imports or health checks do not
// delay searches or the other pools.
func TestStallInOnePoolLeavesTheOthers(t *testing.T) {
	kindOf := map[Pool]string{PoolCore: "health_check", PoolIO: "search_slot", PoolHeavy: "import_download", PoolMigrate: "migrate_move"}
	for _, stalled := range []Pool{PoolCore, PoolIO, PoolHeavy, PoolMigrate} {
		t.Run(string(stalled), func(t *testing.T) {
			s := newStore(t, t.TempDir())
			defer s.Close()
			o := fastOpts()
			o.Pools = map[Pool]int{PoolCore: 1, PoolIO: 2, PoolHeavy: 1, PoolMigrate: 1}
			q := New(s, o)
			hold := make(chan struct{})
			started := make(chan struct{}, 8)
			for p, kind := range kindOf {
				hang := p == stalled
				q.Handle(kind, func(ctx context.Context, r *Run) error {
					if hang && r.Job.Params["hang"] == true {
						started <- struct{}{}
						<-hold // ignores ctx on purpose
					}
					return nil
				})
			}
			ctx := context.Background()
			if err := q.Start(ctx); err != nil {
				t.Fatal(err)
			}
			defer q.Stop()
			defer close(hold)
			// Fill every worker of the stalled pool, plus one more waiting.
			n := o.Pools[stalled] + 1
			for i := range n {
				if _, _, err := q.Enqueue(ctx, kindOf[stalled], nil, map[string]any{"hang": true}, fmt.Sprintf("hang:%d", i), me); err != nil {
					t.Fatal(err)
				}
			}
			for range o.Pools[stalled] {
				select {
				case <-started:
				case <-time.After(5 * time.Second):
					t.Fatal("hung jobs did not start")
				}
			}
			for p, kind := range kindOf {
				if p == stalled {
					continue
				}
				id, _, err := q.Enqueue(ctx, kind, nil, nil, "", me)
				if err != nil {
					t.Fatal(err)
				}
				began := time.Now()
				waitState(t, s, id, "done")
				if d := time.Since(began); d > 2*time.Second {
					t.Fatalf("%s waited %s behind the stalled %s pool", kind, d, stalled)
				}
			}
		})
	}
}

func TestPoolOf(t *testing.T) {
	for kind, want := range map[string]Pool{"health_check": PoolCore, "search_slot": PoolIO, "grab": PoolIO,
		"subtitle_search": PoolIO, "import_download": PoolHeavy, "rename_files": PoolHeavy, "scan_title": PoolHeavy, "migrate_move": PoolMigrate, "migrate_undo": PoolMigrate, "something_new": PoolIO} {
		if got := PoolOf(kind); got != want {
			t.Errorf("PoolOf(%s) = %s, want %s", kind, got, want)
		}
	}
}
