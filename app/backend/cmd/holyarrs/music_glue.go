package main

import (
	"context"
	"net/http"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/music"
	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"
)

// Wrappers over the Music code in internal/music, so the files here
// call it in one short form.

// mh is the Music handlers over this API's services, built once.
func (a *handlers) mh() *music.Handlers {
	a.musOnce.Do(func() {
		h := &music.Handlers{Deps: &a.s.Deps, Services: music.Services{
			Store: a.s.Store, Packages: a.s.Packages, Queue: a.s.Queue,
			Library: func() *library.Service { return a.s.Library }, Live: a.s.Live, Parts: a.s.Parts, Asks: a.s.Asks, Trailers: a.trailers,
			ImageBase: func() string { return a.s.ImageBase }, MusicControl: a.s.MusicControl, ArtistVideos: a.s.ArtistVideos,
			PartIsOn: a.partIsOn, KidMax: a.kidMax, Person: a.person, OpenArtistAsks: func(ctx context.Context, mbid string, me int64) ([]requests.AskBrief, error) {
				return requests.OpenArtistAsks(ctx, a.s.Store.Read(), mbid, me)
			},

			ArtistVideosOn: a.artistVideosOn, MusicFresh: a.musicFresh, ExploreCaching: func(w http.ResponseWriter, r *http.Request, c string) { a.disc.Caching(w, r, c) },
			TitleID: a.titleID, LibraryError: library.WriteError,
			TitleView: func(ctx context.Context, t mediafiles.Title) (any, error) { return a.titleView(ctx, t) },
			PublishTitleView: func(v any) {
				if tv, ok := v.(TitleView); ok {
					a.tv().PublishView(tv)
				}
			},
			Limit: func(userID int64, now time.Time) bool { return a.discover.Allow(userID, now) },
			TrackRoutes: func(m *http.ServeMux) {
				music.TrackHandlers[EpisodeSlot]{Store: a.s.Store, Slots: discover.TrackSlots}.Routes(m)
			}}}
		if a.shows != nil {
			h.SlotOfTitle, h.SeasonLabel = a.shows.SlotOfTitle, a.shows.SeasonLabel
		}
		a.mus = h
	})
	return a.mus
}

// musicProvider is what the shared handlers (live search, New & Popular)
// ask about music. With Music off it says so and gives nothing.
type musicProvider struct{ a *handlers }

func (a *handlers) musicProvider() musicProvider { return musicProvider{a} }

func (p musicProvider) On() bool { return p.a.mh().MusicOn() }

func (p musicProvider) WarmNew() { p.a.mh().WarmNew() }

func (a *handlers) musicTrendArtFile(ctx context.Context, kind, mbid, size string) (name, path string, ok bool) {
	return a.mh().TrendArtFile(ctx, kind, mbid, size)
}

func (a *handlers) writeMusicJSON(w http.ResponseWriter, r *http.Request, v any, extra ...string) {
	a.mh().WriteMusicJSON(w, r, v, extra...)
}

func (a *handlers) albumLabels(ctx context.Context, slotIDs []int64) string {
	return a.mh().AlbumLabels(ctx, slotIDs)
}

func (a *handlers) partLabel(ctx context.Context, partID int64) (titleID int64, title, season, album string) {
	return a.mh().PartLabel(ctx, partID)
}

// musicError answers music refusals with a status and a reason; anything
// else goes to library.WriteError.
func musicError(w http.ResponseWriter, err error) { music.Error(w, err, library.WriteError) }

// MusicLoops are Music's background loops (main adds them to the parts).
func (a *handlers) MusicLoops() []jobs.Loop { return a.mh().MusicLoops() }
