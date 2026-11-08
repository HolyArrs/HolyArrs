package api

import (
	"context"
	"database/sql"
	"github.com/HolyArrs/holyarrs/app/backend/internal/titleview"
	"net/http"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
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
		h := &music.Handlers{Deps: &a.s.Deps, Services: music.Services{
			Store: a.s.Store, Packages: a.s.Packages, Queue: a.s.Queue,
			Library: func() *library.Service { return a.s.Library }, Live: a.s.Live, Parts: a.s.Parts, Asks: a.s.Asks, Trailers: a.trailers,
			ImageBase: func() string { return a.s.ImageBase }, MusicControl: a.s.MusicControl, ArtistVideos: a.s.ArtistVideos,
			PartIsOn: a.partIsOn, KidMax: a.kidMax, Person: a.person, OpenArtistAsks: a.openArtistAsks,
			ArtistVideosOn: a.artistVideosOn, MusicFresh: a.musicFresh, ExploreCaching: a.exploreCaching,
			TitleID: a.titleID, LibraryError: libraryError,
			TitleView: func(ctx context.Context, t mediafiles.Title) (any, error) { return a.titleView(ctx, t) },
			PublishTitleView: func(v any) {
				if tv, ok := v.(TitleView); ok {
					a.publishTitleView(tv)
				}
			},
			Limit: func(userID int64, now time.Time) bool { return a.discover.Allow(userID, now) },
			TrackRoutes: func(m *http.ServeMux) {
				music.TrackHandlers[EpisodeSlot]{Store: a.s.Store, Slots: trackSlots}.Routes(m)
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
type musicProvider struct{ a *API }

func (a *API) musicProvider() musicProvider { return musicProvider{a} }

func (p musicProvider) On() bool { return p.a.mh().MusicOn() }

func (p musicProvider) WarmNew() { p.a.mh().WarmNew() }

func (a *API) musicTrendArtFile(ctx context.Context, kind, mbid, size string) (name, path string, ok bool) {
	return a.mh().TrendArtFile(ctx, kind, mbid, size)
}

func (a *API) pictureRecords(ctx context.Context, artists, albums []string, wait time.Duration) map[string]picEntry {
	return a.mh().PictureRecords(ctx, artists, albums, wait)
}

func (a *API) pictureVersion(kind, mbid string) string { return a.mh().PictureVersion(kind, mbid) }

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

func (a *API) albumLabels(ctx context.Context, slotIDs []int64) string {
	return a.mh().AlbumLabels(ctx, slotIDs)
}

func (a *API) partLabel(ctx context.Context, partID int64) (titleID int64, title, season, album string) {
	return a.mh().PartLabel(ctx, partID)
}

var standIn = music.StandIn

func noPicture(w http.ResponseWriter)      { music.NoPicture(w) }
func artSize(kind, size string) string     { return music.ArtSize(kind, size) }
func serverPlaceholder(h http.Header) bool { return music.ServerPlaceholder(h) }
func serverStandIn(h http.Header) bool     { return music.ServerStandIn(h) }
func coverSize(r *http.Request) string     { return music.CoverSize(r) }

// musicError answers music refusals with a status and a reason; anything
// else goes to libraryError.
func musicError(w http.ResponseWriter, err error) { music.Error(w, err, libraryError) }

// trackSlots builds the Slot views for an album's tracks, keyed by item id.
func trackSlots(ctx context.Context, db *sql.DB, p mediafiles.Part) (map[int64][]EpisodeSlot, error) {
	vs, err := database.Select[mediafiles.Version](ctx, db, "versions", "title_id = ? ORDER BY is_main DESC, id", p.TitleID)
	if err != nil {
		return nil, err
	}
	label := map[int64]string{}
	for _, v := range vs {
		label[v.ID] = v.Label
	}
	slots, err := database.Select[mediafiles.Slot](ctx, db, "slots", "item_id IN (SELECT id FROM items WHERE part_id = ? AND kind = 'track') ORDER BY id", p.ID)
	if err != nil {
		return nil, err
	}
	byItem := map[int64][]EpisodeSlot{}
	for _, s := range slots {
		es := EpisodeSlot{ID: s.ID, Version: label[s.VersionID], VersionID: s.VersionID, State: string(s.State), StateLabel: StateLabels[s.State], FileID: s.FileID}
		if s.LastReason != nil {
			es.Reason = *s.LastReason
		}
		byItem[s.ItemID] = append(byItem[s.ItemID], es)
	}
	trimEpisodeSlots(ctx, byItem)
	return byItem, nil
}

// MusicLoops are Music's background loops (main adds them to the parts).
func (a *API) MusicLoops() []jobs.Loop { return a.mh().MusicLoops() }

// trimEpisodeSlots leaves out the Library-only details on episode and track
// Slots, as titleview.TrimForViewer does for a Title's Slots.
func trimEpisodeSlots(ctx context.Context, slots map[int64][]EpisodeSlot) {
	if titleview.SeesLibraryDetail(ctx) {
		return
	}
	for _, sl := range slots {
		for i := range sl {
			sl[i].Reason = ""
			sl[i].File = titleview.TrimFile(sl[i].File)
		}
	}
}
