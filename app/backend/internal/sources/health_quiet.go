package sources

import (
	"context"
	"slices"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// Successful feed checks that change nothing must not wake the disk just to be recorded, so their
// health data is kept in this process instead.
// Scheduled checks run at most once a minute, so this bound covers a full day with room to spare
// and stops the record from growing for ever.
const maxQuietDayCalls = 2048

// quietSample is one quiet check, with when it ran and how long it took.
type quietSample struct {
	at time.Time
	ms int
}

// quietDay sums the quiet checks of one day.
type quietDay struct {
	calls int
	ms    int64
}

// quietSourceCalls is the in-memory record of quiet checks for one Source.
type quietSourceCalls struct {
	last     time.Time
	dayCalls []quietSample
	days     map[string]quietDay
	recent   []Call
}

// Summarise adds the successful quiet feed checks of this process to the durable summary.
// Work, failures and recovery checks stay in the durable call record.
func (s *Searcher) Summarise(ctx context.Context, now time.Time) (map[int64]Summary, error) {
	all, err := Summarise(ctx, s.Store.Read(), now)
	if err != nil {
		return nil, err
	}
	s.quietMu.Lock()
	defer s.quietMu.Unlock()
	for id, q := range s.quietCalls {
		if q.last.Before(now.Add(-keepCalls)) {
			delete(s.quietCalls, id)
			continue
		}
		q.prune(now)
		if sum, exists := all[id]; exists {
			all[id] = addQuietSummary(sum, q, now)
		}
	}
	return all, nil
}

// StatsFor merges the durable history with the bounded quiet checks of this process.
func (s *Searcher) StatsFor(ctx context.Context, sourceID int64, days int, now time.Time) (Stats, error) {
	st, err := StatsFor(ctx, s.Store.Read(), sourceID, days, now)
	if err != nil {
		return st, err
	}
	s.quietMu.Lock()
	defer s.quietMu.Unlock()
	q := s.quietCalls[sourceID]
	if q == nil {
		return st, nil
	}
	if q.last.Before(now.Add(-keepCalls)) {
		delete(s.quietCalls, sourceID)
		return st, nil
	}
	q.prune(now)
	st.Summary = addQuietSummary(st.Summary, q, now)
	for i := range st.Days {
		d := &st.Days[i]
		x := q.days[d.Day]
		ok := d.Calls - d.Failures
		if ok+x.calls > 0 {
			d.AvgMS = int((int64(d.AvgMS)*int64(ok) + x.ms) / int64(ok+x.calls))
		}
		d.Calls += x.calls
	}
	st.Recent = append(st.Recent, q.recent...)
	slices.SortStableFunc(st.Recent, compareCallTime)
	if len(st.Recent) > 20 {
		st.Recent = st.Recent[:20]
	}
	return st, nil
}

// ForgetSource drops the in-memory history and client of a Source that was removed.
func (s *Searcher) ForgetSource(sourceID int64) {
	s.quietMu.Lock()
	delete(s.quietCalls, sourceID)
	s.quietMu.Unlock()
	s.mu.Lock()
	delete(s.clients, sourceID)
	s.mu.Unlock()
}

// prune drops quiet data older than the day window and the kept record.
func (q *quietSourceCalls) prune(now time.Time) {
	cut := now.Add(-24 * time.Hour)
	q.dayCalls = slices.DeleteFunc(q.dayCalls, func(c quietSample) bool { return c.at.Before(cut) })
	firstDay := now.UTC().AddDate(0, 0, -89).Format("2006-01-02")
	for day := range q.days {
		if day < firstDay {
			delete(q.days, day)
		}
	}
	kept := database.FormatTime(now.Add(-keepCalls))
	q.recent = slices.DeleteFunc(q.recent, func(c Call) bool { return c.At < kept })
}

// rememberQuietFeedCall notes one successful feed check that wrote nothing to the database.
func (s *Searcher) rememberQuietFeedCall(ctx context.Context, src domain.Source, at time.Time, took time.Duration) {
	s.quietMu.Lock()
	defer s.quietMu.Unlock()
	// Holding the lock across this read matters. Removal commits first and then takes the same lock in
	// ForgetSource, so a probe that is finishing cannot bring back deleted history.
	// The fingerprint also tells a reused row id from the old Source.
	var current domain.Source
	if err := database.Get(ctx, s.Store.Read(), "sources", src.ID, &current); err != nil ||
		feedSourceFingerprint([]domain.Source{src}) != feedSourceFingerprint([]domain.Source{current}) {
		return
	}
	if s.quietCalls == nil {
		s.quietCalls = make(map[int64]*quietSourceCalls)
	}
	q := s.quietCalls[src.ID]
	if q == nil {
		q = &quietSourceCalls{days: make(map[string]quietDay)}
		s.quietCalls[src.ID] = q
	}
	at = at.UTC()
	q.prune(at)
	ms := int(took.Milliseconds())
	if q.last.Before(at) {
		q.last = at
	}
	q.dayCalls = append(q.dayCalls, quietSample{at: at, ms: ms})
	if len(q.dayCalls) > maxQuietDayCalls {
		q.dayCalls = slices.Delete(q.dayCalls, 0, len(q.dayCalls)-maxQuietDayCalls)
	}
	day := at.Format("2006-01-02")
	d := q.days[day]
	d.calls++
	d.ms += int64(ms)
	q.days[day] = d
	q.recent = append(q.recent, Call{At: database.FormatTime(at), Op: "feed", OK: true, MS: ms})
	slices.SortStableFunc(q.recent, func(a, b Call) int { return compareCallTime(a, b) })
	if len(q.recent) > 20 {
		q.recent = q.recent[:20]
	}
}

// compareCallTime orders calls newest first.
func compareCallTime(a, b Call) int {
	if a.At > b.At {
		return -1
	}
	if a.At < b.At {
		return 1
	}
	return 0
}

// addQuietSummary folds the quiet checks into a durable summary, keeping the average weighted by call count.
func addQuietSummary(sum Summary, q *quietSourceCalls, now time.Time) Summary {
	if !q.last.Before(now.Add(-keepCalls)) {
		last := database.FormatTime(q.last)
		if sum.LastSuccess == nil || *sum.LastSuccess < last {
			sum.LastSuccess = &last
		}
	}
	var n int
	var ms int64
	for _, c := range q.dayCalls {
		if !c.at.Before(now.Add(-24 * time.Hour)) {
			n++
			ms += int64(c.ms)
		}
	}
	ok := sum.Calls - sum.Failures
	if ok+n > 0 {
		sum.AvgMS = int((int64(sum.AvgMS)*int64(ok) + ms) / int64(ok+n))
	}
	sum.Calls += n
	return sum
}
