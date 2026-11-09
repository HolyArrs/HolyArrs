package migrate

import (
	"context"
	"sync"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// TestMoveStoppedByShutdownResumes: a stop of the app (docker stop, an
// update) during a move leaves the move to resume after the restart, as a
// hard kill does, instead of failing it with a line per item.
func TestMoveStoppedByShutdownResumes(t *testing.T) {
	l := newMoveLab(t)
	ctx := context.Background()
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	blocking := true
	var mu sync.Mutex
	l.mv.PartOn = func(string) bool {
		mu.Lock()
		b := blocking
		mu.Unlock()
		if b {
			once.Do(func() { close(entered) })
			<-release
		}
		return true
	}
	p := l.plan(t)
	mv, err := l.mv.Start(ctx, l.snaps, Choices{PathRules: l.rules, People: l.people}, PlanHash(p), domain.Actor{Kind: "user", ID: 1})
	if err != nil {
		t.Fatal(err)
	}
	appCtx, stopApp := context.WithCancel(ctx)
	if err := l.q.Start(appCtx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("the move never started")
	}
	stopApp() // the app is stopping
	mu.Lock()
	blocking = false
	mu.Unlock()
	close(release)
	l.q.Stop()

	got, err := GetMove(ctx, l.st, mv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State == "failed" {
		t.Fatalf("a stop failed the move: %+v", got.Report)
	}

	// The restart: Recover, then the queue picks the move up again.
	if err := l.mv.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	if err := l.q.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer l.q.Stop()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		got, err = GetMove(ctx, l.st, mv.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.State == "done" || got.State == "failed" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got.State != "done" {
		t.Fatalf("the move did not resume to done: %s %+v", got.State, got.Report)
	}
	if got.Report == nil || got.Report.Counts[ResFailed] != 0 {
		t.Fatalf("the resumed move has failed lines: %+v", got.Report)
	}
}
