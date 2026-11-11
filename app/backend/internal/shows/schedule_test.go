package shows

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func TestShouldRefreshShow(t *testing.T) {
	ago := func(d time.Duration) *time.Time { x := now.Add(-d); return &x }
	h, day := time.Hour, 24*time.Hour
	cases := []struct {
		name    string
		fetched *time.Time
		status  string
		facts   ShowFacts
		want    bool
		why     string
	}{
		{"never fetched", nil, "running", ShowFacts{}, true, "never fetched"},
		{"stale even when ended", ago(31 * day), "ended", ShowFacts{}, true, "last fetch over 30 days ago"},
		{"aired episode without a name beats fresh", ago(time.Hour), "running", ShowFacts{AiredWithoutName: true}, true, "an aired episode has no name yet"},
		{"fresh", ago(5 * h), "running", ShowFacts{}, false, "last fetch under 6 hours ago"},
		{"running", ago(7 * h), "running", ShowFacts{}, true, "still running"},
		{"announced counts as not ended", ago(7 * h), "announced", ShowFacts{}, true, "still running"},
		{"ended, recent finale", ago(2 * day), "ended", ShowFacts{LastAired: ago(20 * day)}, true, "ended, last episode in the last 30 days"},
		{"ended long ago", ago(2 * day), "ended", ShowFacts{LastAired: ago(90 * day)}, false, "ended"},
		{"ended without dates", ago(2 * day), "ended", ShowFacts{}, false, "ended"},
	}
	for _, c := range cases {
		yes, why := ShouldRefreshShow(mediafiles.Title{RefreshedAt: c.fetched, Status: c.status}, c.facts, now)
		if yes != c.want || why != c.why {
			t.Errorf("%s: %v %q, want %v %q", c.name, yes, why, c.want, c.why)
		}
	}
}

// clock is a settable time for the gate tests.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time      { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) set(s string)        { c.mu.Lock(); c.t, _ = time.Parse(time.RFC3339, s); c.mu.Unlock() }
func (c *clock) add(d time.Duration) { c.mu.Lock(); c.t = c.t.Add(d); c.mu.Unlock() }

// TestEpisodeBecomesWantedOnTime is the air-date gate: S02E02 airs at
// 19 October 2026 02:00 UTC. A minute before, nothing happens and nothing is
// written; at the air time its Slot is wanted, with an Event naming it.
// With a 30-minute wait it moves half an hour later.
func TestEpisodeBecomesWantedOnTime(t *testing.T) {
	for _, wait := range []int{0, 30} {
		svc, s, _, _ := addShowService(t)
		ctx := context.Background()
		addShowFolder(t, s)
		if wait > 0 {
			d, _ := library.ReadShowDefaults(ctx, s.Read())
			d.WaitMinutes = wait
			if err := SetShowDefaults(ctx, s, d, domain.System); err != nil {
				t.Fatal(err)
			}
		}
		c := &clock{}
		c.set("2026-10-13T12:00:00Z")
		svc.Now = c.now
		id, _, err := svc.AddShow(ctx, "tmdb:100", ShowAddOptions{}, domain.System)
		if err != nil {
			t.Fatal(err)
		}
		if n, err := svc.OpenEpisodeGates(ctx); err != nil || n != 0 {
			t.Fatalf("wait %d: nothing aired since the add, opened %d %v", wait, n, err)
		}
		c.set("2026-10-19T01:59:00Z")
		c.add(time.Duration(wait) * time.Minute)
		writes := s.WriteCount()
		if n, _ := svc.OpenEpisodeGates(ctx); n != 0 || s.WriteCount() != writes {
			t.Fatalf("wait %d: a minute early: opened %d, writes %d -> %d", wait, n, writes, s.WriteCount())
		}
		c.add(time.Minute)
		if n, err := svc.OpenEpisodeGates(ctx); err != nil || n != 1 {
			t.Fatalf("wait %d: on time: opened %d %v", wait, n, err)
		}
		eps := episodes(t, s, id)
		if eps["S02E02"].State != mediafiles.SlotWanted || eps["S02E03"].State != mediafiles.SlotNotOutYet || !strings.HasPrefix(eps["S02E02"].Reason, "aired 2026-10-19 02:00") {
			t.Fatalf("wait %d: after the air time %+v", wait, eps)
		}
		es, _, _ := s.Events(ctx, database.EventFilter{TitleID: id, Kinds: []string{domain.EventGateOpened}})
		if len(es) != 1 || !strings.Contains(es[0].Summary, "S02E02") || es[0].SlotID == nil || *es[0].SlotID != eps["S02E02"].SlotID {
			t.Fatalf("wait %d: events %+v", wait, es)
		}
		// Idle again: no writes.
		writes = s.WriteCount()
		if n, _ := svc.OpenEpisodeGates(ctx); n != 0 || s.WriteCount() != writes {
			t.Fatalf("wait %d: second run opened %d or wrote", wait, n)
		}
	}
}

// TestNewEpisodeFromRefreshBecomesWanted: a refresh adds an episode that
// airs in an hour; the minute watcher makes it wanted when it airs, and a
// removed show does not wake.
func TestNewEpisodeFromRefreshBecomesWanted(t *testing.T) {
	svc, s, tmdb, _ := addShowService(t)
	ctx := context.Background()
	addShowFolder(t, s)
	c := &clock{}
	c.set("2026-10-13T12:00:00Z")
	svc.Now = c.now
	id, _, err := svc.AddShow(ctx, "tmdb:100", ShowAddOptions{}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	sh := labShow("tmdb", "100")
	sh.Episodes = append(sh.Episodes, metadata.Episode{Season: 2, Number: 4, Name: "Surprise", AirDate: tm("2026-10-13T13:00:00Z"), AirDay: "2026-10-13"})
	tmdb.set(sh, "tmdb:100")
	out, err := svc.RefreshShow(ctx, id, domain.System, nil)
	if err != nil || out.EpisodesAdded != 1 {
		t.Fatalf("refresh %+v %v", out, err)
	}
	if e := episodes(t, s, id)["S02E04"]; e.State != mediafiles.SlotNotOutYet || !e.Keep {
		t.Fatalf("new episode %+v", e)
	}
	c.set("2026-10-13T13:00:00Z")
	if n, _ := svc.OpenEpisodeGates(ctx); n != 1 || episodes(t, s, id)["S02E04"].State != mediafiles.SlotWanted {
		t.Fatalf("not wanted on time: %d", n)
	}
	// A removed show's episodes stay as they are.
	err = s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE titles SET state = 'removed' WHERE id = ?", id)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	c.set("2026-10-19T03:00:00Z")
	if n, _ := svc.OpenEpisodeGates(ctx); n != 0 {
		t.Fatalf("removed show woke %d episodes", n)
	}
}

// TestShowRefreshScheduleJob runs the refresh_shows Job: only the shows the
// rules pick are fetched; by hand, every show; one failing show does not
// stop the run.
func TestShowRefreshScheduleJob(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	src := &fakeShows{}
	q := jobs.New(s, jobs.Options{Workers: 1, PollInterval: 5 * time.Millisecond, BaseBackoff: 10 * time.Millisecond})
	svc := New(&library.Service{Store: s, Queue: q, Shows: metadata.ShowSources{"tmdb": src}, Now: func() time.Time { return now }})
	svc.Register(q)
	q.Handle(library.KindScanTitle, func(context.Context, *jobs.Run) error { return nil })
	addShowFolder(t, s)
	mk := func(id, name, status string, fetched time.Duration) int64 {
		sh := labShow("tmdb", id)
		sh.Name, sh.Status = name, status
		sh.IDs = map[string]string{"tmdb": id}
		src.set(sh, "tmdb:"+id)
		tid, _, err := svc.AddShow(ctx, "tmdb:"+id, ShowAddOptions{}, domain.System)
		if err != nil {
			t.Fatal(err)
		}
		at := now.Add(-fetched)
		if err := s.Write(ctx, func(tx *sql.Tx) error {
			_, err := tx.Exec("UPDATE titles SET refreshed_at = ? WHERE id = ?", database.FormatTime(at), tid)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return tid
	}
	fresh := mk("1", "Fresh Show", "running", time.Hour)
	running := mk("2", "Running Show", "running", 7*time.Hour)
	ended := mk("3", "Ended Show", "ended", 48*time.Hour)
	gone := mk("4", "Gone Show", "running", 8*time.Hour)
	delete(src.shows, "tmdb:4")
	// The ended show's last episode aired long ago, the lab show's S02E01
	// yesterday: make the ended one old.
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE items SET air_date = ? WHERE title_id = ? AND air_date > ?", database.FormatTime(now.AddDate(-1, 0, 0)), ended, database.FormatTime(now.AddDate(-1, 0, 0)))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	ids, why, err := svc.ShowRefreshPlan(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	// The lab show's "TBA" (S02E03) has no air date, so it does not count
	// as an aired episode without a name.
	if len(ids) != 2 || why[running] != "still running" || why[gone] != "still running" || why[fresh] != "" || why[ended] != "" {
		t.Fatalf("plan %v %v", ids, why)
	}
	if err := q.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(q.Stop)
	if _, _, err := q.Enqueue(ctx, KindRefreshShows, nil, map[string]any{"scheduled": true}, "schedule:refresh_shows", domain.System); err != nil {
		t.Fatal(err)
	}
	js := waitJobs(t, s, 1)
	var job domain.Job
	for _, j := range js {
		if j.Kind == KindRefreshShows {
			job = j
		}
	}
	if job.State != "done" || job.Result["refreshed"] != float64(2) || job.Result["dropped"] != float64(1) {
		t.Fatalf("job %+v", job)
	}
	var g mediafiles.Title
	_ = database.Get(ctx, s.Read(), "titles", gone, &g)
	if g.DroppedAt == nil {
		t.Fatal("the show the source forgot is not flagged")
	}
	ids, _, _ = svc.ShowRefreshPlan(ctx, true)
	if len(ids) != 4 {
		t.Fatalf("by hand %v", ids)
	}
	// The movie refresh plan never takes shows.
	mids, _, _ := svc.RefreshPlan(ctx, true, nil)
	if len(mids) != 0 {
		t.Fatalf("movie plan took shows %v", mids)
	}
}

func scheduleSvc(t *testing.T, s *database.Store, sources metadata.ShowSources) (*Service, *jobs.Queue) {
	t.Helper()
	q := jobs.New(s, jobs.Options{Workers: 1, PollInterval: 5 * time.Millisecond, BaseBackoff: 10 * time.Millisecond})
	svc := New(&library.Service{Store: s, Queue: q, Shows: sources, Now: func() time.Time { return now }})
	svc.Register(q)
	q.Handle(library.KindScanTitle, func(context.Context, *jobs.Run) error { return nil })
	return svc, q
}

func runRefreshShows(t *testing.T, s *database.Store, q *jobs.Queue) domain.Job {
	t.Helper()
	ctx := context.Background()
	if err := q.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(q.Stop)
	if _, _, err := q.Enqueue(ctx, KindRefreshShows, nil, map[string]any{"by_hand": true}, "refresh_shows_test", domain.System); err != nil {
		t.Fatal(err)
	}
	for _, j := range waitJobs(t, s, 1) {
		if j.Kind == KindRefreshShows {
			return j
		}
	}
	t.Fatal("no refresh_shows job")
	return domain.Job{}
}

// TestRefreshShowsJobStopsOnNoAccess: when every show follows TMDB and TMDB
// refuses the key, the Job fails fast. With a show on another source the
// run goes on and lists the one that failed.
func TestRefreshShowsJobStopsOnNoAccess(t *testing.T) {
	ctx := context.Background()
	mk := func(svc *Service, src *fakeShows, source, id string) {
		sh := labShow(source, id)
		sh.Name = "Show " + id
		sh.IDs = map[string]string{source: id}
		src.set(sh, source+":"+id)
		if _, _, err := svc.AddShow(ctx, source+":"+id, ShowAddOptions{Source: source}, domain.System); err != nil {
			t.Fatal(err)
		}
	}
	t.Run("every show on tmdb is refused", func(t *testing.T) {
		s := open(t)
		tmdb := &fakeShows{}
		svc, q := scheduleSvc(t, s, metadata.ShowSources{"tmdb": tmdb})
		addShowFolder(t, s)
		mk(svc, tmdb, "tmdb", "1")
		mk(svc, tmdb, "tmdb", "2")
		tmdb.mu.Lock()
		tmdb.err = metadata.ErrNoAccess
		tmdb.mu.Unlock()
		job := runRefreshShows(t, s, q)
		if job.State != "failed" || job.Error == nil {
			t.Fatalf("job %+v", job)
		}
		if _, listed := job.Result["failed"]; listed {
			t.Fatalf("the Job recorded per-show failures instead of aborting: %+v", job.Result)
		}
	})
	t.Run("one show fails on a mixed set", func(t *testing.T) {
		s := open(t)
		tmdb, maze := &fakeShows{}, &fakeShows{}
		svc, q := scheduleSvc(t, s, metadata.ShowSources{"tmdb": tmdb, "tvmaze": maze})
		addShowFolder(t, s)
		mk(svc, tmdb, "tmdb", "1")
		mk(svc, maze, "tvmaze", "2")
		maze.mu.Lock()
		maze.err = errors.New("tvmaze is down")
		maze.mu.Unlock()
		job := runRefreshShows(t, s, q)
		failed, _ := job.Result["failed"].([]any)
		if job.State != "done" || job.Result["refreshed"] != float64(1) || len(failed) != 1 {
			t.Fatalf("job %+v", job)
		}
	})
}

// Shows with no score yet are picked once the 6 hours have passed, even
// when the rules say no, at most library.ScoreFillPerRun a run; dropped
// shows are skipped.
func TestShowRefreshPlanFillsScores(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	svc, _ := scheduleSvc(t, s, metadata.ShowSources{})
	total := library.ScoreFillPerRun + 5
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		for i := range total {
			name, at := fmt.Sprintf("Show %03d", i), now.Add(-7*time.Hour)
			var dropped any
			switch i {
			case 0:
				at = now.Add(-5 * time.Hour)
			case 1:
				dropped = database.FormatTime(at)
			}
			if _, err := tx.Exec(`INSERT INTO titles (created_at, created_by, updated_at, updated_by, kind, name, sort_name, metadata_source, why_added, status, refreshed_at, dropped_at)
				VALUES (?, '{"kind":"system"}', ?, '{"kind":"system"}', 'show', ?, ?, 'tmdb', 'by_hand', 'ended', ?, ?)`,
				database.FormatTime(now), database.FormatTime(now), name, strings.ToLower(name), database.FormatTime(at), dropped); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	ids, why, err := svc.ShowRefreshPlan(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	filled := 0
	for _, id := range ids {
		if why[id] == "no score yet" {
			filled++
		}
	}
	if len(ids) != library.ScoreFillPerRun || filled != len(ids) || why[1] != "" || why[2] != "" || why[3] != "no score yet" {
		t.Fatalf("planned %d, filled %d, why %q %q %q", len(ids), filled, why[1], why[2], why[3])
	}
}
