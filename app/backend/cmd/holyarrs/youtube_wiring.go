package main

import (
	"context"
	"log"
	"os"
	"os/exec"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	ytlib "github.com/HolyArrs/holyarrs/app/backend/internal/youtube"
)

// newYouTubeService builds the YouTube library from the app's store, queue
// and live hub.
func newYouTubeService(st *database.Store, q *jobs.Queue, live *webserver.LiveHub) *ytlib.Service {
	svc := &ytlib.Service{Store: st, Queue: q,
		Tools: func() ytlib.Tools { return ytlib.FindTools(os.Getenv, exec.LookPath) },
		Run:   func(t ytlib.Tools) ytlib.Runner { return ytlib.Runner{Tools: t} },
		CheckFolder: func(ctx context.Context, path string) error {
			return library.CheckPlainFolder(ctx, st, path)
		},
		Person: func(ctx context.Context, userID int64) (accounts.PersonAccess, error) {
			return accounts.ReadPersonAccess(ctx, st.Read(), userID)
		},
		FreeSpace: importer.FreeSpace,
	}
	if live != nil {
		svc.Live = live
	}
	return svc
}

// youtubeService builds the YouTube library from the app's services.
func (a *handlers) youtubeService() *ytlib.Service {
	return newYouTubeService(a.s.Store, a.s.Queue, a.s.Live)
}

// youtubeJobs registers the check, download and adopt jobs while the youtube
// feature switch is on; with it off the kinds have no handler.
func youtubeJobs(st *database.Store, q *jobs.Queue, live *webserver.LiveHub) {
	if !startup.FeatureOn(startup.YouTube) {
		return
	}
	svc := newYouTubeService(st, q, live)
	q.Handle(ytlib.KindCheck, svc.HandleCheck)
	q.Handle(ytlib.KindDownload, svc.HandleDownload)
	q.Handle(ytlib.KindAdopt, svc.HandleAdopt)
}

// youtubeForgets removes a deleted person's YouTube library while the
// youtube feature switch is on.
func youtubeForgets(au *accounts.Auth, st *database.Store, q *jobs.Queue, live *webserver.LiveHub) {
	if !startup.FeatureOn(startup.YouTube) {
		return
	}
	svc := newYouTubeService(st, q, live)
	au.OnPersonDeleted(func(ctx context.Context, userID int64) {
		if err := svc.ForgetPerson(ctx, userID); err != nil {
			log.Printf("youtube: removing user %d's library: %v", userID, err)
		}
	})
}

// youtubeRoutes registers the YouTube routes, and saving a movie or
// episode file to a device, while the youtube feature switch is on; with it
// off they are not there at all.
func (a *handlers) youtubeRoutes(rt *webserver.Router) {
	if !startup.FeatureOn(startup.YouTube) {
		return
	}
	(&ytlib.Handlers{Svc: a.youtubeService()}).Routes(rt)
	(&library.FileDownloadHandlers{Store: a.s.Store}).Routes(rt.Mux())
}
