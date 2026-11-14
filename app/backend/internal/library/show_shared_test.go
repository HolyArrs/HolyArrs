package library

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// TestShowOnNeverAdd: the Never-add list matches a show by any one of its
// ids of the same kind, never by another source's number, and ShowByIDs
// finds the show Title that shares an id.
func TestShowOnNeverAdd(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	db := s.Read()
	if _, err := AddNeverAdd(ctx, s, mediafiles.NeverAdd{Kind: "show", SourceIDs: map[string]string{"tmdb": "900", "tvdb": "123"}, Name: "Never Show"}, domain.System); err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct {
		ids  map[string]string
		want bool
	}{
		"same tvdb":              {map[string]string{"tvdb": "123"}, true},
		"same tmdb":              {map[string]string{"tmdb": "900"}, true},
		"one of several matches": {map[string]string{"tvmaze": "5", "tvdb": "123"}, true},
		"123 under tmdb is not":  {map[string]string{"tmdb": "123"}, false},
		"other tvdb":             {map[string]string{"tvdb": "124"}, false},
		"no ids":                 {map[string]string{}, false},
		"empty value":            {map[string]string{"tvdb": ""}, false},
	} {
		if got, err := ShowOnNeverAdd(ctx, db, c.ids); err != nil || got != c.want {
			t.Errorf("%s: %v %v, want %v", name, got, err, c.want)
		}
	}
	// A movie entry does not block a show with the same id.
	if _, err := AddNeverAdd(ctx, s, mediafiles.NeverAdd{Kind: "movie", SourceIDs: map[string]string{"tmdb": "777"}, Name: "Never Movie"}, domain.System); err != nil {
		t.Fatal(err)
	}
	if got, _ := ShowOnNeverAdd(ctx, db, map[string]string{"tmdb": "777"}); got {
		t.Fatal("a movie on the list blocked a show")
	}

	sh := seedLibShow(t, s, "Lab Show")
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE titles SET source_ids = '{"tvdb":"456","imdb":"tt456"}' WHERE id = ?`, sh.Title)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct {
		ids  map[string]string
		want int64
	}{
		"by tvdb":                {map[string]string{"tvdb": "456"}, sh.Title},
		"by imdb":                {map[string]string{"imdb": "tt456"}, sh.Title},
		"first id that matches":  {map[string]string{"tvdb": "999", "imdb": "tt456"}, sh.Title},
		"unknown":                {map[string]string{"tvdb": "999"}, 0},
		"456 under another kind": {map[string]string{"tmdb": "456"}, 0},
		"none":                   {nil, 0},
	} {
		if got, err := ShowByIDs(ctx, db, c.ids); err != nil || got != c.want {
			t.Errorf("%s: %d %v, want %d", name, got, err, c.want)
		}
	}
	// A removed show is not found.
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE titles SET state = 'removed' WHERE id = ?", sh.Title)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got, _ := ShowByIDs(ctx, db, map[string]string{"tvdb": "456"}); got != 0 {
		t.Fatalf("removed show found: %d", got)
	}
}

// TestScanThenSearchChain: ScanThenSearch queues a scan whose chain ends in
// the search of everything the title wants, and running that step queues the
// title's missing search once.
func TestScanThenSearchChain(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	q := jobs.New(s, jobs.Options{})
	svc := &Service{Store: s, Queue: q, Now: func() time.Time { return now }}
	if err := svc.ScanThenSearch(ctx, 42, domain.System); err != nil {
		t.Fatal(err)
	}
	scans, err := database.Select[domain.Job](ctx, s.Read(), "jobs", "kind = ?", KindScanTitle)
	if err != nil || len(scans) != 1 || scans[0].Subject == nil || scans[0].Subject.ID != 42 {
		t.Fatalf("scan jobs %+v %v", scans, err)
	}
	then, _ := scans[0].Params["then"].([]any)
	if len(then) != 1 || then[0] != stepSearchWanted {
		t.Fatalf("chain %v", scans[0].Params)
	}
	// Asked again before it ran: still one scan.
	if err := svc.ScanThenSearch(ctx, 42, domain.System); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, s, "jobs"); n != 1 {
		t.Fatalf("a repeated ask queued %d jobs", n)
	}
	// The chain's last step queues the missing search for that title.
	if err := svc.RunNextStep(ctx, &jobs.Run{Job: scans[0]}, 42); err != nil {
		t.Fatal(err)
	}
	ms, err := database.Select[domain.Job](ctx, s.Read(), "jobs", "kind = ?", jobs.KindSearchMissing)
	if err != nil || len(ms) != 1 || ms[0].Subject == nil || ms[0].Subject.ID != 42 {
		t.Fatalf("missing search jobs %+v %v", ms, err)
	}
	if err := svc.RunNextStep(ctx, &jobs.Run{Job: domain.Job{Params: map[string]any{"then": []any{"nonsense"}}}}, 42); err == nil {
		t.Fatal("an unknown step was accepted")
	}
}

// TestNoShowsFallback: a Service built without the shows module (nothing
// registered) says so on show work, opens no gates and registers no Jobs.
func TestNoShowsFallback(t *testing.T) {
	was := newShowParts
	newShowParts = nil
	t.Cleanup(func() { newShowParts = was })
	s := open(t)
	ctx := context.Background()
	q := jobs.New(s, jobs.Options{})
	svc := &Service{Store: s, Queue: q, Now: func() time.Time { return now }}
	p := svc.showParts()
	if _, err := p.Refresh(ctx, 1, domain.System, nil); !errors.Is(err, ErrNoShows) {
		t.Fatalf("Refresh: %v", err)
	}
	if _, err := p.PlanAdopt(ctx, 1, ""); !errors.Is(err, ErrNoShows) {
		t.Fatalf("PlanAdopt: %v", err)
	}
	if _, err := p.AdoptOne(ctx, mediafiles.LibraryFolder{}, AdoptChoice{}, domain.System); !errors.Is(err, ErrNoShows) {
		t.Fatalf("AdoptOne: %v", err)
	}
	if _, err := p.RecordFound(ctx, mediafiles.LibraryFolder{}, mediafiles.Version{}, nil, domain.System); !errors.Is(err, ErrNoShows) {
		t.Fatalf("RecordFound: %v", err)
	}
	if err := p.SearchShow(ctx, 1, domain.System); !errors.Is(err, ErrNoShows) {
		t.Fatalf("SearchShow: %v", err)
	}
	if n, err := svc.openEpisodeGates(ctx); err != nil || n != 0 {
		t.Fatalf("gates: %d %v", n, err)
	}
	svc.Register(q) // must not panic, and adds no show Job
	// A show title's refresh through the shared code reports the missing module.
	sh := seedLibShow(t, s, "Lab Show")
	if _, err := svc.RefreshTitle(ctx, sh.Title, domain.System, nil); !errors.Is(err, ErrNoShows) {
		t.Fatalf("RefreshTitle on a show: %v", err)
	}
	// And so does the search step of a new show's chain.
	if err := svc.RunNextStep(ctx, &jobs.Run{Job: domain.Job{Params: map[string]any{"then": []any{StepSearchShow}}}}, sh.Title); !errors.Is(err, ErrNoShows) {
		t.Fatalf("search_show step: %v", err)
	}
}

// An episode known only by its day opens at the end of that day in UTC, and
// one with an exact time opens then; a wait pushes both later. The shows
// supervisor pass (every minute) is what moves the Slot, not a refresh.
func TestEpisodeGateFromDayOnlyAndTimed(t *testing.T) {
	dayEnd := time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC) // air day Nov 12, no time
	timed := time.Date(2026, 11, 13, 2, 0, 0, 0, time.UTC)  // aired 9pm in New York on Nov 12
	cases := []struct {
		name string
		air  time.Time
		wait time.Duration
		now  time.Time
		open bool
	}{
		{"day only, 7 minutes before its end (10:53 in Sydney)", dayEnd, 0, time.Date(2026, 11, 12, 23, 53, 0, 0, time.UTC), false},
		{"day only, once its end has passed", dayEnd, 0, time.Date(2026, 11, 13, 0, 1, 0, 0, time.UTC), true},
		{"timed, before the air time", timed, 0, time.Date(2026, 11, 13, 1, 59, 0, 0, time.UTC), false},
		{"timed, after the air time", timed, 0, time.Date(2026, 11, 13, 2, 0, 0, 0, time.UTC), true},
		{"timed, held by the wait", timed, 6 * time.Hour, time.Date(2026, 11, 13, 7, 0, 0, 0, time.UTC), false},
		{"timed, wait over", timed, 6 * time.Hour, time.Date(2026, 11, 13, 8, 0, 0, 0, time.UTC), true},
	}
	for _, c := range cases {
		air := c.air
		if open, why := EpisodeGate(&air, c.wait, c.now); open != c.open {
			t.Errorf("%s: open %v (%s)", c.name, open, why)
		}
	}
}
