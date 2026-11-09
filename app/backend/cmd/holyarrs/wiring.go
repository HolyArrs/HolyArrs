package main

import (
	"context"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/backups"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/parts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/health"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
	"github.com/HolyArrs/holyarrs/app/backend/internal/music"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// registerHooks hands core the functions, parts and feature names that
// modules own. Each module that
// leaves a leftover package adds its registrations here.
func registerHooks() {
	startup.RegisterHeldFeatures()
	parts.Register()
	secrets.RegisterRewriter("sources", sources.EncryptPlainExtras)
}

// routes returns the handler that serves every route of the app. Each module
// that is split out of this package adds one registration line here.
func routes(a *handlers, c config) http.Handler {
	rt := webserver.NewRouter()
	a.Routes(rt)
	a.ModuleRoutes(rt)
	return webserver.UnderURLBase(c.URLBase, webserver.Compress(limitBodies(a.Chain(rt))))
}

// backupsHealth gives the health engine the state of backups.
func backupsHealth(bm *backups.Manager) func(context.Context) (health.Backups, error) {
	return func(ctx context.Context) (health.Backups, error) {
		newest, every, lastErr, err := bm.State(ctx)
		return health.Backups{Newest: newest, Every: every, LastErr: lastErr, Started: bm.Passed(),
			KeyPrint: bm.KeyPrint(), KeyKitPrint: bm.KeyKitPrint(ctx)}, err
	}
}

// musicDeps hands the music module its metadata calls and feature switches.
// The Deezer and bio reads are the server's, or nil when a direct stand-in
// answers instead.
func musicDeps(svc *metadata.Service, deezer func(context.Context, string) (metadata.DeezerProfile, error),
	deezerAlbum func(context.Context, int64) (metadata.DeezerAlbumInfo, error),
	bio func(context.Context, string) (*metadata.WikiSummary, error)) music.Deps {
	return music.Deps{MusicTrending: svc.MusicTrending, MusicPictures: svc.MusicPictures, MusicNew: svc.MusicNewWindow, MusicSimilar: svc.MusicSimilar, MusicCharts: svc.MusicCharts, MusicGenres: svc.MusicGenreList, MusicGenreTile: svc.MusicGenreTile,
		MusicShelves: svc.MusicShelves, MusicPools: svc.MusicPools, MusicRowsTplOff: startup.FeatureOn("music_rows_tpl_off"), MusicListeners: svc.MusicListeners, MusicSimilarSet: svc.MusicSimilarSet, MusicArtistData: svc.MusicArtistData,
		MusicSuggest: svc.MusicSuggest, MusicTopSongs: svc.MusicTopSongs, MusicRising: svc.MusicRisingToday, MusicDiscography: svc.MusicDiscography,
		MusicDeezer: deezer, MusicDeezerAlbum: deezerAlbum, MusicArtistBio: bio}
}
