package downloads

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
)

type guardedStub struct {
	Engine
	calls int
	err   error
}

func (s *guardedStub) Health(context.Context) (Health, error) { s.calls++; return Health{}, s.err }

func (s *guardedStub) Add(context.Context, AddRequest) (string, error) { s.calls++; return "", s.err }

func TestDownloaderGuardCountsLogicalCallsWithoutReplayingAdd(t *testing.T) {
	r := outbound.New([]byte("isolated-test-registry-key"), outbound.Options{Threshold: 2, Cooldown: time.Minute})
	s := &guardedStub{err: errDownloaderUnavailable}
	g := &guardedEngine{Engine: s, target: outbound.Target{Kind: "downloader", Identity: []string{"owned"}}, begin: r.Begin}
	for range 2 {
		if _, err := g.Add(context.Background(), AddRequest{}); !errors.Is(err, errDownloaderUnavailable) {
			t.Fatal(err)
		}
	}
	if _, err := g.Add(context.Background(), AddRequest{}); !errors.Is(err, outbound.ErrSuppressed) {
		t.Fatal(err)
	}
	if s.calls != 2 {
		t.Fatalf("mutation replayed: %d calls", s.calls)
	}
	if len(r.States()) != 1 {
		t.Fatal("failing downloader missing safe retry state")
	}
}

func TestDownloaderGuardDoesNotTripForSignInOrCancellation(t *testing.T) {
	for _, cause := range []error{ErrAuth, ErrNotFound, ErrCategory, context.Canceled} {
		r := outbound.New([]byte("isolated-test-registry-key"), outbound.Options{Threshold: 1})
		s := &guardedStub{err: cause}
		g := &guardedEngine{Engine: s, target: outbound.Target{Kind: "downloader"}, begin: r.Begin}
		ctx := context.Background()
		if cause == context.Canceled {
			var cancel context.CancelFunc
			ctx, cancel = context.WithCancel(ctx)
			cancel()
		}
		for range 3 {
			_, _ = g.Health(ctx)
		}
		if len(r.States()) != 0 {
			t.Fatalf("terminal/caller failure tripped breaker: %v", cause)
		}
	}
}

func TestDownloaderGuardClassifiesTemporaryHTTPFailures(t *testing.T) {
	for _, status := range []int{408, 429, 503} {
		r := outbound.New([]byte("isolated-test-registry-key"), outbound.Options{Threshold: 1})
		g := &guardedEngine{Engine: &guardedStub{err: adapterHTTPError{status}}, target: outbound.Target{Kind: "downloader"}, begin: r.Begin}
		_, _ = g.Health(context.Background())
		if len(r.States()) != 1 {
			t.Fatalf("HTTP%d did not trip", status)
		}
	}
}

func TestOptionalPolicyRefusalDoesNotTripDownloaderGuard(t *testing.T) {
	for _, kind := range []string{"transmission", "nzbget", "deluge", "rtorrent"} {
		t.Run(kind, func(t *testing.T) {
			e, err := (&Manager{}).EngineFor(context.Background(), domain.Downloader{Adapter: kind, URL: "http://127.0.0.1:1"})
			if err != nil {
				t.Fatal(err)
			}
			r := outbound.New([]byte("isolated-test-registry-key"), outbound.Options{Threshold: 1})
			g := &guardedEngine{Engine: e, target: outbound.Target{Kind: "downloader"}, begin: r.Begin}
			for range 2 {
				if _, err := g.Health(context.Background()); !errors.Is(err, outbound.ErrAddressRefused) {
					t.Fatalf("policy cause lost: %v", err)
				}
			}
			if len(r.States()) != 0 {
				t.Fatal("network policy refusal tripped a transient breaker")
			}
		})
	}
}
