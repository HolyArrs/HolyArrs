package api

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/dlsync"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloader"
	"github.com/HolyArrs/holyarrs/app/backend/internal/store"
)

// feedStore does one background read by hand: every torrent Downloader's
// whole list goes into the reader's Store (made when the API has none),
// the way the reader's pollers fill it. The Downloads view reads only it.
func feedStore(t *testing.T, a *API) *dlsync.Sync {
	t.Helper()
	if a.s.DLSync == nil {
		a.s.DLSync = &dlsync.Sync{Store: &dlsync.Store{}, DB: a.s.Store}
	}
	ctx := context.Background()
	ds, err := store.Select[domain.Downloader](ctx, a.read(), "downloaders", "1=1")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range ds {
		if d.Removed || d.Protocol != "torrent" {
			continue
		}
		var jobs []downloader.Job
		err := a.manager().ControlEngine(ctx, d, func(e downloader.Engine) error {
			al, ok := e.(downloader.AllLister)
			if !ok {
				return downloader.ErrNotSupported
			}
			var err error
			jobs, err = al.ListAll(ctx)
			return err
		})
		if errors.Is(err, downloader.ErrNotSupported) {
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
	ds, err := store.Select[domain.Downloader](ctx, a.read(), "downloaders", "1=1")
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
