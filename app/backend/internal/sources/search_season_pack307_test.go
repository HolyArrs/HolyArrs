package sources

import (
	"net/url"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A season with one episode already in the Library still asks each
// Source for the whole season, so season packs come back.
func TestHandleSeason_AsksSeasonWithAnEpisodeInLibrary(t *testing.T) {
	ts := &tvServer{answer: func(q url.Values) []string {
		if q.Get("ep") == "" {
			return []string{"Copper.Valley.S01.1080p.WEB-DL.H.264-QUILL"}
		}
		return nil
	}}
	f := seedShow(t, ts.start(t), func(label string, sl *mediafiles.Slot) {
		if label == "S01E01" {
			sl.State = mediafiles.SlotInLibrary
		}
	})
	j := runKind(t, f.fixture, &Searcher{Store: f.s}, jobs.KindSearchSeason, &domain.Subject{Type: "part", ID: f.parts[1]}, map[string]any{"version_id": f.verID, "by_hand": true})
	if len(ts.queries) == 0 || ts.queries[0].Get("season") != "1" || ts.queries[0].Has("ep") {
		t.Fatalf("no season query: %v; job %s %+v", ts.queries, j.State, j.Result)
	}
	// The pack share rule still decides whether the pack is taken; here it
	// is not, so the two episodes are then searched on their own.
	if n, _ := j.Result["episodes_queued"].(float64); n != 2 {
		t.Fatalf("result %+v", j.Result)
	}
	if st := f.state(t, "S01E01"); st != mediafiles.SlotInLibrary {
		t.Fatalf("S01E01 is %s", st)
	}
}
