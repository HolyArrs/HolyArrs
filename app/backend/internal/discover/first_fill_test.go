package discover

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	engine "github.com/HolyArrs/holyarrs/app/backend/internal/suggest"
)

// slowLister answers once release is closed and reports its ask's context.
type slowLister struct {
	release  chan struct{}
	deadline chan time.Duration
	gone     chan error
}

func (s *slowLister) ExploreList(ctx context.Context, kind, path string, q url.Values) ([]metadata.DiscoverItem, error) {
	d, _ := ctx.Deadline()
	s.deadline <- time.Until(d)
	select {
	case <-s.release:
		return []metadata.DiscoverItem{{Kind: "movie", TMDBID: "1", Name: "One"}}, nil
	case <-ctx.Done():
		s.gone <- ctx.Err()
		return nil, ctx.Err()
	}
}

func (s *slowLister) PersonCredits(ctx context.Context, name string) ([]metadata.DiscoverItem, error) {
	return nil, nil
}

func (s *slowLister) GenreID(ctx context.Context, kind, genre string) (int64, error) { return 0, nil }

// A page that leaves does not cancel the shared fetch: the list still lands
// for the next page, and a first fill (nothing kept) gets the long wait.
func TestExploreListOutlivesTheAsker(t *testing.T) {
	l := &slowLister{release: make(chan struct{}), deadline: make(chan time.Duration, 1), gone: make(chan error, 1)}
	e := &Explore{Store: noStore, Paced: &engine.Paced{Source: ListerSource{L: l}, Burst: 100}}
	s := ExploreSource{E: e, Lister: l, Calls: &engine.Calls{}}
	q := engine.Request{List: "trending", Kind: "movie"}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := s.List(ctx, q)
		done <- err
	}()
	if d := <-l.deadline; d < firstFillWait-time.Minute {
		t.Fatalf("first fill waits %v, want about %v", d, firstFillWait)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("leaving asker got %v", err)
	}
	close(l.release)
	deadline := time.Now().Add(5 * time.Second)
	for {
		if got, ok := s.cachedList(context.Background(), q.String()); ok && len(got.items) == 1 {
			break
		}
		select {
		case err := <-l.gone:
			t.Fatalf("shared fetch was cancelled: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("the list never landed after the asker left")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// The first sign-in of a member with no pool (the owner right after setup)
// queues their Explore pools now, before the first visit.
func TestFirstSignInQueuesPools(t *testing.T) {
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	e := &Explore{Store: func() *database.Store { return st }}
	e.OnEvent(model.Event{Kind: "signed_in", Actor: model.Actor{Kind: "user", ID: 7}})
	stop := time.Now().Add(5 * time.Second)
	for {
		_, queued, _ := e.BuildCounts()
		if queued == len(ExploreSwitches) {
			return
		}
		if time.Now().After(stop) {
			t.Fatalf("queued %d pools, want %d", queued, len(ExploreSwitches))
		}
		time.Sleep(10 * time.Millisecond)
	}
}
