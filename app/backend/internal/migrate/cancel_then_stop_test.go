package migrate

import (
	"context"
	"sync"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// TestMoveCancelledThenStoppedStaysStopped: a person cancels a move, and
// the app stops before the slow step returns. The move ends failed and does
// not resume after the restart.
func TestMoveCancelledThenStoppedStaysStopped(t *testing.T) {
	l := newMoveLab(t)
	ctx := context.Background()
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	l.mv.PartOn = func(string) bool {
		once.Do(func() { close(entered) })
		<-release
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
	if err := l.q.Cancel(ctx, mv.JobID); err != nil {
		t.Fatal(err)
	}
	stopApp()
	close(release)
	l.q.Stop()

	got, err := GetMove(ctx, l.st, mv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State == "running" || got.State == "queued" {
		t.Fatalf("a cancelled move was left %s to resume", got.State)
	}
	if err := l.mv.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	if err := l.q.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer l.q.Stop()
	time.Sleep(200 * time.Millisecond)
	if got, _ = GetMove(ctx, l.st, mv.ID); got.State == "done" {
		t.Fatal("the cancelled move resumed and finished after the restart")
	}
}
