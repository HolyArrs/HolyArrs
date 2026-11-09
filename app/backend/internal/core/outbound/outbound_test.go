package outbound

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestBreakerSuppressesAndRecoversWithOneProbe(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	r := New([]byte("test process key"), Options{Threshold: 3, Cooldown: 30 * time.Second, Now: func() time.Time { return now }})
	target := Target{Kind: "metadata", Identity: []string{"https://bad.example", "secret"}}
	for range 3 {
		op, err := r.Begin(context.Background(), target)
		if err != nil {
			t.Fatal(err)
		}
		op.Finish(TransientFailure)
	}
	if _, err := r.Begin(context.Background(), target); !errors.Is(err, ErrSuppressed) {
		t.Fatalf("open Begin error = %v", err)
	}
	states := r.States()
	if len(states) != 1 || states[0].Status != "retrying" || states[0].Reason == "" || states[0].ID == "" {
		t.Fatalf("states = %#v", states)
	}
	if states[0].ID == target.Identity[0] || states[0].ID == target.Identity[1] {
		t.Fatalf("identity leaked: %#v", states[0])
	}
	now = now.Add(30 * time.Second)
	probe, err := r.Begin(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	const callers = 20
	var wg sync.WaitGroup
	errCh := make(chan error, callers)
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := r.Begin(context.Background(), target)
			errCh <- err
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if !errors.Is(err, ErrSuppressed) {
			t.Errorf("concurrent Begin error = %v", err)
		}
	}
	probe.Finish(Success)
	if _, err := r.Begin(context.Background(), target); err != nil {
		t.Fatalf("recovered Begin: %v", err)
	}
}

func TestCancellationAndEndpointIsolation(t *testing.T) {
	now := time.Now()
	r := New([]byte("test process key"), Options{Threshold: 1, Cooldown: time.Minute, Now: func() time.Time { return now }})
	a := Target{Kind: "source", Identity: []string{"a", "key"}}
	b := Target{Kind: "source", Identity: []string{"b", "key"}}
	op, _ := r.Begin(context.Background(), a)
	op.Finish(Canceled)
	if _, err := r.Begin(context.Background(), a); err != nil {
		t.Fatalf("cancellation poisoned endpoint: %v", err)
	}
	op, _ = r.Begin(context.Background(), a)
	op.Finish(TransientFailure)
	if _, err := r.Begin(context.Background(), a); !errors.Is(err, ErrSuppressed) {
		t.Fatalf("bad endpoint was not suppressed: %v", err)
	}
	if _, err := r.Begin(context.Background(), b); err != nil {
		t.Fatalf("unrelated endpoint suppressed: %v", err)
	}
}

func TestCanceledHalfOpenProbeDoesNotProveRecoveryOrFailure(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	r := New([]byte("test process key"), Options{Threshold: 1, Cooldown: time.Minute, Now: func() time.Time { return now }})
	target := Target{Kind: "alert", Identity: []string{"one"}}
	op, _ := r.Begin(context.Background(), target)
	op.Finish(TransientFailure)
	now = now.Add(time.Minute)
	probe, err := r.Begin(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	probe.Finish(Canceled)
	probe, err = r.Begin(context.Background(), target)
	if err != nil {
		t.Fatalf("replacement probe: %v", err)
	}
	probe.Finish(TransientFailure)
	if _, err := r.Begin(context.Background(), target); !errors.Is(err, ErrSuppressed) {
		t.Fatalf("failed probe did not reopen breaker: %v", err)
	}
}

func TestAttemptDeadlineAndStatusClassification(t *testing.T) {
	r := New([]byte("test process key"), Options{})
	op, err := r.Begin(context.Background(), Target{Kind: "test", Identity: []string{"one"}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := op.AttemptContext(context.Background(), 5*time.Millisecond)
	defer cancel()
	<-ctx.Done()
	if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatalf("attempt error = %v", ctx.Err())
	}
	if got := HTTPOutcome(0, ctx.Err(), context.Background()); got != TransientFailure {
		t.Fatalf("deadline outcome = %v", got)
	}
	canceled, stop := context.WithCancel(context.Background())
	stop()
	if got := HTTPOutcome(0, canceled.Err(), canceled); got != Canceled {
		t.Fatalf("caller cancellation outcome = %v", got)
	}
	for _, status := range []int{http.StatusRequestTimeout, http.StatusTooManyRequests, http.StatusInternalServerError} {
		if got := HTTPOutcome(status, nil, context.Background()); got != TransientFailure {
			t.Errorf("status %d outcome = %v", status, got)
		}
	}
	for _, status := range []int{http.StatusMovedPermanently, http.StatusBadRequest, http.StatusUnauthorized} {
		if got := HTTPOutcome(status, nil, context.Background()); got != TerminalFailure {
			t.Errorf("status %d outcome = %v", status, got)
		}
	}
	op.Finish(TerminalFailure)
}

func TestRegistryStaysBoundedAndKeepsActiveCalls(t *testing.T) {
	r := New([]byte("test process key"), Options{Max: 2})
	active, _ := r.Begin(context.Background(), Target{Kind: "test", Identity: []string{"active"}})
	second, _ := r.Begin(context.Background(), Target{Kind: "test", Identity: []string{"second"}})
	untracked, err := r.Begin(context.Background(), Target{Kind: "test", Identity: []string{"while-full"}})
	if err != nil {
		t.Fatal(err)
	}
	untracked.Finish(TransientFailure)
	if len(r.entries) != 2 {
		t.Fatalf("active entries = %d", len(r.entries))
	}
	second.Finish(Success)
	done, _ := r.Begin(context.Background(), Target{Kind: "test", Identity: []string{"done"}})
	done.Finish(Success)
	third, err := r.Begin(context.Background(), Target{Kind: "test", Identity: []string{"third"}})
	if err != nil {
		t.Fatal(err)
	}
	third.Finish(Success)
	if len(r.entries) != 2 {
		t.Fatalf("entries = %d", len(r.entries))
	}
	active.Finish(Success)
}

func TestStateOpenUntilShowsNextTry(t *testing.T) {
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	r := New([]byte("test process key"), Options{Threshold: 2, Cooldown: 45 * time.Second, Now: func() time.Time { return now }})
	target := Target{Kind: "downloader", Identity: []string{"lab"}}
	for range 2 {
		op, err := r.Begin(context.Background(), target)
		if err != nil {
			t.Fatal(err)
		}
		op.Finish(TransientFailure)
	}
	states := r.States()
	if len(states) != 1 || !states[0].OpenUntil.Equal(now.Add(45*time.Second)) {
		t.Fatalf("states = %#v", states)
	}
}
