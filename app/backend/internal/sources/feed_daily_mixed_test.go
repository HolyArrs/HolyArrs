package sources

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	model "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A daily show whose aired episodes are no longer kept up to date, with only
// the newest one kept: the feed check still takes that one's air-day release.
func TestFeed_DailyShowMixedKept(t *testing.T) {
	now := time.Now().UTC()
	newest := now.AddDate(0, 0, -1)
	lf := &liveFeed{}
	lf.set(feedOf(item("The Daily Show "+newest.Format("2006 01 02")+" Ron Howard 1080p WEB h264-EDITH", time.Now().Add(-time.Hour), 50, "")))
	f := seedShow(t, lf.url(t), nil)
	var newSlot int64
	err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE titles SET name = 'The Daily Show', sort_name = 'daily show', year = 1996, episode_order = 'daily' WHERE id = ?", f.titleID); err != nil {
			return err
		}
		p := &mediafiles.Part{TitleID: f.titleID, Kind: "season", Number: ptr(31), Name: "Season 31", KeepUpToDate: true,
			SourceIDs: map[string]string{}, Pressings: []mediafiles.Pressing{}}
		if _, err := database.Insert(tx, "parts", p, model.System); err != nil {
			return err
		}
		for n := 1; n <= 130; n++ {
			air := newest.AddDate(0, 0, n-129)
			day := air.Format("2006-01-02")
			it := &mediafiles.Item{TitleID: f.titleID, PartID: &p.ID, Kind: "episode", Numbers: mediafiles.Numbers{Season: ptr(31), Episode: ptr(n)},
				Name: ptr(fmt.Sprintf("Guest %d", n)), AirDate: &air, AirDay: &day, Runtime: ptr(45), KeepUpToDate: n >= 129, SourceIDs: map[string]string{}}
			if _, err := database.Insert(tx, "items", it, model.System); err != nil {
				return err
			}
			sl := &mediafiles.Slot{ItemID: it.ID, VersionID: f.verID, State: mediafiles.SlotWanted}
			if air.After(now) {
				sl.State = mediafiles.SlotNotOutYet
			}
			if _, err := database.Insert(tx, "slots", sl, model.System); err != nil {
				return err
			}
			if n == 129 {
				newSlot = sl.ID
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sr := &Searcher{Store: f.s}
	j := runKind(t, f.fixture, sr, jobs.KindFeedCheck, nil, nil)
	if j.State != "done" {
		t.Fatalf("feed job %s %v", j.State, j.Error)
	}
	var st, why string
	_ = f.s.Read().QueryRow("SELECT state, coalesce(last_reason,'') FROM slots WHERE id = ?", newSlot).Scan(&st, &why)
	if mediafiles.SlotState(st) != mediafiles.SlotChosen {
		rows, _ := f.s.Read().Query("SELECT name, state, coalesce(json_extract(verdict,'$.reasons'),'') FROM releases")
		for rows != nil && rows.Next() {
			var a, b, c string
			_ = rows.Scan(&a, &b, &c)
			t.Logf("release %s %s %s", a, b, c)
		}
		t.Fatalf("S31E129 %s (%s) result %+v", st, why, j.Result)
	}
}
