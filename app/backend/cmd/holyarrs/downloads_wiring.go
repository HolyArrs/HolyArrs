package main

import (
	"context"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/shows"
)

// addMatchedMovie adds a movie an outside torrent matched, the normal way,
// without searching and without keeping it up to date (downloads.Web).
func (a *handlers) addMatchedMovie(ctx context.Context, id string) (int64, error) {
	m, err := a.s.Metadata.Movie(ctx, id)
	if err != nil {
		return 0, err
	}
	opt := library.AddOptions{}
	if err := a.kindDefaultsFor(ctx, "movie", nil, &opt.LibraryFolderID, &opt.PickRulesID, &opt.KeepUpToDate, &opt.SearchOnAdd); err != nil {
		return 0, err
	}
	off := false
	opt.KeepUpToDate, opt.SearchOnAdd, opt.WhyAdded = &off, false, "import"
	tid, _, err := a.s.Library.Add(ctx, m, opt, domain.System)
	return tid, err
}

// addMatchedShow is addMatchedMovie for a show.
func (a *handlers) addMatchedShow(ctx context.Context, id string) (int64, error) {
	opt := shows.ShowAddOptions{}
	if err := a.kindDefaultsFor(ctx, "show", nil, &opt.LibraryFolderID, &opt.PickRulesID, &opt.KeepUpToDate, &opt.SearchOnAdd); err != nil {
		return 0, err
	}
	off := false
	opt.KeepUpToDate, opt.SearchOnAdd, opt.WhyAdded = &off, false, "import"
	tid, _, err := shows.New(a.s.Library).AddShow(ctx, id, opt, domain.System)
	return tid, err
}

// DownloadsWeb is the Downloads routes' handler, for the program's wiring
// (the background reader's matching and live updates).
func (a *handlers) DownloadsWeb() *downloads.Web { return a.dl }
