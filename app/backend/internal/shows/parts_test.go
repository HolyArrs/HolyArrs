package shows

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// TestRefreshTitleRoutesShowsThroughShowParts: the shared single-title
// refresh hands a show to the shows module and reports its gates; a show the
// source forgot comes back dropped and flagged, not as the movie path's error.
func TestRefreshTitleRoutesShowsThroughShowParts(t *testing.T) {
	svc, s, tmdb, _ := addShowService(t)
	ctx := context.Background()
	addShowFolder(t, s)
	id, _, err := svc.AddShow(ctx, "tmdb:100", ShowAddOptions{}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	// A new episode that airs in the past: the gate opens on refresh.
	sh := labShow("tmdb", "100")
	sh.Episodes = append(sh.Episodes, metadata.Episode{Season: 2, Number: 4, Name: "Late", AirDate: tm("2026-10-13T11:00:00Z"), AirDay: "2026-10-13", ID: "tmdb-204"})
	tmdb.set(sh, "tmdb:100")
	out, err := svc.Service.RefreshTitle(ctx, id, domain.System, nil)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if errors.Is(err, library.ErrNoShows) || out.GateDriver != "air_date" || out.Dropped {
		t.Fatalf("outcome %+v", out)
	}
	if e := episodes(t, s, id)["S02E04"]; e.State != mediafiles.SlotWanted {
		t.Fatalf("a refreshed episode that already aired is %s, want wanted", e.State)
	}
	// The source forgets the show.
	tmdb.mu.Lock()
	tmdb.shows = map[string]metadata.Show{}
	tmdb.mu.Unlock()
	out, err = svc.Service.RefreshTitle(ctx, id, domain.System, nil)
	if err != nil || !out.Dropped || out.GateDriver != "air_date" {
		t.Fatalf("forgotten show: %+v %v", out, err)
	}
	var ti mediafiles.Title
	if err := database.Get(ctx, s.Read(), "titles", id, &ti); err != nil || ti.DroppedAt == nil {
		t.Fatalf("title not flagged: %+v %v", ti.DroppedAt, err)
	}
}

// TestShowPartsInstalled: importing shows installs the real show steps, so
// the library's Register adds the refresh_shows Job and it runs.
func TestShowPartsInstalled(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	q := jobs.New(s, jobs.Options{Workers: 1, PollInterval: 5 * time.Millisecond, BaseBackoff: 10 * time.Millisecond})
	lib := &library.Service{Store: s, Queue: q, Now: func() time.Time { return now }}
	lib.Register(q) // library only: the show Jobs come from the installed parts
	if err := q.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(q.Stop)
	if _, _, err := q.Enqueue(ctx, KindRefreshShows, nil, map[string]any{"by_hand": true}, "show_parts_installed", domain.System); err != nil {
		t.Fatal(err)
	}
	js := waitJobs(t, s, 1)
	if js[0].Kind != KindRefreshShows || js[0].State != "done" {
		t.Fatalf("refresh_shows had no working handler: %+v", js[0])
	}
}

// TestScanThenSearchShowQueuesSeasonSearches runs the whole chain through
// the queue: the add's scan Job, then the whole-show search step that
// queues one season search per wanted season.
func TestScanThenSearchShowQueuesSeasonSearches(t *testing.T) {
	svc, s, _, _ := addShowService(t)
	ctx := context.Background()
	q := jobs.New(s, jobs.Options{Workers: 1, PollInterval: 5 * time.Millisecond, BaseBackoff: 10 * time.Millisecond})
	svc.Queue = q
	svc.Register(q)
	q.Handle(jobs.KindSearchSeason, func(context.Context, *jobs.Run) error { return nil })
	addShowFolder(t, s)
	id, _, err := svc.AddShow(ctx, "tmdb:100", ShowAddOptions{SearchOnAdd: true}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	want, err := ShowSeasonsToSearch(ctx, s.Read(), id)
	if err != nil || len(want) == 0 {
		t.Fatalf("seasons to search %v %v", want, err)
	}
	if err := q.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(q.Stop)
	js := waitJobs(t, s, 1+len(want))
	for _, j := range js {
		if j.State != "done" {
			t.Fatalf("job %s ended %s: %+v", j.Kind, j.State, j.Error)
		}
	}
	ss, err := database.Select[domain.Job](ctx, s.Read(), "jobs", "kind = 'search_season' ORDER BY id")
	if err != nil || len(ss) != len(want) {
		t.Fatalf("season searches %d, want %d (%v)", len(ss), len(want), err)
	}
	for _, j := range ss {
		if j.Subject == nil || j.Subject.Type != "part" {
			t.Fatalf("search subject %+v", j.Subject)
		}
		var owner int64
		if err := s.Read().QueryRow("SELECT title_id FROM parts WHERE id = ?", j.Subject.ID).Scan(&owner); err != nil || owner != id {
			t.Fatalf("season search for part %d belongs to title %d, want %d (%v)", j.Subject.ID, owner, id, err)
		}
	}
}
