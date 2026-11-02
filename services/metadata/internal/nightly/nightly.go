// Package nightly fetches ahead, once a night, what title pages ask for
// most: Wikidata facts (awards, the Wikipedia summary) and MDBList
// ratings for trending, service-top and newly released titles. Answers
// already stored and fresh are skipped; MDBList calls are taken only
// while its hourly budget has more than a reserve left, so a page asked
// for during the night still gets its live answer, and whatever the
// budget or the night window does not reach is left for the next night.
package nightly

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/CoderLuii/holyarrs-metadata/internal/model"
	"github.com/CoderLuii/holyarrs-metadata/internal/wikidata"
)

// Title is one candidate title (Kind movie or tv, a TMDB id) and the list
// it came from.
type Title struct {
	Kind string
	ID   int64
	From string
}

// Ratings is the stored MDBList answers (extras.MDBList).
type Ratings interface {
	Due(ctx context.Context, kind, id string) bool
	Fetch(ctx context.Context, kind, id string) bool
}

// Facts is the stored Wikidata answers (wikidata.Service).
type Facts interface {
	Stored(ctx context.Context, kind string, tmdb int64) bool
	Title(ctx context.Context, kind string, tmdb int64) (wikidata.Entity, error)
}

// Options configure the job.
type Options struct {
	// Titles lists the candidates, most wanted first (duplicates are
	// dropped).
	Titles  func(ctx context.Context) []Title
	Ratings Ratings // nil: no MDBList key
	Facts   Facts   // nil: Wikidata off
	// Left is the MDBList budget left now (budget.Budget.Left).
	Left func() float64
	// Reserve calls of the MDBList budget are always left for live asks
	// (default 5).
	Reserve float64
	// Hour is the UTC hour the night run starts (default 2); Window is how
	// long it may run (default 6 h).
	Hour   int
	Window time.Duration
	// Wait is how long to wait for the MDBList budget to refill before
	// looking again (default a minute).
	Wait time.Duration
	Now  func() time.Time
	Logf func(string, ...any)
}

// Report is one night's counts.
type Report struct {
	Titles        int
	From          map[string]int
	RatingsGot    int
	RatingsFresh  int
	RatingsFailed int
	RatingsLeft   int
	FactsGot      int
	FactsFresh    int
	FactsFailed   int
	Skipped       []string // titles whose ratings the budget or window left
}

// Job is the nightly prefetch.
type Job struct{ o Options }

// New makes the job.
func New(o Options) *Job {
	if o.Reserve <= 0 {
		o.Reserve = 5
	}
	if o.Hour < 0 || o.Hour > 23 {
		o.Hour = 2
	}
	if o.Window <= 0 {
		o.Window = 6 * time.Hour
	}
	if o.Wait <= 0 {
		o.Wait = time.Minute
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Logf == nil {
		o.Logf = log.Printf
	}
	return &Job{o: o}
}

// next is the next start at o.Hour UTC after now.
func (j *Job) next(now time.Time) time.Time {
	now = now.UTC()
	t := time.Date(now.Year(), now.Month(), now.Day(), j.o.Hour, 0, 0, 0, time.UTC)
	if !t.After(now) {
		t = t.Add(24 * time.Hour)
	}
	return t
}

// Run runs once a night until ctx ends.
func (j *Job) Run(ctx context.Context) {
	for {
		wait := time.NewTimer(j.next(j.o.Now()).Sub(j.o.Now()))
		select {
		case <-ctx.Done():
			wait.Stop()
			return
		case <-wait.C:
		}
		j.RunOnce(ctx)
	}
}

// RunOnce makes one night's pass and logs what it fetched and skipped.
func (j *Job) RunOnce(ctx context.Context) Report {
	end := j.o.Now().Add(j.o.Window)
	r := Report{From: map[string]int{}}
	var titles []Title
	seen := map[string]bool{}
	if j.o.Titles != nil {
		for _, t := range j.o.Titles(ctx) {
			k := fmt.Sprintf("%s:%d", t.Kind, t.ID)
			if (t.Kind != "movie" && t.Kind != "tv") || t.ID <= 0 || seen[k] {
				continue
			}
			seen[k] = true
			titles = append(titles, t)
			r.From[t.From]++
		}
	}
	r.Titles = len(titles)
	// Wikidata first: its budget is large and each answer is one or two calls.
	if j.o.Facts != nil {
		for _, t := range titles {
			if ctx.Err() != nil || !j.o.Now().Before(end) {
				break
			}
			if j.o.Facts.Stored(ctx, t.Kind, t.ID) {
				r.FactsFresh++
				continue
			}
			if _, err := j.o.Facts.Title(ctx, t.Kind, t.ID); err != nil && err != wikidata.ErrNotFound {
				r.FactsFailed++
				continue
			}
			r.FactsGot++
		}
	}
	if j.o.Ratings != nil {
		for i, t := range titles {
			id := fmt.Sprint(t.ID)
			if !j.o.Ratings.Due(ctx, t.Kind, id) {
				r.RatingsFresh++
				continue
			}
			if !j.budget(ctx, end) {
				for _, rest := range titles[i:] {
					if j.o.Ratings.Due(ctx, rest.Kind, fmt.Sprint(rest.ID)) {
						r.RatingsLeft++
						if len(r.Skipped) < 20 {
							r.Skipped = append(r.Skipped, fmt.Sprintf("%s:%d", rest.Kind, rest.ID))
						}
					}
				}
				break
			}
			if j.o.Ratings.Fetch(ctx, t.Kind, id) {
				r.RatingsGot++
			} else {
				r.RatingsFailed++
			}
		}
	}
	from := make([]string, 0, len(r.From))
	for k, n := range r.From {
		from = append(from, fmt.Sprintf("%s %d", k, n))
	}
	j.o.Logf("nightly prefetch: %d titles (%s); ratings fetched %d, fresh %d, failed %d, skipped %d (budget or window); wikidata fetched %d, fresh %d, failed %d; skipped first: %s",
		r.Titles, strings.Join(from, ", "), r.RatingsGot, r.RatingsFresh, r.RatingsFailed, r.RatingsLeft, r.FactsGot, r.FactsFresh, r.FactsFailed, strings.Join(r.Skipped, " "))
	return r
}

// budget waits until the MDBList budget has more than the reserve left;
// false when the window or ctx ends first.
func (j *Job) budget(ctx context.Context, end time.Time) bool {
	for {
		if ctx.Err() != nil || !j.o.Now().Before(end) {
			return false
		}
		if j.o.Left == nil || j.o.Left() < 0 || j.o.Left() >= j.o.Reserve+1 {
			return true
		}
		t := time.NewTimer(j.o.Wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return false
		case <-t.C:
		}
	}
}

// Fetcher reads TMDB (the server's budgeted client).
type Fetcher interface {
	Fetch(ctx context.Context, q model.Query) (model.Result, error)
}

// TMDBLists reads the trending (day and week) and newly released (now
// playing, on the air) lists: six TMDB calls a night.
func TMDBLists(ctx context.Context, f Fetcher) []Title {
	var out []Title
	for _, l := range []struct{ path, kind, from string }{
		{"/trending/movie/day", "movie", "trending"}, {"/trending/tv/day", "tv", "trending"},
		{"/trending/movie/week", "movie", "trending"}, {"/trending/tv/week", "tv", "trending"},
		{"/movie/now_playing", "movie", "new"}, {"/tv/on_the_air", "tv", "new"},
	} {
		params, err := model.TMDBRead(l.path, url.Values{})
		if err != nil {
			continue
		}
		res, err := f.Fetch(ctx, model.Query{Provider: "tmdb", Kind: "raw", Operation: model.OperationTMDB, Path: l.path, Params: params, Page: 1, Limit: 20})
		if err != nil {
			continue
		}
		var d struct {
			Results []struct {
				ID int64 `json:"id"`
			} `json:"results"`
		}
		if json.Unmarshal(res.Data, &d) != nil {
			continue
		}
		for _, it := range d.Results {
			out = append(out, Title{Kind: l.kind, ID: it.ID, From: l.from})
		}
	}
	return out
}

// ServiceTop reads the stored service-top lists (each region's top titles
// and newly added ones); no call is made.
func ServiceTop(lists ...[]byte) []Title {
	var out []Title
	for _, b := range lists {
		var d struct {
			Services []struct {
				Items []struct {
					TMDBID int64  `json:"tmdb_id"`
					Kind   string `json:"kind"`
				} `json:"items"`
				New []struct {
					TMDBID int64  `json:"tmdb_id"`
					Kind   string `json:"kind"`
				} `json:"new"`
			} `json:"services"`
		}
		if json.Unmarshal(b, &d) != nil {
			continue
		}
		for _, s := range d.Services {
			for _, it := range s.Items {
				out = append(out, Title{Kind: it.Kind, ID: it.TMDBID, From: "service-top"})
			}
			for _, it := range s.New {
				out = append(out, Title{Kind: it.Kind, ID: it.TMDBID, From: "service-new"})
			}
		}
	}
	return out
}
