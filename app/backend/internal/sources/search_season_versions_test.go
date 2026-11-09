package sources

import (
	"context"
	"database/sql"
	"net/url"
	"slices"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// addUHD gives the show fixture a 4K Version with one Slot per episode in the
// same state as the HD one.
func addUHD(t *testing.T, f showFix) (int64, map[string]int64) {
	t.Helper()
	ctx := context.Background()
	slots := map[string]int64{}
	var vid int64
	err := f.s.Write(ctx, func(tx *sql.Tx) error {
		var pr int64
		if err := tx.QueryRow("SELECT id FROM pick_rules WHERE name = ?", mediafiles.FourKName).Scan(&pr); err != nil {
			return err
		}
		main := mustVersion(t, f)
		v := &mediafiles.Version{TitleID: f.titleID, Label: "4K", PickRulesID: pr, LibraryFolderID: main.LibraryFolderID, FolderName: main.FolderName + " [4K]", KeepUpToDate: true}
		if _, err := database.Insert(tx, "versions", v, domain.System); err != nil {
			return err
		}
		vid = v.ID
		for label, id := range f.slots {
			var hd mediafiles.Slot
			if err := database.Get(ctx, tx, "slots", id, &hd); err != nil {
				return err
			}
			sl := &mediafiles.Slot{ItemID: hd.ItemID, VersionID: v.ID, State: hd.State}
			if _, err := database.Insert(tx, "slots", sl, domain.System); err != nil {
				return err
			}
			slots[label] = sl.ID
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return vid, slots
}

func verSlotState(t *testing.T, f showFix, id int64) mediafiles.SlotState {
	t.Helper()
	var st string
	if err := f.s.Read().QueryRow("SELECT state FROM slots WHERE id = ?", id).Scan(&st); err != nil {
		t.Fatal(err)
	}
	return mediafiles.SlotState(st)
}

// A season search runs per Version: the HD pack fills only HD Slots and the 4K
// pack only 4K Slots, each judged by its own pick rules.
func TestHandleSeason_PerVersion(t *testing.T) {
	ts := &tvServer{answer: func(url.Values) []string {
		return []string{
			"Copper.Valley.S01.1080p.WEB-DL.H.264-QUILL",
			"Copper.Valley.S01.2160p.WEB-DL.DV.HDR.H.265-QUILL|12000",
		}
	}}
	f := seedShow(t, ts.start(t), nil)
	uhd, uhdSlots := addUHD(t, f)
	var grabs [][2]int64
	sr := &Searcher{Store: f.s, OnChosen: func(_ context.Context, slotID, releaseID int64) { grabs = append(grabs, [2]int64{slotID, releaseID}) }}

	j := runKind(t, f.fixture, sr, jobs.KindSearchSeason, &domain.Subject{Type: "part", ID: f.parts[1]}, map[string]any{"version_id": f.verID})
	if j.State != "done" {
		t.Fatalf("HD job %s %+v", j.State, j.Error)
	}
	hdPack := relByName(t, f.fixture, "Copper.Valley.S01.1080p.WEB-DL.H.264-QUILL")
	for _, l := range []string{"S01E01", "S01E02", "S01E03"} {
		if st := f.state(t, l); st != mediafiles.SlotChosen {
			t.Fatalf("HD %s is %s", l, st)
		}
		if st := verSlotState(t, f, uhdSlots[l]); st != mediafiles.SlotWanted {
			t.Fatalf("the HD search touched 4K %s: %s", l, st)
		}
		if slices.Contains(PickSlots(hdPack), uhdSlots[l]) {
			t.Fatalf("the HD pack holds a 4K Slot: %v", PickSlots(hdPack))
		}
	}

	j = runKind(t, f.fixture, sr, jobs.KindSearchSeason, &domain.Subject{Type: "part", ID: f.parts[1]}, map[string]any{"version_id": uhd})
	if j.State != "done" {
		t.Fatalf("4K job %s %+v", j.State, j.Error)
	}
	var uhdPack domain.Release
	rs, _ := database.Select[domain.Release](context.Background(), f.s.Read(), "releases", "name = ? AND state = 'chosen'", "Copper.Valley.S01.2160p.WEB-DL.DV.HDR.H.265-QUILL")
	if len(rs) != 1 {
		t.Fatalf("4K pack chosen %d times", len(rs))
	}
	uhdPack = rs[0]
	for _, l := range []string{"S01E01", "S01E02", "S01E03"} {
		if st := verSlotState(t, f, uhdSlots[l]); st != mediafiles.SlotChosen {
			t.Fatalf("4K %s is %s", l, st)
		}
		if !slices.Contains(PickSlots(uhdPack), uhdSlots[l]) || slices.Contains(PickSlots(uhdPack), f.slots[l]) {
			t.Fatalf("4K pack Slots %v", PickSlots(uhdPack))
		}
	}
	if len(grabs) != 2 || grabs[0][1] != hdPack.ID || grabs[1][1] != uhdPack.ID {
		t.Fatalf("grabs %v", grabs)
	}
}

// The missing search queues one season search per Version.
func TestMissingSearch_PerVersion(t *testing.T) {
	ts := &tvServer{answer: func(url.Values) []string { return nil }}
	f := seedShow(t, ts.start(t), nil)
	uhd, _ := addUHD(t, f)
	sr := &Searcher{Store: f.s}
	if j := runKind(t, f.fixture, sr, jobs.KindSearchMissing, nil, nil); j.State != "done" {
		t.Fatalf("%s %+v", j.State, j.Error)
	}
	for _, v := range []int64{f.verID, uhd} {
		var n int
		_ = f.s.Read().QueryRow("SELECT count(*) FROM jobs WHERE kind = ? AND json_extract(params, '$.version_id') = ?", jobs.KindSearchSeason, v).Scan(&n)
		if n != 1 {
			t.Fatalf("Version %d: %d season searches", v, n)
		}
	}
}
