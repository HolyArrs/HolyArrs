// Package adaptertest holds the behaviour contract that every outside-service
// adapter must pass in its tests.
package adaptertest

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Contract describes the behaviour an adapter operation must show.
type Contract struct {
	Name         string
	Success      func(context.Context) error
	Failure      func(context.Context) error
	Cancellation bool
	Unsupported  []string
}

// Fake is an in-memory adapter that proves the contract itself.
type Fake struct{ Err error }

// Run checks success, a meaningful remote failure and, when asked, that a
// canceled caller gets context.Canceled within two seconds.
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

// Do returns the context error if there is one, else the configured result.
func (f Fake) Do(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return f.Err
}
