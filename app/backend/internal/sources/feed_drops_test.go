package sources

import (
	"encoding/json"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// A feed check read 121 new Releases, stored 2 and said nothing about
// the rest, so a daily show's episode vanished without a trace. The feed
// check event now counts why each one went nowhere, and names a few of those
// that matched a Title in the Library.
func TestFeed_DropReasonsOnEvent(t *testing.T) {
	day := time.Now().UTC().AddDate(0, 0, -1)
	daily := "Copper Valley " + day.Format("2006 01 02") + " Ron Howard 1080p WEB h264-EDITH"
	pub := time.Now().Add(-time.Hour)
	var items []string
	for i := range 118 {
		items = append(items, item(fmt.Sprintf("Other.Show.%03d.S01E01.1080p.WEB.h264-GRP", i), pub, 5, ""))
	}
	items = append(items, item(daily, pub, 50, ""), item(web1080, pub, 5, ""), item("Dune.Part.Two.2024.720p.WEB-DL.DDP5.1.H.264-QUILL", pub, 5, ""))
	lf := &liveFeed{}
	lf.set(feedOf(items...))

	// The show is in the Library but none of its episodes is looking: the
	// episode the Release holds is not listed yet, the rest are not kept.
	f := seedShow(t, lf.url(t), func(_ string, sl *mediafiles.Slot) { sl.State = mediafiles.SlotInLibrary })
	sr := &Searcher{Store: f.s}
	j := runKind(t, f.fixture, sr, jobs.KindFeedCheck, nil, nil)
	if j.State != "done" {
		t.Fatalf("feed job %s %v", j.State, j.Error)
	}
	if n, _ := j.Result["releases_new"].(float64); n != 121 {
		t.Fatalf("releases_new %v, want 121 (%+v)", j.Result["releases_new"], j.Result)
	}
	if n, _ := j.Result["releases_stored"].(float64); n != 2 {
		t.Fatalf("releases_stored %v, want 2 (%+v)", j.Result["releases_stored"], j.Result)
	}

	raw := resultJSON(t, j)
	var data struct {
		Dropped  map[string]int      `json:"dropped"`
		Examples map[string][]string `json:"dropped_examples"`
	}
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatal(err)
	}
	want := map[string]int{dropNoTitle: 118, dropTitleNotLooking: 1}
	for k, n := range want {
		if data.Dropped[k] != n {
			t.Fatalf("dropped %v, want %v", data.Dropped, want)
		}
	}
	if !slices.Contains(data.Examples[dropTitleNotLooking], daily) {
		t.Fatalf("examples %v do not name %q", data.Examples, daily)
	}
	if len(data.Examples[dropNoTitle]) != 0 {
		t.Fatalf("unknown names are counted, not listed: %v", data.Examples[dropNoTitle])
	}

	// The next check sees the same feed: the stored two are seen, the rest
	// are read again and counted again.
	j = runKind(t, f.fixture, sr, jobs.KindFeedCheck, nil, nil)
	raw = resultJSON(t, j)
	data.Dropped = nil
	_ = json.Unmarshal([]byte(raw), &data)
	if data.Dropped[dropSeen] != 2 || data.Dropped[dropTitleNotLooking] != 1 {
		t.Fatalf("second check dropped %v", data.Dropped)
	}
}

// A show whose looking episodes are all not kept drops its Release with its
// own reason.
func TestFeed_DropReasonNotKept(t *testing.T) {
	lf := &liveFeed{}
	lf.set(feedOf(item("Copper.Valley.S02E01.1080p.WEB.h264-EDITH", time.Now().Add(-time.Hour), 50, "")))
	f := seedShow(t, lf.url(t), nil)
	exec(t, f.fixture, "UPDATE items SET keep_up_to_date = 0 WHERE title_id = ?", f.titleID)
	sr := &Searcher{Store: f.s}
	j := runKind(t, f.fixture, sr, jobs.KindFeedCheck, nil, nil)
	if j.State != "done" {
		t.Fatalf("feed job %s %v", j.State, j.Error)
	}
	raw := resultJSON(t, j)
	var data struct {
		Dropped map[string]int `json:"dropped"`
	}
	_ = json.Unmarshal([]byte(raw), &data)
	if data.Dropped[dropNotKept] != 1 {
		t.Fatalf("dropped %v (%s)", data.Dropped, raw)
	}
}

// resultJSON is a feed check's result, which is also its event's Data.
func resultJSON(t *testing.T, j domain.Job) string {
	t.Helper()
	b, err := json.Marshal(j.Result)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
