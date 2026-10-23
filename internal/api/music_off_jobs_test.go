package api

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs/internal/domain"
	"github.com/CoderLuii/holyarrs/internal/metadata"
	"github.com/CoderLuii/holyarrs/internal/parts"
	"github.com/CoderLuii/holyarrs/internal/store"
	"github.com/CoderLuii/holyarrs/internal/supervisor"
)

// musicOffControl is a parts Control with Music turned off.
func musicOffControl(t *testing.T, s *store.Store) *parts.Control {
	t.Helper()
	pc := &parts.Control{Store: s, Reg: supervisor.NewRegistry(), Drain: time.Second, Logf: func(string, ...any) {}}
	if err := pc.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := pc.Set(context.Background(), parts.Music, false, domain.System); err != nil {
		t.Fatal(err)
	}
	return pc
}

// With Music off, the music chart and cover warm-up reads nothing from the
// metadata server.
func TestMusicWarmUpDoesNotRunWhenMusicIsOff(t *testing.T) {
	s, err := store.Open(t.TempDir(), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	var calls atomic.Int32
	a := New(Server{Store: s, Parts: musicOffControl(t, s), Version: "test",
		MusicTrending: func(context.Context, string, string) (metadata.MusicTrend, error) {
			calls.Add(1)
			return metadata.MusicTrend{}, nil
		},
		MusicCharts: func(context.Context, string, string, string) (metadata.MusicChart, error) {
			calls.Add(1)
			return metadata.MusicChart{}, nil
		},
		MusicCover: func(context.Context, string, string, string) (string, error) {
			calls.Add(1)
			return "", nil
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), musicWarmDelay+3*time.Second)
	defer cancel()
	a.warmMusic(ctx)
	if n := calls.Load(); n != 0 {
		t.Fatalf("music warm-up asked the metadata server %d times with Music off", n)
	}
}

// With Music off, the music Job kinds (artist refresh, artist pictures,
// album search) are not queued.
func TestMusicJobKindsAreOffWhenMusicIsOff(t *testing.T) {
	s, err := store.Open(t.TempDir(), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	pc := musicOffControl(t, s)
	for _, kind := range []string{"refresh_artists", "artist_pictures", "search_album"} {
		if !pc.KindOff(kind) {
			t.Errorf("%s would still be queued with Music off", kind)
		}
	}
}
