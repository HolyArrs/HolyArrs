package downloads

import (
	"database/sql"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

func TestQ1MinuteChecksStopAfterAnHour(t *testing.T) {
	last := retryNow.Add(-2 * time.Minute)
	for _, c := range []struct {
		tries int
		want  time.Duration
	}{{0, time.Minute}, {59, time.Minute}, {60, time.Hour}, {500, time.Hour}} {
		p := retryPlan([]mediafiles.WaitingImport{wi(1, 10, importer.NotPresentYetWhy, 30*24*time.Hour)}, map[int64]retryMark{10: {c.tries, last}})
		if got := p[10].Sub(last); c.tries > 0 && got != c.want {
			t.Fatalf("tries %d: wait %v, want %v", c.tries, got, c.want)
		}
	}
}

func TestQ1WrongPathPairKeepsHint(t *testing.T) {
	d := domain.Download{ID: 10, SlotIDs: []int64{1}}
	mk := func(r string) map[int64]slotInfo {
		return map[int64]slotInfo{1: {line: EpisodeLine{State: string(mediafiles.SlotNeedsADecision)}, active: sql.NullInt64{Int64: 10, Valid: true}, lastReason: r}}
	}
	if waitingForFiles(d, mk(notYetThere)) {
		t.Fatal("wrong path pair shown as waiting for files")
	}
	if !waitingForFiles(d, mk(importer.NotPresentYetWhy)) || !waitingForFiles(d, mk(importer.CopyingWhy)) {
		t.Fatal("real wait not shown")
	}
}
