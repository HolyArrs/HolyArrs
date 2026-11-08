package updates

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestUpdateFeedErrorKeepsLastGood(t *testing.T) {
	w := newWorld(t)
	f := newFeed(t)
	w.e.ReleaseFeed = f.srv.URL
	ctx := context.Background()
	f.body = `{"tag_name": "v9.0.0"}`
	if i := w.e.CheckUpdate(ctx); !i.Newer || i.Latest != "v9.0.0" {
		t.Fatalf("first check %+v", i)
	}
	f.code, f.body = 500, "boom"
	i := w.e.CheckUpdate(ctx)
	if i.Latest != "v9.0.0" || !i.Newer || i.Problem == "" {
		t.Fatalf("after a failed check %+v", i)
	}
	// What was kept is also what is stored: a new Checker reads it back.
	e2 := &Checker{Store: w.s, Version: "v1.1.0", ReleaseFeed: f.srv.URL, Now: w.e.Now}
	if i := e2.Update(ctx); i.Latest != "v9.0.0" || !i.Newer {
		t.Fatalf("after a restart %+v", i)
	}
}

func TestUpdateEveryBadSettingFallsBack(t *testing.T) {
	w := newWorld(t)
	f := newFeed(t)
	f.body = `{"tag_name": "v9.0.0"}`
	ctx := context.Background()
	for _, v := range []string{"0", "-1", "999", "x", "2"} {
		if err := w.e.saveSetting(ctx, settingUpdateDays, v); err != nil {
			t.Fatal(err)
		}
		if got := w.e.Every(ctx); got != 24*time.Hour {
			t.Fatalf("update_check_every_days=%q gives %v, want a day", v, got)
		}
	}
	if err := w.e.saveSetting(ctx, settingUpdateDays, "7"); err != nil {
		t.Fatal(err)
	}
	if got := w.e.Every(ctx); got != 7*24*time.Hour {
		t.Fatalf("7 gives %v", got)
	}
	// With a junk value the feed is asked once a day, not on every tick.
	if err := w.e.saveSetting(ctx, settingUpdateDays, "x"); err != nil {
		t.Fatal(err)
	}
	w.e.ReleaseFeed = f.srv.URL
	w.e.Scheduled(ctx)
	w.e.Scheduled(ctx)
	if f.calls.Load() != 1 {
		t.Fatalf("asked %d times with the clock still", f.calls.Load())
	}
	w.now = w.now.Add(25 * time.Hour)
	w.e.Scheduled(ctx)
	if f.calls.Load() != 2 {
		t.Fatalf("asked %d times after a day", f.calls.Load())
	}
}

// Run with -race: the scheduler, the check button, the reader and the reset
// share one Checker.
func TestUpdateCheckerConcurrentSafe(t *testing.T) {
	w := newWorld(t)
	f := newFeed(t)
	f.body = `{"tag_name": "v9.0.0"}`
	w.e.ReleaseFeed = f.srv.URL
	ctx := context.Background()
	var wg sync.WaitGroup
	for g := 0; g < 20; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				switch g % 4 {
				case 0:
					w.e.Scheduled(ctx)
				case 1:
					w.e.CheckUpdate(ctx)
				case 2:
					w.e.Update(ctx)
				default:
					w.e.ResetUpdate(ctx)
				}
			}
		}()
	}
	wg.Wait()
	if v := w.e.Update(ctx).Version; v != w.e.Version {
		t.Fatalf("version %q, want %q", v, w.e.Version)
	}
}
