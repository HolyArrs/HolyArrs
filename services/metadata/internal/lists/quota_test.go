package lists

import (
	"context"
	"testing"
	"time"
)

// The month cap is held to half the plan, whatever LISTS_MONTH_CAP says.
func TestMonthCapHeldToQuotaShare(t *testing.T) {
	s, _ := openTest(t, &fakeSource{}, Options{MonthCap: 900, PlanLimit: 1000})
	if s.opts.MonthCap != 500 {
		t.Fatalf("month cap %d, want 500", s.opts.MonthCap)
	}
	if c := PlanCost(s.plan); c > 500 {
		t.Fatalf("plan costs %d a month, over half the plan", c)
	}
}

// The source's own count stops calls at half the period, counting calls
// spent elsewhere (by hand) in the same period.
func TestQuotaHeadersStopAtHalf(t *testing.T) {
	src := &fakeSource{quota: 1000, used: 495}
	s, _ := openTest(t, src, Options{DayCap: 1000})
	_ = s.Refresh(context.Background())
	_ = s.Refresh(context.Background())
	q := s.Quota()
	if q.Limit != 1000 {
		t.Fatalf("quota not kept: %+v", q)
	}
	if q.Used() > 500 {
		t.Fatalf("spent to %d of 1000, over half", q.Used())
	}
	if src.n != 5 {
		t.Fatalf("%d calls, want 5 (495 + 5 = 500)", src.n)
	}
	st := s.Status(context.Background())
	if st.QuotaShare != 0.5 || st.Quota.Remaining != 500 {
		t.Fatalf("status %+v", st)
	}
}

// M9: a quota seen without a reset header goes stale a day after it was
// seen, so a count over half cannot stop the lists for good.
func TestQuotaWithoutResetGoesStale(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	s, _ := openTest(t, &fakeSource{}, Options{DayCap: 1000, Now: func() time.Time { return now }})
	if _, err := s.db.Exec(`INSERT INTO lists_quota (id, lim, remaining, reset_at, seen_at) VALUES (1, 1000, 100, 0, ?)`, now.Unix()); err != nil {
		t.Fatal(err)
	}
	if q := s.Quota(); q.Limit != 1000 || s.canSpend() {
		t.Fatalf("fresh quota without reset should hold: %+v", q)
	}
	now = now.Add(QuotaStale + time.Minute)
	if q := s.Quota(); q.Limit != 0 || !s.canSpend() {
		t.Fatalf("stale quota still holds: %+v", q)
	}
}

// AC19: the default day cap spreads the month cap over 30 days.
func TestDefaultDayCapSpreadsTheMonth(t *testing.T) {
	s, _ := openTest(t, &fakeSource{}, Options{})
	if s.opts.MonthCap != DefaultMonthCap || s.opts.DayCap != 17 {
		t.Fatalf("month %d day %d", s.opts.MonthCap, s.opts.DayCap)
	}
}
