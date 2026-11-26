package sources

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// End to end: with Releases with several seasons on and both seasons
// missing, a season search takes the S01-S02 pack over the S01 pack of the
// same quality, so one Download fills every missing episode.
func TestHandleSeason_PrefersPackCoveringMoreSeasons(t *testing.T) {
	ts := &tvServer{answer: func(url.Values) []string {
		return []string{
			"Copper.Valley.S01.1080p.WEB-DL.H.264-QUILL",
			"Copper.Valley.S01-S02.1080p.WEB-DL.H.264-QUILL|6000",
		}
	}}
	f := seedShow(t, ts.start(t), func(label string, sl *mediafiles.Slot) {
		if label == "S02E02" {
			sl.State = mediafiles.SlotWanted
		}
	})
	past := time.Now().UTC().AddDate(0, 0, -1)
	exec(t, f.fixture, "UPDATE items SET air_date = ?, air_day = ? WHERE id = ?", past.Format(time.RFC3339Nano), past.Format("2006-01-02"), f.items["S02E02"])
	setSetting(t, f.fixture, SettingMultiSeason, "true")
	var grabs [][2]int64
	sr := &Searcher{Store: f.s, OnChosen: func(_ context.Context, slotID, releaseID int64) { grabs = append(grabs, [2]int64{slotID, releaseID}) }}
	j := runKind(t, f.fixture, sr, jobs.KindSearchSeason, &domain.Subject{Type: "part", ID: f.parts[1]}, map[string]any{"version_id": f.verID})
	if j.State != "done" {
		t.Fatalf("job %s %+v", j.State, j.Error)
	}
	multi := relByName(t, f.fixture, "Copper.Valley.S01-S02.1080p.WEB-DL.H.264-QUILL")
	if multi.State != "chosen" || len(PickSlots(multi)) != 5 {
		t.Fatalf("multi-season pack %s %v %v", multi.State, PickSlots(multi), reasonCodes(multi))
	}
	if r := relByName(t, f.fixture, "Copper.Valley.S01.1080p.WEB-DL.H.264-QUILL"); r.State != "passed_over" {
		t.Fatalf("one-season pack %s %v", r.State, reasonCodes(r))
	}
	for _, l := range []string{"S01E01", "S01E02", "S01E03", "S02E01", "S02E02"} {
		if st := f.state(t, l); st != mediafiles.SlotChosen {
			t.Fatalf("%s is %s", l, st)
		}
	}
	if len(grabs) != 1 || grabs[0][1] != multi.ID {
		t.Fatalf("one Download: %v", grabs)
	}
}

// Search all missing queues one season search per season. Once the first
// takes the S01-S02 pack, the season 2 search finds nothing looking and
// grabs nothing more, so the pack is the only Download.
func TestHandleSeason_NextSeasonSearchSkipsAfterMultiSeasonPack(t *testing.T) {
	ts := &tvServer{answer: func(url.Values) []string {
		return []string{
			"Copper.Valley.S01.1080p.WEB-DL.H.264-QUILL",
			"Copper.Valley.S02.1080p.WEB-DL.H.264-QUILL",
			"Copper.Valley.S01-S02.1080p.WEB-DL.H.264-QUILL|6000",
		}
	}}
	f := seedShow(t, ts.start(t), func(label string, sl *mediafiles.Slot) {
		if label == "S02E02" {
			sl.State = mediafiles.SlotWanted
		}
	})
	past := time.Now().UTC().AddDate(0, 0, -1)
	exec(t, f.fixture, "UPDATE items SET air_date = ?, air_day = ? WHERE id = ?", past.Format(time.RFC3339Nano), past.Format("2006-01-02"), f.items["S02E02"])
	setSetting(t, f.fixture, SettingMultiSeason, "true")
	var grabs [][2]int64
	sr := &Searcher{Store: f.s, OnChosen: func(_ context.Context, slotID, releaseID int64) { grabs = append(grabs, [2]int64{slotID, releaseID}) }}
	for _, p := range []int64{f.parts[1], f.parts[2]} {
		if j := runKind(t, f.fixture, sr, jobs.KindSearchSeason, &domain.Subject{Type: "part", ID: p}, map[string]any{"version_id": f.verID}); j.State != "done" {
			t.Fatalf("job %s %+v", j.State, j.Error)
		}
	}
	multi := relByName(t, f.fixture, "Copper.Valley.S01-S02.1080p.WEB-DL.H.264-QUILL")
	if len(grabs) != 1 || grabs[0][1] != multi.ID {
		t.Fatalf("one Download: %v", grabs)
	}
	for _, l := range []string{"S01E01", "S02E01", "S02E02"} {
		if st := f.state(t, l); st != mediafiles.SlotChosen {
			t.Fatalf("%s is %s", l, st)
		}
	}
}
