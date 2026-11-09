package library

import (
	"context"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// TestAskedAlbumSurvivesRefresh: an album taken in by IncludeAlbums (a single
// the artist's album kinds leave out) is stored as included and kept, and a
// later refresh does not drop it.
func TestAskedAlbumSurvivesRefresh(t *testing.T) {
	ctx := context.Background()
	svc, s, _, _ := musicService(t)
	id, _, err := svc.AddArtist(ctx, labArtist, ArtistAddOptions{}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	single := albumsOf(t, s, id)[rgSingle]
	if single.Included || single.Keep {
		t.Fatalf("single before asking %+v", single)
	}
	if err := svc.IncludeAlbums(ctx, id, []int64{single.ID}, domain.System); err != nil {
		t.Fatal(err)
	}
	if got := albumsOf(t, s, id)[rgSingle]; !got.Included || !got.Keep {
		t.Fatalf("single after asking %+v", got)
	}
	for i := 0; i < 2; i++ {
		if _, err := svc.RefreshArtist(ctx, id, domain.System, nil); err != nil {
			t.Fatal(err)
		}
		if got := albumsOf(t, s, id)[rgSingle]; !got.Included || !got.Keep || got.Dropped {
			t.Fatalf("refresh %d dropped the asked single %+v", i+1, got)
		}
	}
	if live := albumsOf(t, s, id)[rgLive]; live.Included {
		t.Fatalf("unasked live album included %+v", live)
	}
}
