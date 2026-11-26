package library

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// removeDuring removes the artist through the real Remove path on the
// third Pressings call, as a person clicking Remove mid-refresh would.
type removeDuring struct {
	*fakeMusic
	svc   *Service
	id    int64
	arm   atomic.Bool
	calls atomic.Int32
}

func (r *removeDuring) Pressings(ctx context.Context, mbid string) ([]metadata.Pressing, error) {
	if r.arm.Load() && r.calls.Add(1) == 3 {
		if _, err := r.svc.Remove(context.Background(), r.id, RemoveChoice{}, true, domain.System); err != nil {
			return nil, err
		}
	}
	return r.fakeMusic.Pressings(ctx, mbid)
}

// TestRefreshStopsWhenArtistRemoved (#805): removing an artist stops its
// running refresh within a few albums, and a queued refresh or scan of a
// removed artist does no work.
func TestRefreshStopsWhenArtistRemoved(t *testing.T) {
	svc, s, fm, _ := musicService(t)
	a := fm.artists[labArtist]
	for i := 0; i < 30; i++ {
		rg := fmt.Sprintf("30000000-0000-0000-0000-%012d", i)
		a.Albums = append(a.Albums, metadata.Album{MBID: rg, Name: fmt.Sprintf("Bootleg %d", i), PrimaryType: "album", ReleaseDate: "2012-01-01"})
	}
	fm.artists[labArtist] = a
	rd := &removeDuring{fakeMusic: fm, svc: svc}
	svc.Artists = rd
	rd.id = addLabArtist(t, svc, AlbumsAll)
	n := len(albumsOf(t, s, rd.id))
	if n < 30 {
		t.Fatalf("only %d albums", n)
	}

	rd.arm.Store(true)
	if _, err := svc.RefreshArtist(context.Background(), rd.id, domain.System, nil); err != nil {
		t.Fatalf("refresh after remove: %v", err)
	}
	if got := rd.calls.Load(); got > removedCheckEvery {
		t.Fatalf("asked for %d of %d albums' Pressings after the artist was removed at album 3", got, n)
	}

	// A refresh or scan queued before the remove ends without asking anything.
	rd.calls.Store(0)
	if _, err := svc.RefreshArtist(context.Background(), rd.id, domain.System, nil); err != nil || rd.calls.Load() != 0 {
		t.Fatalf("queued refresh of a removed artist: err %v, %d Pressings calls", err, rd.calls.Load())
	}
	fm.mu.Lock()
	fm.calls = nil
	fm.mu.Unlock()
	if _, err := svc.RefreshArtist(context.Background(), rd.id, domain.System, nil); err != nil || len(fm.calls) != 0 {
		t.Fatalf("removed artist still read the artist: %v %v", err, fm.calls)
	}
}
