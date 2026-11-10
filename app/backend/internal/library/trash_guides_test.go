package library

import (
	"context"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library/trashtest"
)

// TestBuild: a profile brings its own formats and its default groups'
// ones; scores past the Pick rules bound are cut; a format with a test
// kind HolyArrs cannot run is skipped and named, never half-taken.
func TestBuild(t *testing.T) {
	c := NewTrashGuides()
	c.Raw, c.API = trashtest.Serve(t, trashtest.Files)
	d, err := c.Load(context.Background(), "sonarr")
	if err != nil {
		t.Fatal(err)
	}
	p, ok := d.Profile("p1")
	if !ok || p.Description != "Line one line two" || p.CutoffScore != 10000 {
		t.Fatalf("profile: %+v %v", p, ok)
	}
	plan := d.Build(p)
	got := map[string]int{}
	for _, pl := range plan.Traits {
		got[pl.Trait.Name] = pl.Points
		if pl.Trait.Source != "trash" || pl.Trait.SourceID == "" || len(pl.Trait.Specs) == 0 {
			t.Errorf("trait %+v lacks its source or tests", pl.Trait)
		}
	}
	want := map[string]int{"HEVC": -500, "Good group": 1700, "LQ": -10000}
	if len(got) != len(want) {
		t.Fatalf("traits %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %d, want %d", k, got[k], v)
		}
	}
	if len(plan.Skipped) != 1 || plan.Skipped[0].Name != "Lang only" || len(plan.Skipped[0].Kinds) != 1 || plan.Skipped[0].Kinds[0] != "language" {
		t.Fatalf("skipped: %+v", plan.Skipped)
	}
	for _, pl := range plan.Traits {
		if pl.Trait.Name == "Good group" {
			if s := pl.Trait.Specs[1]; s.Kind != "source" || s.Value != "web_dl" {
				t.Errorf("source test: %+v", s)
			}
		}
	}
}

// TestLoadUnknownApp refuses apps other than sonarr and radarr.
func TestLoadUnknownApp(t *testing.T) {
	if _, err := NewTrashGuides().Load(context.Background(), "lidarr"); err == nil {
		t.Fatal("lidarr was accepted")
	}
}
