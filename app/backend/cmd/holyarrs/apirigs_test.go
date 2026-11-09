package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
)

// Rigs for the tests that cross modules: the whole api with sign-in, the
// Library and the Downloads reader.

// feedStore does one background read by hand: every torrent Downloader's
// whole list goes into the reader's Store (made when the API has none),
// the way the reader's pollers fill it. The Downloads view reads only it.
func feedStore(t *testing.T, a *handlers) *downloads.Sync {
	t.Helper()
	if a.DownloadsWeb().DLSync == nil {
		a.DownloadsWeb().DLSync = &downloads.Sync{Store: &downloads.Store{}, DB: a.DownloadsWeb().Store}
	}
	ctx := context.Background()
	ds, err := database.Select[domain.Downloader](ctx, a.DownloadsWeb().Store.Read(), "downloaders", "1=1")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range ds {
		if d.Removed || d.Protocol != "torrent" {
			continue
		}
		var jobs []downloads.Job
		err := a.DownloadsWeb().Manager().ControlEngine(ctx, d, func(e downloads.Engine) error {
			al, ok := e.(downloads.AllLister)
			if !ok {
				return downloads.ErrNotSupported
			}
			var err error
			jobs, err = al.ListAll(ctx)
			return err
		})
		if errors.Is(err, downloads.ErrNotSupported) {
			err = nil
		}
		if err != nil {
			a.DownloadsWeb().DLSync.Store.Fail(d.ID, 0)
			continue
		}
		a.DownloadsWeb().DLSync.Store.Update(d.ID, 0, jobs, time.Now())
	}
	return a.DownloadsWeb().DLSync
}

// matchStored runs the reader's match lookup (MatchLinkOnly) once for every
// outside item in the Store, as its match queue would.
func matchStored(t *testing.T, a *handlers) {
	t.Helper()
	ctx := context.Background()
	ds, err := database.Select[domain.Downloader](ctx, a.DownloadsWeb().Store.Read(), "downloaders", "1=1")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range ds {
		for _, j := range a.DownloadsWeb().DLSync.Store.Current(d.ID).Items {
			if _, _, err := a.DownloadsWeb().MatchLinkOnly(ctx, d.ID, j.ID, j.Name, j.Category); err != nil {
				t.Fatalf("match %q: %v", j.Name, err)
			}
		}
	}
}

type viewRows struct {
	Downloads []downloads.DownloadRow `json:"downloads"`
}
