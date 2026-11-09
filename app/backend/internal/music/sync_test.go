package music

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

const (
	syncArtist = "b8a7c51f-362c-4dcb-a259-bc6e0095f0a6"
	syncAlbum  = "3ff81a80-fe2f-4d9a-b542-4cb02fd08431"
	syncOther  = "35723b60-732e-4bd8-957f-320b416e7b7f"
)

type onePackage struct {
	mu    sync.Mutex
	wants []string
}

func (o *onePackage) MusicPackages(_ context.Context, since int64, _ int) (metadata.PackagePage, error) {
	page := metadata.PackagePage{SchemaVersion: 1, Since: since, Next: 1, Latest: 1}
	if since == 0 {
		page.Packages = []json.RawMessage{json.RawMessage(`{"schema_version":1,"mbid":"` + syncArtist + `","version":1,"hash":"h","rank":1,"complete":true,"missing":[],
"artist":{"kind":"artist","source":"musicbrainz","source_id":"` + syncArtist + `","title":"Ed","source_data":{"id":"` + syncArtist + `","name":"Ed Sheeran","life-span":{"ended":false}}},
"pictures":{"photo":{"v":"ph0to","w":1200,"h":1200,"color":"#3a66a9"},"wide":{"v":"w1de","w":1920,"h":1080,"color":"#cbc7c3"}},
"release_groups_complete":true,"release_groups":[{"id":"` + syncAlbum + `","title":"Spinning Man","kind":"album","primary_type":"Album","secondary_types":[],"cover":{"v":"c0ver","w":500,"h":500,"color":"#000000"},"tracklist":"pending"}],"albums":{}}`)}
	}
	return page, nil
}

func (o *onePackage) WantMusicPackage(_ context.Context, mbid string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.wants = append(o.wants, mbid)
	return nil
}

func TestMusicPagesUsePackagePictures(t *testing.T) {
	t.Parallel()
	st, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	src := &onePackage{}
	p := &PackageSync{Store: st, Source: src}
	if _, err := p.SyncOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	a := newTestAPI(Server{Packages: p})
	a.s.MusicPictures = func(context.Context, []string, []string) (metadata.MusicPictures, error) {
		t.Error("a kept package's pictures must not be asked of the server")
		return metadata.MusicPictures{}, nil
	}
	if v := a.PictureVersion("artist", syncArtist); v != "ph0to" {
		t.Fatalf("photo version %q", v)
	}
	if v := a.PictureVersion("wide", syncArtist); v != "w1de" {
		t.Fatalf("wide version %q", v)
	}
	if v := a.PictureVersion("album", syncAlbum); v != "c0ver" {
		t.Fatalf("cover version %q", v)
	}
	got := a.PictureRecords(context.Background(), []string{syncArtist}, []string{syncAlbum}, time.Second)
	if got["album|"+syncAlbum].Album.Cover == nil || got["artist|"+syncArtist].Artist.Photo == nil {
		t.Fatalf("records %+v", got)
	}

	// Explore lists an artist no package covers: it is wanted at the server.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go p.Run(ctx)
	deadline := time.Now().Add(3 * time.Second)
	for {
		a.wantPackages([]nameKey{{kind: "artist", mbid: syncArtist}, {kind: "artist", mbid: syncOther}})
		src.mu.Lock()
		n := len(src.wants)
		src.mu.Unlock()
		if n > 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	src.mu.Lock()
	defer src.mu.Unlock()
	if len(src.wants) != 1 || src.wants[0] != syncOther {
		t.Fatalf("wants %v", src.wants)
	}
}
