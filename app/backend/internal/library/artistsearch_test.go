package library

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// slowSearch is MusicBrainz whose name search answers after d.
type slowSearch struct {
	*fakeMusic
	d time.Duration
	n atomic.Int64
}

func (s *slowSearch) SearchArtists(ctx context.Context, q string) ([]metadata.Artist, error) {
	s.n.Add(1)
	select {
	case <-time.After(s.d):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return s.fakeMusic.SearchArtists(ctx, q)
}

// An unseen name never holds the visitor past ArtistSearchWait: the search
// finishes behind, is kept, and the next ask is answered from the store
// without asking MusicBrainz again.
func TestLookupArtistsUnseenNameDoesNotWait(t *testing.T) {
	svc, _, fm, _ := musicService(t)
	slow := &slowSearch{fakeMusic: fm, d: 400 * time.Millisecond}
	svc.Artists = slow
	old := ArtistSearchWait
	ArtistSearchWait = 50 * time.Millisecond
	t.Cleanup(func() { ArtistSearchWait = old })

	start := time.Now()
	_, err := svc.LookupArtists(context.Background(), "lab band")
	if took := time.Since(start); took > 250*time.Millisecond {
		t.Fatalf("unseen name held the request %v", took)
	}
	if !errors.Is(err, ErrSearchFilling) {
		t.Fatalf("err = %v, want ErrSearchFilling", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	var got []metadata.Artist
	for time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		if got, err = svc.LookupArtists(context.Background(), "Lab  Band"); err == nil {
			break
		}
	}
	if err != nil || len(got) != 1 || got[0].MBID != labArtist {
		t.Fatalf("kept answer = %v, %v", got, err)
	}
	start = time.Now()
	if got, err = svc.LookupArtists(context.Background(), "lab band"); err != nil || len(got) != 1 {
		t.Fatalf("second ask = %v, %v", got, err)
	}
	if took := time.Since(start); took > 100*time.Millisecond {
		t.Fatalf("kept name took %v", took)
	}
	if slow.n.Load() != 1 {
		t.Fatalf("MusicBrainz searched %d times, want 1", slow.n.Load())
	}
}
