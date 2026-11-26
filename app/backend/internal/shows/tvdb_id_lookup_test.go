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

// A tvdb:<id> the chosen source (TMDB) has no link for, with no
// TVDB key set, is still found through TVMaze and added from it.
func TestLookupTVDBIDWithoutSourceLink(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	maze := &fakeShows{}
	cars := labShow("tvmaze", "900")
	cars.IDs["tvdb"] = "84847"
	maze.set(cars, "tvdb:84847", "tvmaze:900")
	svc := New(&library.Service{Store: s, Queue: jobs.New(s, jobs.Options{}), Now: func() time.Time { return now },
		Shows: metadata.ShowSources{"tmdb": &fakeShows{}, "tvmaze": maze, "tvdb": &fakeShows{err: metadata.ErrNoTVDBKey}}})

	sh, err := svc.LookupShow(ctx, "tvdb:84847", "tmdb")
	if err != nil || sh.Source != "tvmaze" || sh.IDs["tvdb"] != "84847" {
		t.Fatalf("lookup %+v %v", sh, err)
	}
	// An id nobody knows still finds nothing.
	if _, err := svc.LookupShow(ctx, "tvdb:1", "tmdb"); !errors.Is(err, metadata.ErrShowNotFound) {
		t.Fatalf("unknown id: %v", err)
	}

	addShowFolder(t, s)
	id, _, err := svc.AddShow(ctx, "tvdb:84847", ShowAddOptions{Source: "tmdb"}, domain.System)
	if err != nil {
		t.Fatalf("add %v", err)
	}
	var ti mediafiles.Title
	_ = database.Get(ctx, s.Read(), "titles", id, &ti)
	if ti.MetadataSource != "tvmaze" || ti.SourceIDs["tvdb"] != "84847" {
		t.Errorf("title source %q ids %v", ti.MetadataSource, ti.SourceIDs)
	}
}

// When the helper source can't be reached the error comes back instead
// of an empty list.
func TestLookupTVDBIDHelperDown(t *testing.T) {
	down := errors.New("TVMaze: connection refused")
	svc := showService(open(t), metadata.ShowSources{"tmdb": &fakeShows{}, "tvmaze": &fakeShows{err: down}, "tvdb": &fakeShows{err: metadata.ErrNoTVDBKey}})
	if _, err := svc.LookupShow(context.Background(), "tvdb:84847", "tmdb"); !errors.Is(err, down) {
		t.Fatalf("err %v", err)
	}
}

// The chosen source still wins when it has the link.
func TestLookupTVDBIDPrefersChosenSource(t *testing.T) {
	tmdb, maze := &fakeShows{}, &fakeShows{}
	tmdb.set(labShow("tmdb", "100"), "tvdb:200")
	maze.set(labShow("tvmaze", "300"), "tvdb:200")
	svc := showService(open(t), metadata.ShowSources{"tmdb": tmdb, "tvmaze": maze, "tvdb": &fakeShows{err: metadata.ErrNoTVDBKey}})
	sh, err := svc.LookupShow(context.Background(), "tvdb:200", "tmdb")
	if err != nil || sh.Source != "tmdb" || sh.IDs["tvmaze"] != "300" {
		t.Fatalf("%+v %v", sh, err)
	}
}
