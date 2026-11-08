package api

import (
	"context"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// A person's answer never waits behind a background ask of the same records
// (it may be queued for the background budget): it asks them itself. A kept
// record past its time is served at once and asked again behind it.
func TestPictureRecordsPersonSkipsBackgroundWait(t *testing.T) {
	a := New(Server{Version: "test", ImageBase: "https://api.example.test/v1/img"})
	release := make(chan struct{})
	defer close(release)
	a.s.MusicPictures = func(ctx context.Context, artists, albums []string) (metadata.MusicPictures, error) {
		if metadata.IsAhead(ctx) {
			<-release // held by the gate's background budget
		}
		return metadata.MusicPictures{Albums: map[string]metadata.AlbumPictures{picAlbum: {Cover: &metadata.PictureRecord{V: "c1", W: 500, H: 500}}}}, nil
	}
	a.pictureRecords(metadata.Ahead(context.Background()), nil, []string{picAlbum}, 0)
	start := time.Now()
	got := a.pictureRecords(context.Background(), nil, []string{picAlbum}, 2*time.Second)
	if d := time.Since(start); d > time.Second || got["album|"+picAlbum].album.Cover == nil {
		t.Fatalf("person answer waited %v behind a background ask: %+v", d, got)
	}
	// Past its time (no cover: picShort), the kept record answers at once.
	a.pics.mu.Lock()
	e := a.pics.m["album|"+picAlbum]
	e.album.Cover, e.at = nil, time.Now().Add(-time.Hour)
	a.pics.m["album|"+picAlbum] = e
	a.pics.mu.Unlock()
	a.s.MusicPictures = func(ctx context.Context, artists, albums []string) (metadata.MusicPictures, error) {
		time.Sleep(time.Second)
		return metadata.MusicPictures{}, nil
	}
	start = time.Now()
	a.pictureRecords(context.Background(), nil, []string{picAlbum}, 2*time.Second)
	if d := time.Since(start); d > 300*time.Millisecond {
		t.Fatalf("a kept record waited %v for its refresh", d)
	}
}
