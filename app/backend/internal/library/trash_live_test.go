package library

import (
	"time"

	"context"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"os"
	"testing"
)

// TestLive reads the real TRaSH Guides; run with HOLYARRS_TRASH_LIVE=1.
func TestLive(t *testing.T) {
	if os.Getenv("HOLYARRS_TRASH_LIVE") == "" {
		t.Skip("set HOLYARRS_TRASH_LIVE=1 to read the real TRaSH Guides")
	}
	c := NewTrashGuides()
	for _, app := range []string{"sonarr", "radarr"} {
		d, err := c.Load(context.Background(), app)
		if err != nil {
			t.Fatal(err)
		}
		if len(d.Profiles) == 0 || len(d.Formats) == 0 {
			t.Fatalf("%s: %d profiles, %d formats", app, len(d.Profiles), len(d.Formats))
		}
		for _, p := range d.Profiles {
			plan := d.Build(p)
			t.Logf("%s %-32s scored %2d skipped %2d", app, p.Name, len(plan.Traits), len(plan.Skipped))
			for _, s := range plan.Skipped {
				if len(s.Kinds) > 0 && s.Kinds[0] == "unreadable pattern" {
					t.Errorf("%s: %s has a pattern regexp2 cannot read", app, s.Name)
				}
			}
		}
	}
}

// TestLiveScores scores real-looking Release names with the real
// WEB-1080p profile; run with HOLYARRS_TRASH_LIVE=1.
func TestLiveScores(t *testing.T) {
	if os.Getenv("HOLYARRS_TRASH_LIVE") == "" {
		t.Skip("set HOLYARRS_TRASH_LIVE=1 to read the real TRaSH Guides")
	}
	d, err := NewTrashGuides().Load(context.Background(), "sonarr")
	if err != nil {
		t.Fatal(err)
	}
	var prof TrashProfile
	for _, p := range d.Profiles {
		if p.Name == "WEB-1080p" {
			prof = p
		}
	}
	plan := d.Build(prof)
	var ts []mediafiles.OwnTrait
	body := mediafiles.GoodEveryday()
	for i, pl := range plan.Traits {
		tr := pl.Trait
		tr.ID = int64(i + 1)
		ts = append(ts, tr)
		body.Traits = append(body.Traits, mediafiles.TraitPoints{Trait: mediafiles.OwnTraitKey(tr.ID), Points: pl.Points})
	}
	body.Traits = body.Traits[len(mediafiles.GoodEveryday().Traits):]
	mediafiles.SetOwnTraits(ts)
	t.Cleanup(func() { mediafiles.SetOwnTraits(nil) })
	for _, name := range []string{
		"Arcane.S01E01.1080p.NF.WEB-DL.DDP5.1.H.264-FLUX",
		"Arcane.S01E01.1080p.AMZN.WEB-DL.DDP5.1.H.264-NTb",
		"Arcane.S01E01.1080p.WEB.H264-HOLYARRS",
		"Arcane.S01E01.1080p.WEB-DL.x265-YIFY",
		"Arcane.S01E01.1080p.WEBRip.AV1-LAMA",
	} {
		t.Logf("%6d  %s", mediafiles.CurrentOf(name, 0, time.Time{}, body).Points, name)
	}
}
