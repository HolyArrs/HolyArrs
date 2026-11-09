package music

import (
	"context"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// musicOffControl is a parts Control with Music turned off.
func musicOffControl(t *testing.T, s *database.Store) *parts.Control {
	t.Helper()
	pc := &parts.Control{Store: s, Reg: jobs.NewRegistry(), Drain: time.Second, Logf: func(string, ...any) {}}
	if err := pc.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := pc.Set(context.Background(), parts.Music, false, domain.System); err != nil {
		t.Fatal(err)
	}
	return pc
}

// With Music off, the music loops are added to the parts Control but none
// starts: no music goroutine runs and the metadata server is never asked.
// Turning Music on starts them.
func TestMusicWarmUpDoesNotRunWhenMusicIsOff(t *testing.T) {
	t.Parallel()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	var calls atomic.Int32
	a := newTestAPI(Server{Store: s, Parts: musicOffControl(t, s), Version: "test",

		Deps: Deps{MusicTrending: func(context.Context, string, string) (metadata.MusicTrend, error) {
			calls.Add(1)
			return metadata.MusicTrend{}, nil
		}, MusicCharts: func(context.Context, string, string, string) (metadata.MusicChart, error) {
			calls.Add(1)
			return metadata.MusicChart{}, nil
		}},
	})
	pc := a.s.Parts
	for _, l := range a.MusicLoops() {
		if owner := parts.Loops()[l.Name]; owner != parts.Of(parts.Music) {
			t.Fatalf("loop %s belongs to %q, not Music", l.Name, owner)
		}
		pc.Add(l, true)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pc.Start(ctx)
	time.Sleep(musicWarmDelay + time.Second)
	if n := calls.Load(); n != 0 {
		t.Fatalf("music warm-up asked the metadata server %d times with Music off", n)
	}
	if g := musicGoroutines(); g != "" {
		t.Fatalf("a music goroutine runs with Music off:\n%s", g)
	}
	if err := pc.Set(context.Background(), parts.Music, true, domain.System); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for musicGoroutines() == "" {
		if time.Now().After(deadline) {
			t.Fatal("turning Music on did not start the music loops")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// musicGoroutines returns the stacks of running goroutines that are in a
// music loop, or "".
func musicGoroutines() string {
	buf := make([]byte, 1<<22)
	buf = buf[:runtime.Stack(buf, true)]
	var out []string
	for _, g := range strings.Split(string(buf), "\n\n") {
		if strings.Contains(g, "MusicLoops") || strings.Contains(g, ".warmMusic(") {
			out = append(out, g)
		}
	}
	return strings.Join(out, "\n\n")
}

// With Music off, the music Job kinds (artist refresh, artist pictures,
// album search) are not queued.
func TestMusicJobKindsAreOffWhenMusicIsOff(t *testing.T) {
	t.Parallel()
	s, err := database.Open(t.TempDir(), database.Options{})
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

// With Music off, the shared handlers' Music provider gives nothing and
// starts no build; turned on, it answers again.
func TestMusicProviderFollowsThePart(t *testing.T) {
	t.Parallel()
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	a := newTestAPI(Server{Store: s, Parts: musicOffControl(t, s), Version: "test"})
	if a.MusicOn() {
		t.Fatal("the Music provider answers with Music off")
	}
	a.WarmNew()
	if g := musicGoroutines(); g != "" {
		t.Fatalf("WarmNew started music work with Music off:\n%s", g)
	}
	if err := a.s.Parts.Set(context.Background(), parts.Music, true, domain.System); err != nil {
		t.Fatal(err)
	}
	if !a.MusicOn() {
		t.Fatal("the Music provider stays off with Music on")
	}
}
