package sources

import (
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A feed Release refused for too few seeders is judged again on the next feed
// check, and chosen once it has enough. Seeders, age and size change over
// time, so a refusal is not the last word.
func TestFeed_RefusedReleaseJudgedAgain(t *testing.T) {
	name := "Copper.Valley.S01E03.1080p.WEB.h264-EDITH"
	pub := time.Now().Add(-time.Hour)
	lf := &liveFeed{}
	lf.set(feedOf(item(name, pub, 0, "")))
	f := seedShow(t, lf.url(t), func(label string, sl *mediafiles.Slot) {
		if label != "S01E03" {
			sl.State = mediafiles.SlotInLibrary
		}
	})
	exec(t, f.fixture, "UPDATE sources SET min_seeders = 5 WHERE id = ?", f.srcID)
	sr := &Searcher{Store: f.s}

	j := runKind(t, f.fixture, sr, jobs.KindFeedCheck, nil, nil)
	if j.State != "done" {
		t.Fatalf("feed job %s %v", j.State, j.Error)
	}
	if n, _ := j.Result["chosen"].(float64); n != 0 {
		t.Fatalf("chosen with 0 seeders: %+v", j.Result)
	}
	var state string
	if err := f.s.Read().QueryRow("SELECT state FROM releases WHERE name = ?", name).Scan(&state); err != nil || state != "refused" {
		t.Fatalf("first check stored %q (%v), want refused", state, err)
	}

	lf.set(feedOf(item(name, pub, 20, "")))
	j = runKind(t, f.fixture, sr, jobs.KindFeedCheck, nil, nil)
	if j.State != "done" {
		t.Fatalf("feed job %s %v", j.State, j.Error)
	}
	if n, _ := j.Result["chosen"].(float64); n != 1 {
		t.Fatalf("second check did not choose the Release: %+v", j.Result)
	}
	var rows, seeders int
	if err := f.s.Read().QueryRow("SELECT COUNT(*), MAX(seeders) FROM releases WHERE name = ?", name).Scan(&rows, &seeders); err != nil {
		t.Fatal(err)
	}
	if rows != 1 || seeders != 20 {
		t.Fatalf("got %d records with %d seeders, want 1 with 20", rows, seeders)
	}
}
