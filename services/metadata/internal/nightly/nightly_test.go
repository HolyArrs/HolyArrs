package nightly

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/model"
	"github.com/CoderLuii/holyarrs-metadata/internal/wikidata"
)

type fakeRatings struct {
	due     map[string]bool
	fetched []string
	fail    string
}

func (f *fakeRatings) Due(_ context.Context, kind, id string) bool { return f.due[kind+":"+id] }
func (f *fakeRatings) Fetch(_ context.Context, kind, id string) bool {
	f.fetched = append(f.fetched, kind+":"+id)
	f.due[kind+":"+id] = false
	return kind+":"+id != f.fail
}

type fakeFacts struct {
	stored map[string]bool
	asked  []string
}

func (f *fakeFacts) Stored(_ context.Context, kind string, id int64) bool {
	return f.stored[fmt.Sprintf("%s:%d", kind, id)]
}
func (f *fakeFacts) Title(_ context.Context, kind string, id int64) (wikidata.Entity, error) {
	f.asked = append(f.asked, fmt.Sprintf("%s:%d", kind, id))
	if id == 404 {
		return wikidata.Entity{}, wikidata.ErrNotFound
	}
	return wikidata.Entity{}, nil
}

func TestRunOnceSkipsFreshAndKeepsTheReserve(t *testing.T) {
	now := time.Date(2026, 10, 28, 2, 0, 0, 0, time.UTC)
	left := 7.0
	r := &fakeRatings{due: map[string]bool{"movie:1": true, "tv:2": true, "movie:3": false, "movie:404": true}, fail: "tv:2"}
	f := &fakeFacts{stored: map[string]bool{"movie:3": true}}
	var logged string
	j := New(Options{Ratings: r, Facts: f, Reserve: 5, Wait: time.Millisecond, Window: time.Hour,
		Now:  func() time.Time { return now },
		Left: func() float64 { return left },
		Titles: func(context.Context) []Title {
			return []Title{{"movie", 1, "trending"}, {"tv", 2, "service-top"}, {"movie", 1, "new"}, {"movie", 3, "trending"}, {"movie", 404, "new"}, {"person", 9, "x"}}
		},
		Logf: func(format string, a ...any) { logged = fmt.Sprintf(format, a...) }})
	// Each fetch spends one; with 7 left and a reserve of 5, two fit, and
	// the window ends while the third waits for the budget.
	j.o.Ratings = spend{r, &left}
	rep := runWithClock(j, &now)
	if rep.Titles != 4 || rep.FactsFresh != 1 || rep.FactsGot != 3 || len(f.asked) != 3 {
		t.Fatalf("facts %+v %v", rep, f.asked)
	}
	if rep.RatingsGot != 1 || rep.RatingsFailed != 1 || rep.RatingsFresh != 1 || rep.RatingsLeft != 1 || rep.Skipped[0] != "movie:404" {
		t.Fatalf("ratings %+v %v", rep, r.fetched)
	}
	if !strings.Contains(logged, "ratings fetched 1, fresh 1, failed 1, skipped 1") || !strings.Contains(logged, "movie:404") {
		t.Fatalf("log %q", logged)
	}
}

// spend takes one from left per fetch.
type spend struct {
	*fakeRatings
	left *float64
}

func (s spend) Fetch(ctx context.Context, kind, id string) bool {
	*s.left--
	return s.fakeRatings.Fetch(ctx, kind, id)
}

// runWithClock moves the clock a minute on each budget wait, so the window
// ends.
func runWithClock(j *Job, now *time.Time) Report {
	base := j.o.Now
	j.o.Now = func() time.Time { *now = now.Add(time.Minute); return base() }
	return j.RunOnce(context.Background())
}

func TestNext(t *testing.T) {
	j := New(Options{Hour: 2})
	if got := j.next(time.Date(2026, 10, 28, 1, 0, 0, 0, time.UTC)); got.Day() != 28 || got.Hour() != 2 {
		t.Fatal(got)
	}
	if got := j.next(time.Date(2026, 10, 28, 2, 0, 0, 0, time.UTC)); got.Day() != 29 {
		t.Fatal(got)
	}
}

type fetchFunc func(context.Context, model.Query) (model.Result, error)

func (f fetchFunc) Fetch(ctx context.Context, q model.Query) (model.Result, error) { return f(ctx, q) }

func TestLists(t *testing.T) {
	var paths []string
	got := TMDBLists(context.Background(), fetchFunc(func(_ context.Context, q model.Query) (model.Result, error) {
		paths = append(paths, q.Path)
		if q.Validate() != nil {
			t.Errorf("invalid query %+v", q)
		}
		return model.Result{Data: json.RawMessage(`{"results":[{"id":5},{"id":6}]}`)}, nil
	}))
	if len(paths) != 6 || len(got) != 12 || got[0].Kind != "movie" || got[1].ID != 6 || got[11].From != "new" || got[11].Kind != "tv" {
		t.Fatalf("%v %+v", paths, got)
	}
	st := ServiceTop([]byte(`{"services":[{"items":[{"tmdb_id":1,"kind":"movie"}],"new":[{"tmdb_id":2,"kind":"tv"}]}]}`), []byte(`nope`))
	if len(st) != 2 || st[0].From != "service-top" || st[1].From != "service-new" || st[1].Kind != "tv" {
		t.Fatalf("%+v", st)
	}
}
