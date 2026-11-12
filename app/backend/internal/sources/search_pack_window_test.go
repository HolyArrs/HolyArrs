package sources

import (
	"context"
	"database/sql"
	"net/url"
	"slices"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// packWindowShow sets the fixture show to take season packs two episodes ahead.
func packWindowShow(t *testing.T, f showFix) {
	t.Helper()
	if err := f.s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE titles SET pack_window = 2 WHERE id = ?", f.titleID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func ready(ok bool) func(context.Context, []string) bool {
	return func(context.Context, []string) bool { return ok }
}

func TestHandleSeason_PackWindowQueuesTheFirstEpisodes(t *testing.T) {
	ts := &tvServer{answer: func(url.Values) []string { return []string{"Copper.Valley.S01.1080p.WEB-DL.H.264-QUILL"} }}
	f := seedShow(t, ts.start(t), nil)
	packWindowShow(t, f)
	sr := &Searcher{Store: f.s, PackReady: ready(true)}
	j := runKind(t, f.fixture, sr, jobs.KindSearchSeason, &domain.Subject{Type: "part", ID: f.parts[1]}, map[string]any{"version_id": f.verID})
	if n, _ := j.Result["episodes_queued"].(float64); n != 2 {
		t.Fatalf("result %+v", j.Result)
	}
	for _, q := range ts.queries {
		if !q.Has("ep") {
			t.Fatalf("a season query: %v", q)
		}
	}
	var queued int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM jobs WHERE kind = ? AND json_extract(subject, '$.id') IN (?, ?)", jobs.KindSearchSlot,
		f.slots["S01E01"], f.slots["S01E02"]).Scan(&queued)
	if queued != 2 {
		t.Fatalf("queued %d of the first two", queued)
	}
}

func TestHandleSeason_PackWindowWithoutQbitTakesTheSeason(t *testing.T) {
	ts := &tvServer{answer: func(url.Values) []string { return []string{"Copper.Valley.S01.1080p.WEB-DL.H.264-QUILL"} }}
	f := seedShow(t, ts.start(t), nil)
	packWindowShow(t, f)
	sr := &Searcher{Store: f.s, PackReady: func(context.Context, []string) bool { return false }}
	runKind(t, f.fixture, sr, jobs.KindSearchSeason, &domain.Subject{Type: "part", ID: f.parts[1]}, map[string]any{"version_id": f.verID})
	pack := relByName(t, f.fixture, "Copper.Valley.S01.1080p.WEB-DL.H.264-QUILL")
	if pack.State != "chosen" || len(PickSlots(pack)) != 3 {
		t.Fatalf("whole season as before: %s %v %v", pack.State, PickSlots(pack), reasonCodes(pack))
	}
}

func TestSearch_PackWindowTakesThePackForTheWindow(t *testing.T) {
	ts := &tvServer{answer: func(url.Values) []string {
		return []string{"Copper.Valley.S01.1080p.WEB-DL.H.264-QUILL", "Copper.Valley.S01-S02.1080p.WEB-DL.H.264-QUILL|6000"}
	}}
	f := seedShow(t, ts.start(t), nil)
	packWindowShow(t, f)
	sr := &Searcher{Store: f.s, PackReady: ready(true)}
	runKind(t, f.fixture, sr, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slots["S01E01"]}, nil)
	pack := relByName(t, f.fixture, "Copper.Valley.S01.1080p.WEB-DL.H.264-QUILL")
	if pack.State != "chosen" || !slices.Equal(PickSlots(pack), []int64{f.slots["S01E01"], f.slots["S01E02"]}) {
		t.Fatalf("pack %s %v %v", pack.State, PickSlots(pack), reasonCodes(pack))
	}
	if f.state(t, "S01E03") != mediafiles.SlotWanted {
		t.Fatalf("S01E03 is %s", f.state(t, "S01E03"))
	}
	if r := relByName(t, f.fixture, "Copper.Valley.S01-S02.1080p.WEB-DL.H.264-QUILL"); r.State != "refused" {
		t.Fatalf("several seasons: %s %v", r.State, reasonCodes(r))
	}
}

func TestSearch_PackWindowPrefersTheSingle(t *testing.T) {
	ts := &tvServer{answer: func(url.Values) []string {
		return []string{"Copper.Valley.S01.2160p.WEB-DL.H.265-QUILL|9000", "Copper.Valley.S01E01.720p.HDTV.x264-OXBOW"}
	}}
	f := seedShow(t, ts.start(t), nil)
	packWindowShow(t, f)
	sr := &Searcher{Store: f.s, PackReady: ready(true)}
	runKind(t, f.fixture, sr, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slots["S01E01"]}, nil)
	if r := relByName(t, f.fixture, "Copper.Valley.S01E01.720p.HDTV.x264-OXBOW"); r.State != "chosen" {
		t.Fatalf("single %s %v", r.State, reasonCodes(r))
	}
	if r := relByName(t, f.fixture, "Copper.Valley.S01.2160p.WEB-DL.H.265-QUILL"); r.State == "chosen" {
		t.Fatalf("pack chosen over a passing single: %v", reasonCodes(r))
	}
}

func TestSearch_PackWindowOffRefusesPacks(t *testing.T) {
	ts := &tvServer{answer: func(url.Values) []string { return []string{"Copper.Valley.S01.1080p.WEB-DL.H.264-QUILL"} }}
	f := seedShow(t, ts.start(t), nil)
	packWindowShow(t, f)
	sr := &Searcher{Store: f.s, PackReady: func(context.Context, []string) bool { return false }}
	runKind(t, f.fixture, sr, jobs.KindSearchSlot, &domain.Subject{Type: "slot", ID: f.slots["S01E01"]}, nil)
	if r := relByName(t, f.fixture, "Copper.Valley.S01.1080p.WEB-DL.H.264-QUILL"); !hasCode(r, "match.season_pack") {
		t.Fatalf("no qBittorrent: %s %v", r.State, reasonCodes(r))
	}
}

func TestFeedShow_PackWindowTakesThePackForTheWindow(t *testing.T) {
	ts := &tvServer{answer: func(url.Values) []string {
		return []string{"Copper.Valley.S01.1080p.WEB-DL.H.264-QUILL", "Copper.Valley.S01-S02.1080p.WEB-DL.H.264-QUILL|6000"}
	}}
	f := seedShow(t, ts.start(t), nil)
	packWindowShow(t, f)
	sr := &Searcher{Store: f.s, PackReady: ready(true)}
	if j := runKind(t, f.fixture, sr, jobs.KindFeedCheck, nil, nil); j.State != "done" {
		t.Fatalf("feed %s %+v", j.State, j.Error)
	}
	pack := relByName(t, f.fixture, "Copper.Valley.S01.1080p.WEB-DL.H.264-QUILL")
	if pack.State != "chosen" || !slices.Equal(PickSlots(pack), []int64{f.slots["S01E01"], f.slots["S01E02"]}) {
		t.Fatalf("pack %s %v %v", pack.State, PickSlots(pack), reasonCodes(pack))
	}
	if f.state(t, "S01E03") != mediafiles.SlotWanted {
		t.Fatalf("S01E03 is %s", f.state(t, "S01E03"))
	}
	if r := relByName(t, f.fixture, "Copper.Valley.S01-S02.1080p.WEB-DL.H.264-QUILL"); r.State == "chosen" {
		t.Fatalf("several seasons: %s %v", r.State, reasonCodes(r))
	}
}

func TestFeedShow_PackWindowPrefersTheSingle(t *testing.T) {
	ts := &tvServer{answer: func(url.Values) []string {
		return []string{"Copper.Valley.S01.2160p.WEB-DL.H.265-QUILL|9000", "Copper.Valley.S01E01.720p.HDTV.x264-OXBOW"}
	}}
	f := seedShow(t, ts.start(t), nil)
	packWindowShow(t, f)
	sr := &Searcher{Store: f.s, PackReady: ready(true)}
	runKind(t, f.fixture, sr, jobs.KindFeedCheck, nil, nil)
	if r := relByName(t, f.fixture, "Copper.Valley.S01E01.720p.HDTV.x264-OXBOW"); r.State != "chosen" {
		t.Fatalf("single %s %v", r.State, reasonCodes(r))
	}
	if r := relByName(t, f.fixture, "Copper.Valley.S01.2160p.WEB-DL.H.265-QUILL"); r.State == "chosen" {
		t.Fatalf("pack chosen over a passing single: %v", reasonCodes(r))
	}
}

func TestHandleSeason_PackWindowStillRetriesAFailedEpisode(t *testing.T) {
	ts := &tvServer{answer: func(url.Values) []string { return nil }}
	f := seedShow(t, ts.start(t), func(label string, sl *mediafiles.Slot) {
		if label == "S01E03" {
			sl.State = mediafiles.SlotFailed
		}
	})
	packWindowShow(t, f)
	sr := &Searcher{Store: f.s, PackReady: ready(true)}
	runKind(t, f.fixture, sr, jobs.KindSearchSeason, &domain.Subject{Type: "part", ID: f.parts[1]}, map[string]any{"version_id": f.verID})
	var queued int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM jobs WHERE kind = ? AND json_extract(subject, '$.id') = ?", jobs.KindSearchSlot, f.slots["S01E03"]).Scan(&queued)
	if queued != 1 {
		t.Fatalf("the failed S01E03 was not searched again")
	}
}
