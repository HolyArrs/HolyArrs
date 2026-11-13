package main

import (
	"context"
	"slices"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles"
)

// startWired runs the app and hands back its store, parts control and the
// Job kinds that have a Handler.
func startWired(t *testing.T) (*database.Store, *parts.Control, []string) {
	t.Helper()
	var s *database.Store
	var pc *parts.Control
	var kinds []string
	startApp(t, config{ConfigDir: t.TempDir()}, runOpts{
		store: func(x *database.Store) { s = x },
		parts: func(c *parts.Control) { pc = c },
		kinds: func(k []string) { kinds = k },
	})
	if s == nil || pc == nil {
		t.Fatal("start-up did not hand over the store and parts")
	}
	return s, pc, kinds
}

func lookJobs(t *testing.T, s *database.Store) int {
	t.Helper()
	var n int
	if err := s.Read().QueryRow("SELECT count(*) FROM jobs WHERE kind = ?", subtitles.KindLook).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestStartup_SubtitleEventsQueueSearch(t *testing.T) {
	cases := []struct {
		name    string
		flags   string
		partOn  bool
		wantJob bool
	}{
		{"part on, no switch", "", true, true},
		{"part off", "", false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("HOLYARRS_FLAGS", c.flags)
			s, pc, kinds := startWired(t)
			for _, k := range []string{subtitles.KindSearch, subtitles.KindMissing, subtitles.KindBetter, subtitles.KindLook} {
				if !slices.Contains(kinds, k) {
					t.Errorf("job kind %s has no Handler", k)
				}
			}
			ctx := context.Background()
			if err := pc.Set(ctx, parts.Subtitles, c.partOn, domain.System); err != nil {
				t.Fatal(err)
			}
			base := lookJobs(t, s)
			// "adopted" asks for a look at the files no one has read yet.
			if err := s.AddEvent(ctx, &domain.Event{Kind: "adopted", Summary: "adopted"}); err != nil {
				t.Fatal(err)
			}
			// Event hooks run on AddEvent's caller, so the Job (if any) is
			// already queued here.
			n := lookJobs(t, s)
			if c.wantJob && n <= base {
				t.Fatal("the Event queued no subtitle look")
			}
			if !c.wantJob && n != base {
				t.Fatalf("a subtitle look was queued (%d -> %d)", base, n)
			}
		})
	}
}

// Startup hands the subtitle naming rules to library; without them library
// would leave a subtitle behind next to the old video.
func TestStartup_InjectsSubtitleNames(t *testing.T) {
	library.UseSubtitleNames(nil)
	t.Cleanup(func() { library.UseSubtitleNames(nil) })
	if library.CurrentSubtitleNames().IsBeside("movie", "movie.en.srt") {
		t.Fatal("the default rules already know subtitles")
	}
	startWired(t)
	n := library.CurrentSubtitleNames()
	if !n.IsBeside("movie", "movie.en.srt") || n.IsBeside("movie", "other.en.srt") {
		t.Fatal("start-up did not inject the subtitle naming rules")
	}
}
