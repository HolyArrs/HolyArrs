package jobs

import (
	"context"
	"testing"
)

// A Job the person cancelled stays cancelled when the app also stops
// before the handler returns: Stopping is false and the Job is not left
// running for the next start to resume.
func TestCancelThenStopStaysCancelled(t *testing.T) {
	s := newStore(t, t.TempDir())
	defer s.Close()
	q := New(s, fastOpts())
	started := make(chan struct{})
	release := make(chan struct{})
	stopping := make(chan bool, 1)
	q.Handle("long", func(ctx context.Context, r *Run) error {
		close(started)
		<-release // a slow step that does not watch ctx
		stopping <- r.Stopping()
		return ctx.Err()
	})
	ctx := context.Background()
	appCtx, stopApp := context.WithCancel(ctx)
	if err := q.Start(appCtx); err != nil {
		t.Fatal(err)
	}
	id, _, _ := q.Enqueue(ctx, "long", nil, nil, "", me)
	<-started
	if err := q.Cancel(ctx, id); err != nil {
		t.Fatal(err)
	}
	stopApp()
	close(release)
	q.Stop()
	if <-stopping {
		t.Fatal("Stopping is true for a Job the person cancelled first")
	}
	if j := getJob(t, s, id); j.State != "cancelled" {
		t.Fatalf("state %s, want cancelled", j.State)
	}
}

// A plain app stop still says Stopping and leaves the Job running.
func TestStopAloneIsStopping(t *testing.T) {
	s := newStore(t, t.TempDir())
	defer s.Close()
	q := New(s, fastOpts())
	started := make(chan struct{})
	stopping := make(chan bool, 1)
	q.Handle("long", func(ctx context.Context, r *Run) error {
		close(started)
		<-ctx.Done()
		stopping <- r.Stopping()
		return ctx.Err()
	})
	ctx := context.Background()
	appCtx, stopApp := context.WithCancel(ctx)
	if err := q.Start(appCtx); err != nil {
		t.Fatal(err)
	}
	id, _, _ := q.Enqueue(ctx, "long", nil, nil, "", me)
	<-started
	stopApp()
	q.Stop()
	if !<-stopping {
		t.Fatal("Stopping is false on an app stop")
	}
	if j := getJob(t, s, id); j.State != "running" {
		t.Fatalf("state %s, want running", j.State)
	}
}
