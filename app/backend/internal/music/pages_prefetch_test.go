package music

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

type tracklistArtists struct {
	aheadArtists
	mu2    sync.Mutex
	warmed map[string]bool
}

func (f *tracklistArtists) Pressings(ctx context.Context, mbid string) ([]metadata.Pressing, error) {
	f.mu2.Lock()
	defer f.mu2.Unlock()
	if f.warmed == nil {
		f.warmed = map[string]bool{}
	}
	f.warmed[mbid] = metadata.IsTracklistOnly(ctx)
	return nil, nil
}

// An artist page asks its first albums' tracklists ahead (the one-call
// answer only), the latest first, never upcoming ones.
func TestArtistPagePrefetchesFirstTracklists(t *testing.T) {
	t.Parallel()
	f := &tracklistArtists{}
	a := newTestAPI(Server{Version: "test", Library: &library.Service{Artists: f}})
	id := func(i int) string {
		return "10000000-0000-0000-0000-0000000000" + string(rune('a'+i/16)) + string("0123456789abcdef"[i%16])
	}
	v := ArtistPageView{Latest: &PageAlbum{MBID: id(0)}}
	for i := 0; i < 10; i++ {
		v.Albums = append(v.Albums, PageAlbum{MBID: id(i), Shelf: "albums"})
	}
	v.Albums = append(v.Albums, PageAlbum{MBID: id(20), Shelf: "albums", Upcoming: true}, PageAlbum{MBID: id(21), Shelf: "singles"}, PageAlbum{MBID: id(22), Shelf: "live"})
	ids := firstAlbums(v)
	if len(ids) != 7 || ids[0] != id(0) || ids[6] != id(21) {
		t.Fatalf("first albums %v", ids)
	}
	a.prefetchTracklists(metadata.MemberLookup(context.Background()), ids)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		f.mu2.Lock()
		n := len(f.warmed)
		f.mu2.Unlock()
		if n == len(ids) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	f.mu2.Lock()
	defer f.mu2.Unlock()
	for _, x := range ids {
		if only, ok := f.warmed[x]; !ok || !only {
			t.Fatalf("album %s warmed %v tracklist-only %v", x, ok, only)
		}
	}
}
