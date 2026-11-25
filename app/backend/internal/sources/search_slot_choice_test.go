package sources

import (
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A Slot's own choice wins over its episode and season in the missing
// search: an episode switched off is still searched where its Version
// wants it, and a kept season is left alone where its Version does not.
func TestMissingSearchFollowsSlotChoice(t *testing.T) {
	on, off := true, false
	f := seedShow(t, "http://127.0.0.1:1/api", func(label string, sl *mediafiles.Slot) {
		switch label {
		case "S01E01", "S01E02", "S01E03":
			sl.KeepUpToDate = &off
		case "S02E01":
			sl.KeepUpToDate = &on
		}
	})
	exec(t, f.fixture, "UPDATE items SET keep_up_to_date = 0 WHERE id = ?", f.items["S02E01"])
	j := runKind(t, f.fixture, &Searcher{Store: f.s}, jobs.KindSearchMissing, nil, nil)
	if j.State != "done" {
		t.Fatalf("%s %+v", j.State, j.Error)
	}
	if n := jobCount(t, f.fixture, jobs.KindSearchSeason, f.parts[1]); n != 0 {
		t.Fatalf("season 1 searched %d", n)
	}
	for _, k := range []string{"S01E01", "S01E02", "S01E03"} {
		if n := jobCount(t, f.fixture, jobs.KindSearchSlot, f.slots[k]); n != 0 {
			t.Fatalf("%s searched %d", k, n)
		}
	}
	if n := jobCount(t, f.fixture, jobs.KindSearchSlot, f.slots["S02E01"]); n != 1 {
		t.Fatalf("S02E01 jobs %d", n)
	}
}

func TestSlotKept(t *testing.T) {
	on, off := true, false
	for _, c := range []struct {
		own              *bool
		item, ver, wants bool
	}{
		{nil, true, true, true}, {nil, true, false, false}, {nil, false, true, false},
		{&on, false, false, true}, {&off, true, true, false},
	} {
		if got := mediafiles.SlotKept(mediafiles.Slot{KeepUpToDate: c.own}, c.item, c.ver); got != c.wants {
			t.Errorf("%+v: got %v", c, got)
		}
	}
}
