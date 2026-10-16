// Package adaptertest supplies shared behavioral contracts for adapter tests.
package adaptertest

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Contract describes the behavior every supported adapter operation must expose.
type Contract struct {
	Name         string
	Success      func(context.Context) error
	Failure      func(context.Context) error
	Cancellation bool
	Unsupported  []string
}

// Run checks success, a meaningful remote failure, and caller cancellation.
func Run(t *testing.T, c Contract) {
	t.Helper()
	t.Run(c.Name, func(t *testing.T) {
		t.Run("success", func(t *testing.T) {
			if err := c.Success(context.Background()); err != nil {
				t.Fatalf("successful operation: %v", err)
			}
		})
		t.Run("failure", func(t *testing.T) {
			if err := c.Failure(context.Background()); err == nil {
				t.Fatal("remote failure returned nil")
			}
		})
		if c.Cancellation {
			t.Run("cancellation", func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				done := make(chan error, 1)
				go func() { done <- c.Success(ctx) }()
				select {
				case err := <-done:
					if !errors.Is(err, context.Canceled) {
						t.Fatalf("canceled operation returned %v, want context.Canceled", err)
					}
				case <-time.After(2 * time.Second):
					t.Fatal("canceled operation did not return within 2 seconds")
				}
			})
		}
		for _, capability := range c.Unsupported {
			if capability == "" {
				t.Fatal("unsupported capability needs a name")
			}
			t.Logf("unsupported capability: %s", capability)
		}
	})
}

// Fake is an in-memory adapter used to prove the shared contract itself.
type Fake struct{ Err error }

// Do completes in memory, propagating cancellation before its configured result.
func (f Fake) Do(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return f.Err
}
