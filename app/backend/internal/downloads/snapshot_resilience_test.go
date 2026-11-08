package downloads

import (
	"context"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// Last-known rows, loaded or kept after a failed read, must carry no speeds or ETA.
func TestStore_StaleSnapshotHasNoSpeeds(t *testing.T) {
	s := &Store{}
	s.Load(1, []Job{{ID: "a", Speed: 100, UpSpeed: 50, ETA: 9}})
	if j := s.Current(1).Items["a"]; j.Speed != 0 || j.UpSpeed != 0 || j.ETA != 0 {
		t.Fatalf("loaded row keeps speeds: %+v", j)
	}
	s.Update(1, 0, []Job{{ID: "a", Speed: 100, UpSpeed: 50, ETA: 9}}, time.Now())
	if j := s.Current(1).Items["a"]; j.Speed != 100 || j.UpSpeed != 50 || j.ETA != 9 {
		t.Fatalf("fresh row lost speeds: %+v", j)
	}
	s.Fail(1, 0)
	if j := s.Current(1).Items["a"]; j.Speed != 0 || j.UpSpeed != 0 {
		t.Fatalf("failed client keeps speeds: %+v", j)
	}
}

// A good reader read must clear the fallback flag, even with nothing active.
func TestSync_ReadClearsFallback(t *testing.T) {
	r := newRig(t, own(0.2))
	ctx := context.Background()
	var d domain.Downloader
	if err := database.Get(ctx, r.db.Read(), "downloaders", r.dlID, &d); err != nil {
		t.Fatal(err)
	}
	r.s.Store.SetFallback(r.dlID, true)
	if err := r.s.read(ctx, d); err != nil {
		t.Fatal(err)
	}
	if r.s.Store.Current(r.dlID).Fallback {
		t.Fatal("fallback still set after a good read")
	}
}

// A failed database read must not end the poller, which tries again until stopped.
func TestSync_PollSurvivesDBError(t *testing.T) {
	fastTimers(t)
	r := newRig(t)
	_ = r.db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	done := make(chan struct{})
	go func() { r.s.poll(ctx, r.dlID, make(chan struct{})); close(done) }()
	select {
	case <-done:
		t.Fatal("poller returned on a DB error")
	case <-time.After(150 * time.Millisecond):
	}
	<-done
}
