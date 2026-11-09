package health

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
)

type schedulesAnswer struct {
	Schedules []ScheduleView `json:"schedules"`
	Loops     []LoopView     `json:"loops"`
	SafeMode  bool           `json:"safe_mode"`
}

func scheduleNamed(t *testing.T, a schedulesAnswer, kind string) ScheduleView {
	t.Helper()
	for _, s := range a.Schedules {
		if s.Kind == kind {
			return s
		}
	}
	t.Fatalf("no schedule %s in %+v", kind, a.Schedules)
	return ScheduleView{}
}

// route: GET /api/v1/schedules
// route: POST /api/v1/schedules/{kind}/run
// route: PUT /api/v1/schedules/{kind}
func TestSchedules(t *testing.T) {
	t.Parallel()
	f := newCtl(t)
	// The periodic loops report their interval and passes.
	deadline := time.Now().Add(5 * time.Second)
	var got schedulesAnswer
	for {
		if err := json.Unmarshal(do(t, "GET", f.srv.URL+"/api/v1/schedules", "", 200), &got); err != nil {
			t.Fatal(err)
		}
		ready := len(got.Loops) > 0
		for _, l := range got.Loops {
			ready = ready && l.NextPass != nil && l.LastPass != nil
		}
		if ready || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(got.Loops) != 2 || got.Loops[0].Name != "backups" || got.Loops[0].Part != "core" || got.Loops[1].Part != "movies" || got.Loops[0].NextPass == nil {
		t.Fatalf("loops %+v", got.Loops)
	}
	feed := scheduleNamed(t, got, "feed_check")
	if feed.Label != "Feed check" || feed.EverySeconds != 900 || feed.MinSeconds != 600 || feed.Part != "media" || feed.Settings != "/settings/search" || feed.State != "waiting" {
		t.Fatalf("feed %+v", feed)
	}
	if len(got.Schedules) < 12 {
		t.Fatalf("only %d schedules", len(got.Schedules))
	}
	for _, s := range got.Schedules {
		if s.Label == "" || strings.Contains(s.Label, "_") {
			t.Errorf("%s has no label", s.Kind)
		}
	}

	// Run now queues the Job without moving the clock; a second ask finds it.
	b := do(t, "POST", f.srv.URL+"/api/v1/schedules/feed_check/run", "", 202)
	var run struct {
		JobID   int64 `json:"job_id"`
		Already bool  `json:"already"`
	}
	_ = json.Unmarshal(b, &run)
	if run.JobID == 0 {
		t.Fatalf("run: %s", b)
	}
	do(t, "POST", f.srv.URL+"/api/v1/schedules/nope/run", "", 404)

	// Change the interval within bounds.
	b = do(t, "PUT", f.srv.URL+"/api/v1/schedules/feed_check", `{"every_s": 1800}`, 200)
	var v ScheduleView
	_ = json.Unmarshal(b, &v)
	if v.EverySeconds != 1800 || v.NextRun == nil || time.Until(*v.NextRun) < 25*time.Minute {
		t.Fatalf("put: %s", b)
	}
	for _, bad := range []string{`{"every_s": 60}`, `{"every_s": 99999}`, `{"every_s": 630}`, `nope`} {
		do(t, "PUT", f.srv.URL+"/api/v1/schedules/feed_check", bad, 400)
	}
	do(t, "PUT", f.srv.URL+"/api/v1/schedules/nope", `{"every_s": 600}`, 404)

	// A part that is off: shown and refused.
	if err := f.pc.Set(context.Background(), parts.Movies, false, domain.System); err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(do(t, "GET", f.srv.URL+"/api/v1/schedules", "", 200), &got)
	if s := scheduleNamed(t, got, "refresh_titles"); s.State != "part_off" {
		t.Fatalf("part off: %+v", s)
	}
	if b := do(t, "POST", f.srv.URL+"/api/v1/schedules/refresh_titles/run", "", 409); !strings.Contains(string(b), "part is off") {
		t.Fatalf("refused: %s", b)
	}
}

func TestSchedulesInSafeMode(t *testing.T) {
	t.Parallel()
	f := newCtlMode(t, true)
	var got schedulesAnswer
	_ = json.Unmarshal(do(t, "GET", f.srv.URL+"/api/v1/schedules", "", 200), &got)
	if !got.SafeMode || scheduleNamed(t, got, "feed_check").State != "safe_mode" || scheduleNamed(t, got, "health_check").State == "safe_mode" {
		t.Fatalf("%+v", got.Schedules)
	}
	do(t, "POST", f.srv.URL+"/api/v1/schedules/feed_check/run", "", 409)
	do(t, "POST", f.srv.URL+"/api/v1/schedules/health_check/run", "", 202)
}

func TestMovieRefreshScheduleCanBeTurnedOffAndOn(t *testing.T) {
	t.Parallel()
	f := newCtl(t)
	ctx := context.Background()
	future := time.Now().Add(time.Hour)
	scheduled, _, err := f.q.EnqueueAt(ctx, "refresh_titles", nil, map[string]any{"scheduled": true}, "schedule:refresh_titles", domain.System, future)
	if err != nil {
		t.Fatal(err)
	}
	manual, _, err := f.q.EnqueueAt(ctx, "refresh_titles", nil, map[string]any{"by_hand": true}, "refresh_titles:by_hand", domain.System, future)
	if err != nil {
		t.Fatal(err)
	}

	b := do(t, "PUT", f.srv.URL+"/api/v1/schedules/refresh_titles", `{"enabled":false}`, 200)
	var off ScheduleView
	decodeJSON(t, b, &off)
	if off.Enabled || off.LastRun != nil {
		t.Fatalf("off response %+v", off)
	}
	var scheduledState, manualState string
	if err := f.s.Read().QueryRow("SELECT state FROM jobs WHERE id = ?", scheduled).Scan(&scheduledState); err != nil {
		t.Fatal(err)
	}
	if err := f.s.Read().QueryRow("SELECT state FROM jobs WHERE id = ?", manual).Scan(&manualState); err != nil {
		t.Fatal(err)
	}
	if scheduledState != "cancelled" || manualState != "queued" {
		t.Fatalf("scheduled=%s manual=%s", scheduledState, manualState)
	}
	if err := f.q.Cancel(ctx, manual); err != nil {
		t.Fatal(err)
	}
	var listed schedulesAnswer
	decodeJSON(t, do(t, "GET", f.srv.URL+"/api/v1/schedules", "", 200), &listed)
	if got := scheduleNamed(t, listed, "refresh_titles"); got.State != "off" || got.LastJob != "cancelled" {
		t.Fatalf("listed off schedule %+v", got)
	}

	before := time.Now().UTC()
	b = do(t, "PUT", f.srv.URL+"/api/v1/schedules/refresh_titles", `{"enabled":true}`, 200)
	var on ScheduleView
	decodeJSON(t, b, &on)
	if !on.Enabled || on.State != "waiting" || on.NextRun == nil {
		t.Fatalf("on response %+v", on)
	}
	want := before.Add(time.Duration(on.EverySeconds) * time.Second)
	if on.NextRun.Before(want.Add(-time.Second)) || on.NextRun.After(want.Add(2*time.Second)) {
		t.Fatalf("next run %v, want about %v", on.NextRun, want)
	}
}

func TestOnlyMovieRefreshScheduleMayBeDisabled(t *testing.T) {
	t.Parallel()
	f := newCtl(t)
	b := do(t, "PUT", f.srv.URL+"/api/v1/schedules/refresh_shows", `{"enabled":false}`, 409)
	if !strings.Contains(string(b), "only scheduled movie details refresh") {
		t.Fatalf("response %s", b)
	}
	// Existing interval-only clients remain valid.
	do(t, "PUT", f.srv.URL+"/api/v1/schedules/refresh_titles", `{"every_s":43200}`, 200)
}

func TestSpanWords(t *testing.T) {
	t.Parallel()
	for s, want := range map[int64]string{60: "1 minute", 600: "10 minutes", 3600: "1 hour", 7200: "2 hours", 86400: "1 day", 90: "90 seconds"} {
		if got := Span(s); got != want {
			t.Errorf("span(%d) = %q, want %q", s, got, want)
		}
	}
}

// route: GET /api/v1/jobs/active
func TestActiveJobsOverHTTP(t *testing.T) {
	t.Parallel()
	srv, _, q := setup(t)
	// A queued Job far in the future stays queued, so the list can be checked.
	if _, _, err := q.EnqueueAt(context.Background(), "feed_check", nil, nil, "", domain.Actor{Kind: "user", ID: 1}, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Jobs []ActiveJob `json:"jobs"`
	}
	if err := json.Unmarshal(do(t, "GET", srv.URL+"/api/v1/jobs/active", "", 200), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Jobs) != 1 || got.Jobs[0].Kind != "feed_check" || got.Jobs[0].Label != "Feed check" || got.Jobs[0].State != "queued" || !got.Jobs[0].ByPerson {
		t.Fatalf("active jobs: %+v", got.Jobs)
	}
}
