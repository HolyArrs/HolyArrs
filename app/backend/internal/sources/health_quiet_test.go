package sources

import (
	"context"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// TestQuietFeedHealth_MergesWithoutWrites checks that quiet feed checks show in stats without any database write.
func TestQuietFeedHealth_MergesWithoutWrites(t *testing.T) {
	f := seed(t, "http://example.invalid")
	s := &Searcher{Store: f.s}
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	Record(ctx, f.s, f.srcID, "search", 100*time.Millisecond, nil)
	writes := f.s.WriteCount()
	var src domain.Source
	if err := database.Get(ctx, f.s.Read(), "sources", f.srcID, &src); err != nil {
		t.Fatal(err)
	}
	s.rememberQuietFeedCall(ctx, src, now.Add(-25*time.Hour), 300*time.Millisecond)
	s.rememberQuietFeedCall(ctx, src, now.Add(-time.Minute), 200*time.Millisecond)
	all, err := s.Summarise(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if got := all[f.srcID]; got.Calls != 2 || got.Failures != 0 || got.AvgMS != 150 || got.LastSuccess == nil {
		t.Fatalf("merged summary %+v", got)
	}
	st, err := s.StatsFor(ctx, f.srcID, 3, now)
	if err != nil {
		t.Fatal(err)
	}
	var calls int
	for _, d := range st.Days {
		calls += d.Calls
	}
	if calls != 3 || len(st.Recent) != 3 || st.Recent[0].Op != "search" || st.Summary.Calls != 2 {
		t.Fatalf("merged stats %+v", st)
	}
	if got := f.s.WriteCount(); got != writes {
		t.Fatalf("quiet history wrote %d transactions", got-writes)
	}
	// A new process keeps the durable call and forgets only the quiet checks.
	afterRestart, err := (&Searcher{Store: f.s}).Summarise(ctx, now)
	if err != nil || afterRestart[f.srcID].Calls != 1 {
		t.Fatalf("restart summary %+v, %v", afterRestart, err)
	}
	s.ForgetSource(f.srcID)
	if len(s.quietCalls) != 0 {
		t.Fatal("deleted Source retained quiet history")
	}
}

// TestQuietFeedHealth_BoundsAndExpires checks that quiet data stays bounded and expires with the kept record.
func TestQuietFeedHealth_BoundsAndExpires(t *testing.T) {
	f := seed(t, "http://example.invalid")
	s := &Searcher{Store: f.s}
	now := time.Now().UTC().Truncate(time.Second)
	ctx := context.Background()
	var src domain.Source
	if err := database.Get(ctx, f.s.Read(), "sources", f.srcID, &src); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < maxQuietDayCalls+10; i++ {
		s.rememberQuietFeedCall(ctx, src, now, time.Millisecond)
	}
	q := s.quietCalls[f.srcID]
	if len(q.dayCalls) != maxQuietDayCalls || len(q.recent) != 20 || len(q.days) != 1 {
		t.Fatalf("record is unbounded: samples %d, recent %d, days %d", len(q.dayCalls), len(q.recent), len(q.days))
	}
	for i := 1; i <= 95; i++ {
		s.rememberQuietFeedCall(ctx, src, now.AddDate(0, 0, i), time.Millisecond)
	}
	if len(q.days) != 90 || len(q.dayCalls) > 2 {
		t.Fatalf("expired history retained: samples %d, days %d", len(q.dayCalls), len(q.days))
	}
	st, err := s.StatsFor(context.Background(), f.srcID, 90, now.AddDate(0, 0, 186))
	if err != nil || len(st.Recent) != 0 || len(s.quietCalls) != 0 {
		t.Fatalf("expired quiet history: recent %d, sources %d, %v", len(st.Recent), len(s.quietCalls), err)
	}
	if st.Summary.LastSuccess != nil && *st.Summary.LastSuccess < database.FormatTime(now.AddDate(0, 0, 96)) {
		t.Fatal("expired success retained")
	}
}
