package parts

import (
	"context"
	"errors"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
)

// Hands-off mode turns Subtitles off without touching the saved switches,
// refuses turning it on, survives a restart, and turning the mode off
// brings back exactly what was there before.
func TestHandsOffTurnsSubtitlesOff(t *testing.T) {
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	ctx := context.Background()
	c := &Control{Store: s, Reg: jobs.NewRegistry()}
	if err := c.Load(ctx); err != nil {
		t.Fatal(err)
	}
	if HandsOff(ctx, s.Read()) {
		t.Fatal("hands-off is on in a fresh install")
	}
	steps := []struct {
		name      string
		do        func() error
		wantErr   error
		handsOff  bool
		subtitles bool
	}{
		{"turn on", func() error { return SetHandsOff(ctx, s, c, true, domain.System) }, nil, true, false},
		{"subtitles refused", func() error { return c.Set(ctx, Subtitles, true, domain.System) }, ErrHandsOffSubtitles, true, false},
		{"other parts still switch", func() error { return c.Set(ctx, Music, false, domain.System) }, nil, true, false},
		{"turn off", func() error { return SetHandsOff(ctx, s, c, false, domain.System) }, nil, false, true},
		{"on again", func() error { return SetHandsOff(ctx, s, c, true, domain.System) }, nil, true, false},
	}
	for _, st := range steps {
		if err := st.do(); !errors.Is(err, st.wantErr) {
			t.Fatalf("%s: err %v, want %v", st.name, err, st.wantErr)
		}
		if got := HandsOff(ctx, s.Read()); got != st.handsOff {
			t.Fatalf("%s: hands-off %v", st.name, got)
		}
		if got := c.On(Subtitles); got != st.subtitles {
			t.Fatalf("%s: subtitles on %v", st.name, got)
		}
	}
	for _, p := range c.Off() {
		if p != Subtitles && p != Music {
			t.Fatalf("%s off with hands-off", p)
		}
	}
	again := &Control{Store: s, Reg: jobs.NewRegistry()}
	if err := again.Load(ctx); err != nil {
		t.Fatal(err)
	}
	if again.On(Subtitles) || !again.Active() {
		t.Fatal("Subtitles came back on after a restart with hands-off on")
	}
	for _, st := range again.Statuses() {
		if st.Key == "part:subtitles" && st.Reason != HandsOffSubtitlesReason {
			t.Fatalf("subtitles status reason %q", st.Reason)
		}
	}
}

// A restart keeps Subtitles off in hands-off mode even when no part switch
// was ever saved (the usual case after first-run setup).
func TestHandsOffSurvivesRestartWithNoSavedSwitches(t *testing.T) {
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	ctx := context.Background()
	if err := SetHandsOff(ctx, s, nil, true, domain.System); err != nil {
		t.Fatal(err)
	}
	c := &Control{Store: s, Reg: jobs.NewRegistry()}
	if err := c.Load(ctx); err != nil {
		t.Fatal(err)
	}
	if c.On(Subtitles) || !c.HandsOffOn() {
		t.Fatal("Subtitles came back on after a restart with hands-off on and no saved switches")
	}
}

// Every route and Job kind hands-off mode holds is a real one, so a rename
// elsewhere cannot quietly let file work through.
func TestHandsOffListsAreReal(t *testing.T) {
	for pattern := range HandsOffRoutes {
		if _, ok := RouteOwner(pattern); !ok {
			t.Errorf("hands-off route %q is not in the route table", pattern)
		}
	}
	for kind := range HandsOffKinds {
		if _, ok := startup.OwnerOf(startup.JobOwners, kind); !ok {
			t.Errorf("hands-off job kind %q is not a known kind", kind)
		}
	}
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	ctx := context.Background()
	c := &Control{Store: s, Reg: jobs.NewRegistry()}
	if err := c.Load(ctx); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		kind string
		on   bool
		held bool
	}{
		{"rename_files", false, false}, {"rename_files", true, true}, {"scan_library_folder", true, true},
		{"import_download", true, false}, {"grab", true, false}, {"search_slot", true, false}, {"subtitle_search", true, true},
	}
	for _, tc := range cases {
		if err := SetHandsOff(ctx, s, c, tc.on, domain.System); err != nil {
			t.Fatal(err)
		}
		if got := c.KindOff(tc.kind); got != tc.held {
			t.Errorf("%s with hands-off %v: off %v, want %v", tc.kind, tc.on, got, tc.held)
		}
		if got := c.Held(ctx, s.Read(), tc.kind, nil); got != tc.held {
			t.Errorf("%s with hands-off %v: held %v, want %v", tc.kind, tc.on, got, tc.held)
		}
	}
}
