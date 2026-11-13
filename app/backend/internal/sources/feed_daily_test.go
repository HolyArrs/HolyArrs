package sources

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A daily show's feed Release named by its air day is matched the way a
// search matches it.
func TestFeed_DailyShowByAirDay(t *testing.T) {
	day := time.Now().UTC().AddDate(0, 0, -3)
	lf := &liveFeed{}
	lf.set(feedOf(item("Copper Valley "+day.Format("2006 01 02")+" Ron Howard 1080p WEB h264-EDITH", time.Now().Add(-time.Hour), 50, "")))
	f := seedShow(t, lf.url(t), nil)
	_ = f.s.Write(context.Background(), func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE titles SET episode_order = 'daily' WHERE id = ?", f.titleID); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE items SET air_day = ? WHERE id = ?", day.Format("2006-01-02"), f.items["S02E01"])
		return err
	})
	sr := &Searcher{Store: f.s}
	j := runKind(t, f.fixture, sr, jobs.KindFeedCheck, nil, nil)
	if j.State != "done" {
		t.Fatalf("feed job %s %v", j.State, j.Error)
	}
	if st := f.state(t, "S02E01"); st != mediafiles.SlotChosen {
		var why string
		_ = f.s.Read().QueryRow("SELECT coalesce(last_reason,'') FROM slots WHERE id = ?", f.slots["S02E01"]).Scan(&why)
		t.Fatalf("daily feed episode %s (%s) result %+v", st, why, j.Result)
	}
}
