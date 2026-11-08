package api

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
)

// feedStore does one background read by hand: every torrent Downloader's
// whole list goes into the reader's Store (made when the API has none),
// the way the reader's pollers fill it. The Downloads view reads only it.
func feedStore(t *testing.T, a *API) *downloads.Sync {
	t.Helper()
	if a.s.DLSync == nil {
		a.s.DLSync = &downloads.Sync{Store: &downloads.Store{}, DB: a.s.Store}
	}
	ctx := context.Background()
	ds, err := database.Select[domain.Downloader](ctx, a.read(), "downloaders", "1=1")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range ds {
		if d.Removed || d.Protocol != "torrent" {
			continue
		}
		var jobs []downloads.Job
		err := a.manager().ControlEngine(ctx, d, func(e downloads.Engine) error {
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
			a.s.DLSync.Store.Fail(d.ID, 0)
			continue
		}
		a.s.DLSync.Store.Update(d.ID, 0, jobs, time.Now())
	}
	return a.s.DLSync
}

// matchStored runs the reader's match lookup (MatchLinkOnly) once for every
// outside item in the Store, as its match queue would.
func matchStored(t *testing.T, a *API) {
	t.Helper()
	ctx := context.Background()
	ds, err := database.Select[domain.Downloader](ctx, a.read(), "downloaders", "1=1")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range ds {
		for _, j := range a.s.DLSync.Store.Current(d.ID).Items {
			if _, _, err := a.MatchLinkOnly(ctx, d.ID, j.ID, j.Name, j.Category); err != nil {
				t.Fatalf("match %q: %v", j.Name, err)
			}
		}
	}
}
