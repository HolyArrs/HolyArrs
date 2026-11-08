package api

import (
	"context"
	"net/http"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/music"
)

// Wrappers over the Music code moved to internal/music, so the api files
// still here call it as before. They go when their callers move too.

type (
	picEntry   = music.PicEntry
	sideRow    = music.SideRow
	sideFlight = music.SideFlight
)

// mh is the Music handlers over this API's services, built once.
func (a *API) mh() *music.Handlers {
	a.musOnce.Do(func() {
		a.mus = &music.Handlers{Deps: &a.s.Deps, Services: music.Services{Store: a.s.Store, Packages: a.s.Packages}}
	})
	return a.mus
}

func (a *API) pictureRecords(ctx context.Context, artists, albums []string, wait time.Duration) map[string]picEntry {
	return a.mh().PictureRecords(ctx, artists, albums, wait)
}

func (a *API) pictureVersion(kind, mbid string) string { return a.mh().PictureVersion(kind, mbid) }

func (a *API) warmPictureRecords(list []nameKey) {
	kinds, mbids := make([]string, len(list)), make([]string, len(list))
	for i, n := range list {
		kinds[i], mbids[i] = n.kind, n.mbid
	}
	a.mh().WarmPictureRecords(kinds, mbids)
}

func (a *API) awaitPictureRecords(ctx context.Context, kind, mbid string, wait time.Duration) {
	a.mh().AwaitPictureRecords(ctx, kind, mbid, wait)
}

func (a *API) withPictures(ctx context.Context, b []byte, extra ...string) []byte {
	return a.mh().WithPictures(ctx, b, extra...)
}

func (a *API) writeMusicJSON(w http.ResponseWriter, r *http.Request, v any, extra ...string) {
	a.mh().WriteMusicJSON(w, r, v, extra...)
}

func (a *API) sideRead(ctx context.Context, k string) (sideRow, bool) { return a.mh().SideRead(ctx, k) }

func pageFlight[T any](a *API, ctx context.Context, key string, f func(context.Context) (T, error)) *sideFlight {
	return music.PageFlight(a.mh(), ctx, key, f)
}

func waitFlight[T any](ctx context.Context, fl *sideFlight, by time.Time) (v T, err error, answered bool) {
	return music.WaitFlight[T](ctx, fl, by)
}

func sideGet[T any](a *API, ctx context.Context, kind, key string, fresh time.Duration, fetch func(context.Context) (T, bool, error)) (T, bool) {
	return music.SideGet(a.mh(), ctx, kind, key, fresh, fetch)
}
