package updates_test

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/updates"
)

func pendingStarts(t *testing.T, v *volume) int {
	t.Helper()
	var p struct{ Starts int }
	b, err := os.ReadFile(v.app("pending.json"))
	if err != nil {
		return -1
	}
	_ = json.Unmarshal(b, &p)
	return p.Starts
}

func TestRunImageProgramDoesNothing(t *testing.T) {
	v := newVolume(t)
	r := updates.StartRun(v.cfg, "1.1.4", time.Millisecond, func(int) { t.Error("watchdog fired for the image program") })
	time.Sleep(20 * time.Millisecond)
	if !r.Healthy() {
		t.Fatal("image program should count as healthy")
	}
}

func TestRunHealthyRemovesPendingAndStopsWatchdog(t *testing.T) {
	t.Setenv("HOLYARRS_FROM_VOLUME", "1")
	v := newVolume(t)
	v.set("pending.json", map[string]any{"version": "1.2.0", "starts": 1, "since": time.Now()})
	r := updates.StartRun(v.cfg, "1.2.0", 50*time.Millisecond, func(int) { t.Error("watchdog fired after healthy") })
	r.Ready(time.Millisecond, func() bool { return true })
	time.Sleep(100 * time.Millisecond)
	if !r.Healthy() || v.exists("pending.json") {
		t.Fatal("not marked healthy")
	}
}

func TestRunHangTriggersWatchdog(t *testing.T) {
	t.Setenv("HOLYARRS_FROM_VOLUME", "1")
	v := newVolume(t)
	got := make(chan int, 1)
	updates.StartRun(v.cfg, "1.2.0", 10*time.Millisecond, func(c int) { got <- c })
	select {
	case c := <-got:
		if c == 0 {
			t.Fatal("watchdog exit status 0")
		}
	case <-time.After(time.Second):
		t.Fatal("watchdog did not fire")
	}
}

func TestRunCleanStopBeforeHealthyIsNotCounted(t *testing.T) {
	t.Setenv("HOLYARRS_FROM_VOLUME", "1")
	v := newVolume(t)
	v.set("pending.json", map[string]any{"version": "1.2.0", "starts": 1, "since": time.Now()})
	r := updates.StartRun(v.cfg, "1.2.0", time.Hour, nil)
	r.CleanStop()
	if n := pendingStarts(t, v); n != 0 {
		t.Fatalf("starts %d after a clean stop", n)
	}
}
