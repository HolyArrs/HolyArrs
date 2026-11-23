package sources

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// threeSeasonShow is the show fixture with every episode aired and wanted,
// plus a third season of two aired episodes, all three seasons missing.
func threeSeasonShow(t *testing.T, srcURL string) showFix {
	t.Helper()
	f := seedShow(t, srcURL, func(label string, sl *mediafiles.Slot) { sl.State = mediafiles.SlotWanted })
	past := time.Now().UTC().AddDate(0, 0, -1)
	exec(t, f.fixture, "UPDATE items SET air_date = ?, air_day = ? WHERE id = ?", past.Format(time.RFC3339Nano), past.Format("2006-01-02"), f.items["S02E02"])
	err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		p := &mediafiles.Part{TitleID: f.titleID, Kind: "season", Number: ptr(3), Name: "Season 3", KeepUpToDate: true,
			SourceIDs: map[string]string{}, Pressings: []mediafiles.Pressing{}}
		if _, err := database.Insert(tx, "parts", p, domain.System); err != nil {
			return err
		}
		f.parts[3] = p.ID
		for n := 1; n <= 2; n++ {
			air := past.AddDate(0, 0, -n)
			day := air.Format("2006-01-02")
			it := &mediafiles.Item{TitleID: f.titleID, PartID: &p.ID, Kind: "episode", Numbers: mediafiles.Numbers{Season: ptr(3), Episode: ptr(n)},
				Name: ptr(fmt.Sprintf("Episode %d", n)), AirDate: &air, AirDay: &day, Runtime: ptr(45), KeepUpToDate: true, SourceIDs: map[string]string{}}
			if _, err := database.Insert(tx, "items", it, domain.System); err != nil {
				return err
			}
			sl := &mediafiles.Slot{ItemID: it.ID, VersionID: f.verID, State: mediafiles.SlotWanted}
			if _, err := database.Insert(tx, "slots", sl, domain.System); err != nil {
				return err
			}
			label := fmt.Sprintf("S03E%02d", n)
			f.slots[label], f.items[label] = sl.ID, it.ID
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	setSetting(t, f.fixture, SettingMultiSeason, "true")
	return f
}

var allSevenEps = []string{"S01E01", "S01E02", "S01E03", "S02E01", "S02E02", "S03E01", "S03E02"}

// runSeasons runs the season searches search all missing queues, in the
// given order, and returns the release IDs grabbed.
func runSeasons(t *testing.T, f showFix, order ...int) []int64 {
	t.Helper()
	var grabs []int64
	sr := &Searcher{Store: f.s, OnChosen: func(_ context.Context, _, releaseID int64) { grabs = append(grabs, releaseID) }}
	for _, s := range order {
		if j := runKind(t, f.fixture, sr, jobs.KindSearchSeason, &domain.Subject{Type: "part", ID: f.parts[s]}, map[string]any{"version_id": f.verID, "bulk": true}); j.State != "done" {
			t.Fatalf("season %d job %s %+v", s, j.State, j.Error)
		}
	}
	return grabs
}

// Three seasons missing and one S01-S03 pack: whichever season search runs
// first takes the pack, and the other two find nothing looking.
func TestSearchAllMissing_ThreeSeasonPackIsTheOnlyGrab(t *testing.T) {
	for _, order := range [][]int{{1, 2, 3}, {3, 2, 1}, {2, 1, 3}} {
		t.Run(fmt.Sprint(order), func(t *testing.T) {
			ts := &tvServer{answer: func(url.Values) []string {
				return []string{
					"Copper.Valley.S01.1080p.WEB-DL.H.264-QUILL",
					"Copper.Valley.S02.1080p.WEB-DL.H.264-QUILL",
					"Copper.Valley.S03.1080p.WEB-DL.H.264-QUILL",
					"Copper.Valley.S01-S03.1080p.WEB-DL.H.264-QUILL|9000",
				}
			}}
			f := threeSeasonShow(t, ts.start(t))
			grabs := runSeasons(t, f, order...)
			multi := relByName(t, f.fixture, "Copper.Valley.S01-S03.1080p.WEB-DL.H.264-QUILL")
			if len(grabs) != 1 || grabs[0] != multi.ID {
				t.Fatalf("want only the S01-S03 pack, got %v (pack %d)", grabs, multi.ID)
			}
			for _, l := range allSevenEps {
				if st := f.state(t, l); st != mediafiles.SlotChosen {
					t.Fatalf("%s is %s", l, st)
				}
			}
		})
	}
}

// A pack covering seasons 1 and 2 leaves season 3 to its own season pack in
// the same run, whichever order the season searches run in.
func TestSearchAllMissing_TwoSeasonPackLeavesThirdSeason(t *testing.T) {
	for _, order := range [][]int{{1, 2, 3}, {3, 2, 1}, {2, 3, 1}} {
		t.Run(fmt.Sprint(order), func(t *testing.T) {
			ts := &tvServer{answer: func(url.Values) []string {
				return []string{
					"Copper.Valley.S01.1080p.WEB-DL.H.264-QUILL",
					"Copper.Valley.S02.1080p.WEB-DL.H.264-QUILL",
					"Copper.Valley.S03.1080p.WEB-DL.H.264-QUILL",
					"Copper.Valley.S01-S02.1080p.WEB-DL.H.264-QUILL|6000",
				}
			}}
			f := threeSeasonShow(t, ts.start(t))
			grabs := runSeasons(t, f, order...)
			// Each season search stores its own rows, so grabs are matched by name.
			got := map[string]int{}
			for _, g := range grabs {
				var name string
				if err := f.s.Read().QueryRow("SELECT name FROM releases WHERE id = ?", g).Scan(&name); err != nil {
					t.Fatal(err)
				}
				got[name]++
			}
			if len(grabs) != 2 || got["Copper.Valley.S01-S02.1080p.WEB-DL.H.264-QUILL"] != 1 || got["Copper.Valley.S03.1080p.WEB-DL.H.264-QUILL"] != 1 {
				t.Fatalf("want the S01-S02 pack and the S03 pack once each, got %v", got)
			}
			for _, l := range allSevenEps {
				if st := f.state(t, l); st != mediafiles.SlotChosen {
					t.Fatalf("%s is %s", l, st)
				}
			}
		})
	}
}
