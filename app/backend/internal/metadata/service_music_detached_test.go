package metadata

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

type blockingArtists struct {
	ArtistProvider
	calls   atomic.Int32
	release chan struct{}
}

func (b *blockingArtists) Artist(ctx context.Context, mbid string) (Artist, error) {
	b.calls.Add(1)
	select {
	case <-b.release:
	case <-ctx.Done():
	}
	return Artist{}, nil
}

func TestDetachedArtistMergesSameMBID(t *testing.T) {
	b := &blockingArtists{release: make(chan struct{})}
	m := &serviceMusic{direct: b}
	mbid := "5b11f4ce-a62d-471e-81fc-a69a8278c7da"
	var wg sync.WaitGroup
	flights := make([]*artistFlight, 100)
	for i := range flights {
		wg.Add(1)
		go func() {
			defer wg.Done()
			flights[i] = m.detachedArtist(context.Background(), mbid)
		}()
	}
	wg.Wait()
	close(b.release)
	for _, f := range flights {
		if f == nil {
			t.Fatal("same MBID was refused a flight")
		}
		<-f.done
	}
	if n := b.calls.Load(); n != 1 {
		t.Fatalf("upstream calls = %d, want 1", n)
	}
}

func TestDetachedArtistCapsDistinctMBIDs(t *testing.T) {
	b := &blockingArtists{release: make(chan struct{})}
	m := &serviceMusic{direct: b}
	started := 0
	var flights []*artistFlight
	for i := 0; i < 20; i++ {
		f := m.detachedArtist(context.Background(), fmt.Sprintf("5b11f4ce-a62d-471e-81fc-a69a8278c7%02d", i))
		if f != nil {
			started++
			flights = append(flights, f)
		}
	}
	if started != detachedArtistLimit {
		t.Fatalf("started = %d, want %d", started, detachedArtistLimit)
	}
	close(b.release)
	for _, f := range flights {
		<-f.done
	}
	if f := m.detachedArtist(context.Background(), "5b11f4ce-a62d-471e-81fc-a69a8278c7ff"); f == nil {
		t.Fatal("slot not freed after lookups finished")
	} else {
		<-f.done
	}
	if n := b.calls.Load(); n != int32(detachedArtistLimit)+1 {
		t.Fatalf("upstream calls = %d", n)
	}
}
