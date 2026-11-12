package sources

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A season pack chosen but never taken by a Downloader keeps its
// episodes chosen. Right away they stay busy; after ChoiceWait another
// Release can be chosen for them by hand.
func TestChoose_StuckChoiceCanBeReplaced(t *testing.T) {
	ts := &tvServer{answer: func(url.Values) []string {
		return []string{"Copper.Valley.S01.720p.HDTV.x264-OXBOW", "Copper.Valley.S01.1080p.WEB-DL.H.264-QUILL"}
	}}
	f := seedShow(t, ts.start(t), nil)
	sr := &Searcher{Store: f.s}
	runKind(t, f.fixture, sr, jobs.KindSearchSeason, &domain.Subject{Type: "part", ID: f.parts[1]}, map[string]any{"version_id": f.verID, "choose": false, "by_hand": true})
	first := relByName(t, f.fixture, "Copper.Valley.S01.720p.HDTV.x264-OXBOW")
	second := relByName(t, f.fixture, "Copper.Valley.S01.1080p.WEB-DL.H.264-QUILL")
	if _, err := sr.Choose(context.Background(), first.ID, 0, true, domain.System); err != nil {
		t.Fatal(err)
	}
	if _, err := sr.Choose(context.Background(), second.ID, 0, true, domain.System); !errors.Is(err, ErrBusy) {
		t.Fatalf("a fresh choice was replaced: %v", err)
	}
	var sl mediafiles.Slot
	if err := database.Get(context.Background(), f.s.Read(), "slots", f.slots["S01E02"], &sl); err != nil || StuckChoice(sl, time.Now()) {
		t.Fatalf("fresh choice counts as stuck: %v", err)
	}
	old := database.FormatTime(time.Now().Add(-ChoiceWait - time.Minute))
	if err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE slots SET updated_at = ? WHERE state = 'chosen'", old)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := database.Get(context.Background(), f.s.Read(), "slots", f.slots["S01E02"], &sl); err != nil || !StuckChoice(sl, time.Now()) {
		t.Fatalf("old choice not stuck: %v", err)
	}
	ch, err := sr.Choose(context.Background(), second.ID, 0, true, domain.System)
	if err != nil || len(ch.SlotIDs) != 3 {
		t.Fatalf("replace the stuck choice: %+v %v", ch, err)
	}
	for _, l := range []string{"S01E01", "S01E02", "S01E03"} {
		var got mediafiles.Slot
		if err := database.Get(context.Background(), f.s.Read(), "slots", f.slots[l], &got); err != nil {
			t.Fatal(err)
		}
		if got.State != mediafiles.SlotChosen || got.ChosenReleaseID == nil || *got.ChosenReleaseID != second.ID {
			t.Fatalf("%s is %s for %v", l, got.State, got.ChosenReleaseID)
		}
	}
}

// A chosen Slot with a Download on the way is never stuck.
func TestStuckChoice_ActiveDownloadKeepsIt(t *testing.T) {
	id := int64(4)
	sl := mediafiles.Slot{State: mediafiles.SlotChosen, ActiveDownloadID: &id}
	sl.UpdatedAt = time.Now().Add(-time.Hour)
	if StuckChoice(sl, time.Now()) {
		t.Fatal("a Slot with a Download counts as stuck")
	}
	sl.ActiveDownloadID = nil
	if !StuckChoice(sl, time.Now()) {
		t.Fatal("an old choice with no Download is not stuck")
	}
	sl.State = mediafiles.SlotDownloading
	if StuckChoice(sl, time.Now()) {
		t.Fatal("a downloading Slot counts as stuck")
	}
}
