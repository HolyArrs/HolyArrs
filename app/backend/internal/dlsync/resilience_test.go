package dlsync

import (
	"context"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloader"
)

// Last-known rows (loaded or after a failed read) carry no speeds or ETA.
func TestStaleSnapshotHasNoSpeeds(t *testing.T) {
	s := &Store{}
	s.Load(1, []downloader.Job{{ID: "a", Speed: 100, UpSpeed: 50, ETA: 9}})
	if j := s.Current(1).Items["a"]; j.Speed != 0 || j.UpSpeed != 0 || j.ETA != 0 {
		t.Fatalf("loaded row keeps speeds: %+v", j)
	}
	s.Update(1, 0, []downloader.Job{{ID: "a", Speed: 100, UpSpeed: 50, ETA: 9}}, time.Now())
	if j := s.Current(1).Items["a"]; j.Speed != 100 || j.UpSpeed != 50 || j.ETA != 9 {
		t.Fatalf("fresh row lost speeds: %+v", j)
	}
	s.Fail(1, 0)
	if j := s.Current(1).Items["a"]; j.Speed != 0 || j.UpSpeed != 0 {
		t.Fatalf("failed client keeps speeds: %+v", j)
	}
}

// A good reader read clears the fallback flag, even with nothing active.
func TestReadClearsFallback(t *testing.T) {
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

// A failed DB read does not end the poller; it tries again until stopped.
func TestPollSurvivesDBError(t *testing.T) {
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
