// Package parts holds the five parts a person can turn on, off or restart
// while the app runs (Movies, Shows, Music, Subtitles, the Wishlist), the
// one table that says which routes, screens, background loops and Job
// kinds belong to which part, and the Control that switches them.
// Everything not in a part is the core: sign-in and accounts, Sources,
// Downloaders and Downloads, Library folders, settings, backups, health,
// Alerts and the controls themselves. Sources are not a part.
package parts

import "strings"

// Part is one of the five switchable parts.
type Part string

const (
	Movies    Part = "movies"
	Shows     Part = "shows"
	Music     Part = "music"
	Subtitles Part = "subtitles"
	Wishlist  Part = "wishlist"
)

// All lists the parts in the order screens show them.
var All = []Part{Movies, Shows, Music, Subtitles, Wishlist}

// Label is the part's name on screen.
func (p Part) Label() string {
	switch p {
	case Movies:
		return "Movies"
	case Shows:
		return "Shows"
	case Music:
		return "Music"
	case Subtitles:
		return "Subtitles"
	case Wishlist:
		return "The Wishlist"
	}
	return string(p)
}

// Valid reports whether p is one of the five parts.
func Valid(p Part) bool {
	for _, q := range All {
		if p == q {
			return true
		}
	}
	return false
}

// Owner says who a route, screen, loop or Job kind belongs to: one Part,
// the core, the media parts together, or the title it is about.
type Owner string

const (
	// Core is always on.
	Core Owner = "core"
	// Media is shared by Movies, Shows and Music: it is on while any of
	// the three is on (searching, Downloads that need a decision, naming).
	Media Owner = "media"
	// ByTitle is decided by the title the request or Job is about: a
	// movie belongs to Movies, a show to Shows, an artist to Music.
	ByTitle Owner = "title"
)

// Of is the Owner for one part.
func Of(p Part) Owner { return Owner(p) }

// ForTitleKind maps a title kind (movie, show, artist) or a Library
// folder kind (music) to its part.
func ForTitleKind(kind string) (Part, bool) {
	switch kind {
	case "movie":
		return Movies, true
	case "show", "episode", "season":
		return Shows, true
	case "artist", "music", "album", "track":
		return Music, true
	}
	return "", false
}

var (
	movies    = Of(Movies)
	shows     = Of(Shows)
	music     = Of(Music)
	subtitles = Of(Subtitles)
	wishlist  = Of(Wishlist)
)

// Routes gives every route the server registers its owner. A test in
// internal/api fails for a registered route missing here, and for a row
// whose route is gone.
var Routes = map[string]Owner{
	"GET /api/v1/data-packs":                            Core,
	"GET /api/v1/data-packs/{id}":                       Core,
	"GET /api/v1/data-packs/{id}/preview":               Core,
	"POST /api/v1/data-packs/validate":                  Core,
	"POST /api/v1/data-packs":                           Core,
	"POST /api/v1/data-packs/{id}/apply":                Core,
	"GET /api/v1/titles/{id}/copies":                    ByTitle,
	"POST /api/v1/titles/{id}/versions/{vid}/adopt":     ByTitle,
	"GET /api/v1/people/search":                         movies,
	"GET /api/v1/people/popular":                        movies,
	"GET /api/v1/people/shelves":                        movies,
	"GET /api/v1/people/shelves/{id}":                   movies,
	"GET /api/v1/people/{id}":                           movies,
	"POST /api/v1/people/{id}/movies/review":            movies,
	"GET /api/v1/titles/{id}/people":                    ByTitle,
	"PUT /api/v1/titles/{id}/files/{fid}/choices":       ByTitle,
	"POST /api/v1/titles/{id}/files/{fid}/keep-and-add": ByTitle,
	"PUT /api/v1/downloads/{id}/edition":                Media,
	"GET /api/v1/collections":                           movies,
	"GET /api/v1/collections/{id}":                      movies,
	"POST /api/v1/collections/{id}/remove/preview":      movies,
	"POST /api/v1/collections/{id}/remove":              movies,
	"GET /api/v1/library/copies":                        movies,
	"POST /api/v1/library/copies/search/preview":        movies,
	"POST /api/v1/library/copies/search":                movies,

	// The app's pages, sign-in, accounts, people and keys.
	"GET /":                                     Core,
	"GET /login":                                Core,
	"GET /healthz":                              Core,
	"GET /live":                                 Core,
	"GET /api/v1/openapi.json":                  Core,
	"GET /api/v1/auth/state":                    Core,
	"POST /api/v1/auth/setup":                   Core,
	"POST /api/v1/auth/login":                   Core,
	"POST /api/v1/auth/reauth":                  Core,
	"POST /api/v1/auth/oidc/start":              Core,
	"POST /api/v1/auth/oidc/link":               Core,
	"GET /api/v1/auth/oidc/callback":            Core,
	"POST /api/v1/auth/oidc/finish":             Core,
	"POST /api/v1/auth/oidc/link-finish":        Core,
	"POST /api/v1/auth/forward/login":           Core,
	"POST /api/v1/auth/forward/link":            Core,
	"POST /api/v1/auth/login/two-factor":        Core,
	"POST /api/v1/auth/plex/start":              Core,
	"POST /api/v1/auth/plex/finish":             Core,
	"POST /api/v1/auth/media-server/{kind}":     Core,
	"GET /api/v1/auth/invites/{token}":          Core,
	"POST /api/v1/auth/invites/{token}":         Core,
	"GET /api/v1/auth/me":                       Core,
	"POST /api/v1/auth/logout":                  Core,
	"POST /api/v1/auth/password":                Core,
	"POST /api/v1/auth/two-factor/setup":        Core,
	"POST /api/v1/auth/two-factor/enable":       Core,
	"POST /api/v1/auth/two-factor/disable":      Core,
	"POST /api/v1/auth/two-factor/backup-codes": Core,
	"GET /api/v1/auth/keys":                     Core,
	"POST /api/v1/auth/keys":                    Core,
	"DELETE /api/v1/auth/keys/{id}":             Core,
	"GET /api/v1/auth/settings":                 Core,
	"PUT /api/v1/auth/settings":                 Core,
	"GET /api/v1/users":                         Core,
	"PUT /api/v1/users/{id}":                    Core,
	"POST /api/v1/users/{id}/two-factor/reset":  Core,
	"GET /api/v1/users/invites":                 Core,
	"POST /api/v1/users/invites":                Core,
	"DELETE /api/v1/users/invites/{id}":         Core,
	"GET /api/v1/auth/sessions":                 Core,
	"DELETE /api/v1/auth/sessions":              Core,
	"DELETE /api/v1/auth/sessions/{sid}":        Core,
	"GET /api/v1/users/{id}/sessions":           Core,
	"POST /api/v1/users/{id}/sessions/end-all":  Core,
	"DELETE /api/v1/users/{id}/sessions/{sid}":  Core,

	// Backups, health, Alerts, media servers, history and Jobs.
	"GET /api/v1/backups":                  Core,
	"POST /api/v1/backups":                 Core,
	"PUT /api/v1/backups/settings":         Core,
	"GET /api/v1/backups/{name}":           Core,
	"DELETE /api/v1/backups/{name}":        Core,
	"POST /api/v1/backups/restore":         Core,
	"GET /api/v1/health":                   Core,
	"GET /api/v1/health/checks":            Core,
	"POST /api/v1/health/checks/run":       Core,
	"GET /api/v1/health/states":            Core,
	"GET /api/v1/settings/health":          Core,
	"PUT /api/v1/settings/health":          Core,
	"GET /api/v1/settings/alerts":          Core,
	"PUT /api/v1/settings/alerts":          Core,
	"GET /api/v1/alerts":                   Core,
	"POST /api/v1/alerts":                  Core,
	"PUT /api/v1/alerts/{id}":              Core,
	"DELETE /api/v1/alerts/{id}":           Core,
	"POST /api/v1/alerts/{id}/test":        Core,
	"GET /api/v1/my/alerts":                Core,
	"PUT /api/v1/my/alerts":                Core,
	"GET /api/v1/media-servers":            Core,
	"POST /api/v1/media-servers":           Core,
	"PUT /api/v1/media-servers/{id}":       Core,
	"DELETE /api/v1/media-servers/{id}":    Core,
	"POST /api/v1/media-servers/{id}/test": Core,
	"GET /api/v1/events":                   Core,
	"GET /api/v1/jobs":                     Core,
	"GET /api/v1/jobs/{id}":                Core,
	"GET /api/v1/settings/trakt-key":       Core,
	"PUT /api/v1/settings/trakt-key":       Core,
	"GET /api/v1/settings/trakt-token":     Core,
	"PUT /api/v1/settings/trakt-token":     Core,
	"GET /api/v1/settings/tvdb-key":        Core,
	"PUT /api/v1/settings/tvdb-key":        Core,
	"GET /api/v1/settings/theaudiodb-key":  Core,
	"PUT /api/v1/settings/theaudiodb-key":  Core,

	// Library folders.
	"GET /api/v1/library-folders":             Core,
	"POST /api/v1/library-folders":            Core,
	"PUT /api/v1/library-folders/{id}":        Core,
	"DELETE /api/v1/library-folders/{id}":     Core,
	"POST /api/v1/library-folders/{id}/check": Core,

	// Sources, Downloaders and Downloads (linked apps are core).
	"GET /api/v1/sources":                                        Core,
	"GET /api/v1/sources/{id}/stats":                             Core,
	"POST /api/v1/sources":                                       Core,
	"PUT /api/v1/sources/{id}":                                   Core,
	"DELETE /api/v1/sources/{id}":                                Core,
	"POST /api/v1/sources/{id}/test":                             Core,
	"POST /api/v1/sources/{id}/pause":                            Core,
	"POST /api/v1/sources/{id}/resume":                           Core,
	"GET /api/v1/downloaders":                                    Core,
	"POST /api/v1/downloaders":                                   Core,
	"PUT /api/v1/downloaders/{id}":                               Core,
	"DELETE /api/v1/downloaders/{id}":                            Core,
	"POST /api/v1/downloaders/{id}/test":                         Core,
	"POST /api/v1/downloaders/{id}/pause":                        Core,
	"POST /api/v1/downloaders/{id}/resume":                       Core,
	"GET /api/v1/downloaders/{id}/attempts":                      Core,
	"POST /api/v1/downloaders/{id}/attempts/{attemptID}/abandon": Core,
	"GET /api/v1/settings/downloaders":                           Core,
	"PUT /api/v1/settings/downloaders":                           Core,
	"GET /api/v1/downloads":                                      Core,
	"GET /api/v1/downloads/view":                                 Core,
	"POST /api/v1/downloads/{id}/mark-failed":                    Core,
	"POST /api/v1/downloads/{id}/remove":                         Core,
	"GET /api/v1/imports":                                        Core,

	// Parts and the controls.
	"GET /api/v1/parts":                      Core,
	"POST /api/v1/parts/{part}/on":           Core,
	"POST /api/v1/parts/{part}/off":          Core,
	"POST /api/v1/parts/{part}/restart":      Core,
	"POST /api/v1/stages/{name}/pause":       Core,
	"POST /api/v1/stages/{name}/resume":      Core,
	"POST /api/v1/stages/{name}/restart":     Core,
	"POST /api/v1/links/{kind}/{id}/restart": Core,
	"POST /api/v1/app/restart":               Core,
	"POST /api/v1/app/safe-mode":             Core,
	"POST /api/v1/app/shutdown":              Core,
	"GET /api/v1/app/storage":                Core,
	"GET /api/v1/logs":                       Core,
	"GET /api/v1/logs/download":              Core,
	"GET /api/v1/schedules":                  Core,
	"GET /api/v1/app/update":                 Core,
	"GET /api/v1/settings/ui":                Core,
	"PUT /api/v1/settings/ui":                Core,
	"POST /api/v1/app/update/check":          Core,
	"POST /api/v1/schedules/{kind}/run":      Core,
	"PUT /api/v1/schedules/{kind}":           Core,

	// The move tool (6.1) reads old apps whatever parts are on.
	"POST /api/v1/migrate/probe":            Core,
	"POST /api/v1/migrate/read":             Core,
	"GET /api/v1/migrate/saved-keys":        Core,
	"DELETE /api/v1/migrate/saved-keys":     Core,
	"POST /api/v1/settings/import-secrets":  Core,
	"GET /api/v1/migrate/snapshots":         Core,
	"GET /api/v1/migrate/snapshots/{id}":    Core,
	"DELETE /api/v1/migrate/snapshots/{id}": Core,
	"POST /api/v1/migrate/plan":             Core,
	"POST /api/v1/migrate/dry-run":          Core,
	"GET /api/v1/migrate/moves":             Core,
	"POST /api/v1/migrate/moves":            Core,
	"GET /api/v1/migrate/moves/{id}":        Core,
	"GET /api/v1/migrate/moves/{id}/report": Core,
	"POST /api/v1/migrate/moves/{id}/undo":  Core,

	// Shared by Movies, Shows and Music.
	"GET /api/v1/titles":                      Media,
	"GET /api/v1/library-summary":             Media,
	"GET /api/v1/home":                        Media,
	"POST /api/v1/titles/refresh":             Media,
	"GET /api/v1/artwork/{name}":              Media,
	"GET /api/v1/artwork/tmdb/{size}/{file}":  Media,
	"GET /api/v1/calendar":                    Media,
	"GET /api/v1/calendar/feed-key":           Media,
	"POST /api/v1/calendar/feed-key":          Media,
	"DELETE /api/v1/calendar/feed-key":        Media,
	"GET /api/v1/calendar/feed.ics":           Media,
	"GET /api/v1/pick-rules":                  Media,
	"GET /api/v1/settings/status":             Core,
	"GET /api/v1/metadata/health":             Core,
	"GET /api/v1/settings/title-defaults":     Media,
	"PUT /api/v1/settings/title-defaults":     Media,
	"GET /api/v1/pick-rules/{id}":             Media,
	"POST /api/v1/pick-rules":                 Media,
	"PUT /api/v1/pick-rules/{id}":             Media,
	"DELETE /api/v1/pick-rules/{id}":          Media,
	"GET /api/v1/never-add":                   Media,
	"POST /api/v1/never-add":                  Media,
	"DELETE /api/v1/never-add/{id}":           Media,
	"GET /api/v1/outside-lists":               Media,
	"POST /api/v1/outside-lists":              Media,
	"PUT /api/v1/outside-lists/{id}":          Media,
	"DELETE /api/v1/outside-lists/{id}":       Media,
	"POST /api/v1/outside-lists/{id}/sync":    Media,
	"POST /api/v1/releases/{id}/choose":       Media,
	"POST /api/v1/releases/{id}/turn-down":    Media,
	"GET /api/v1/held":                        Media,
	"DELETE /api/v1/held":                     Media,
	"DELETE /api/v1/held/{id}":                Media,
	"GET /api/v1/turned-down":                 Media,
	"DELETE /api/v1/turned-down":              Media,
	"DELETE /api/v1/turned-down/{id}":         Media,
	"POST /api/v1/search/missing":             Media,
	"POST /api/v1/search/better":              Media,
	"POST /api/v1/search/feed-check":          Media,
	"GET /api/v1/searches":                    Media,
	"GET /api/v1/settings/search":             Media,
	"PUT /api/v1/settings/search":             Media,
	"GET /api/v1/needs-decision":              Media,
	"POST /api/v1/needs-decision/{id}/decide": Media,
	"GET /api/v1/rename":                      Media,
	"POST /api/v1/rename":                     Media,
	"GET /api/v1/settings/naming":             Media,
	"PUT /api/v1/settings/naming":             Media,
	"POST /api/v1/settings/naming/example":    Media,
	"GET /api/v1/settings/files":              Media,
	"PUT /api/v1/settings/files":              Media,
	"POST /api/v1/holding/clean":              Media,
	"POST /api/v1/library-folders/{id}/scan":  Media,
	"POST /api/v1/library-folders/{id}/adopt": Media,
	"POST /api/v1/library/rescan":             Media,

	// One title: its kind decides.
	"GET /api/v1/titles/{id}":                   ByTitle,
	"PUT /api/v1/titles/{id}":                   ByTitle,
	"POST /api/v1/titles/{id}/move":             ByTitle,
	"POST /api/v1/titles/{id}/versions":         ByTitle,
	"PUT /api/v1/titles/{id}/versions/{vid}":    ByTitle,
	"DELETE /api/v1/titles/{id}/versions/{vid}": ByTitle,
	"POST /api/v1/titles/{id}/refresh":          ByTitle,
	"POST /api/v1/titles/{id}/search":           ByTitle,
	"GET /api/v1/titles/{id}/releases":          ByTitle,
	"GET /api/v1/titles/{id}/rename":            ByTitle,
	"POST /api/v1/titles/{id}/rename":           ByTitle,
	"POST /api/v1/titles/{id}/delete":           ByTitle,

	// Movies.
	"POST /api/v1/titles":                                  movies,
	"GET /api/v1/titles/lookup":                            movies,
	"GET /api/v1/metadata/search":                          movies,
	"GET /api/v1/metadata/{kind}/{tmdb_id}":                Media,
	"GET /api/v1/metadata/{kind}/{tmdb_id}/more-from":      Media,
	"GET /api/v1/metadata/{kind}/{tmdb_id}/reviews":        Media,
	"GET /api/v1/metadata/show/{tmdb_id}/seasons/{number}": shows,
	"GET /api/v1/trailer/batch":                            Media,
	"GET /api/v1/trailer/{kind}/{tmdb_id}":                 Media,
	"GET /api/v1/trailer/stream/{id}/manifest.m3u8":        Media,
	"GET /api/v1/trailer/stream/{id}/seg/{n}":              Media,
	"GET /api/v1/discover":                                 Media,
	"GET /api/v1/watch-services":                           Media,
	"GET /api/v1/top10":                                    Media,
	"GET /api/v1/explore/head":                             Media,
	"GET /api/v1/explore/tail":                             Media,
	"GET /api/v1/explore/top":                              Media,
	"GET /api/v1/explore/service-top":                      Media,
	"GET /api/v1/explore/genre":                            Media,
	"GET /api/v1/explore/genre-row":                        Media,
	"GET /api/v1/explore/new":                              Media,
	"GET /api/v1/explore/new/services":                     Media,
	"GET /api/v1/explore/browse":                           Media,
	"GET /api/v1/explore/browse/section":                   Media,
	"GET /api/v1/explore/browse/category":                  Media,
	"POST /api/v1/explore/counts":                          Media,
	"GET /api/v1/explore/strip":                            Media,
	"POST /api/v1/explore/strip/counts":                    Media,
	"DELETE /api/v1/explore/strip/counts":                  Media,
	"GET /api/v1/explore/title-stats":                      Media,
	"GET /api/v1/music-trending":                           music,
	"GET /api/v1/explore/music":                            music,
	"GET /api/v1/music-trending/art/{kind}/{mbid}":         music,
	"GET /api/v1/settings/release-gate":                    movies,
	"PUT /api/v1/settings/release-gate":                    movies,

	// Shows.
	"POST /api/v1/shows":                       shows,
	"GET /api/v1/shows/lookup":                 shows,
	"GET /api/v1/settings/shows":               shows,
	"PUT /api/v1/settings/shows":               shows,
	"GET /api/v1/titles/{id}/seasons":          shows,
	"PUT /api/v1/titles/{id}/seasons/{number}": shows,
	"GET /api/v1/titles/{id}/episodes":         shows,
	"PUT /api/v1/episodes/{id}":                shows,

	// Music.
	"POST /api/v1/artists":           music,
	"GET /api/v1/artists/lookup":     music,
	"GET /api/v1/settings/music":     music,
	"PUT /api/v1/settings/music":     music,
	"GET /api/v1/titles/{id}/albums": music,
	"PUT /api/v1/titles/{id}/artist": music,
	"GET /api/v1/albums/{id}":        music,
	"GET /api/v1/albums/{id}/tracks": music,
	"GET /api/v1/albums/{id}/cover":  music,
	"PUT /api/v1/albums/{id}":        music,

	// Subtitles (4.16).
	"GET /api/v1/settings/subtitles":            subtitles,
	"PUT /api/v1/settings/subtitles":            subtitles,
	"GET /api/v1/subtitles/languages":           subtitles,
	"GET /api/v1/titles/{id}/subtitles":         subtitles,
	"PUT /api/v1/titles/{id}/subtitle-wish":     subtitles,
	"POST /api/v1/subtitles/search":             subtitles,
	"GET /api/v1/subtitles/missing":             subtitles,
	"POST /api/v1/subtitles/missing/search":     subtitles,
	"POST /api/v1/files/{id}/subtitles/choices": subtitles,
	"POST /api/v1/files/{id}/subtitles/choose":  subtitles,
	"POST /api/v1/subtitles/{id}/sync":          subtitles,
	"POST /api/v1/subtitles/{id}/turn-down":     subtitles,
	"GET /api/v1/subtitle-sites":                subtitles,
	"GET /api/v1/subtitle-sites/kinds":          subtitles,
	"POST /api/v1/subtitle-sites":               subtitles,
	"PUT /api/v1/subtitle-sites/{id}":           subtitles,
	"DELETE /api/v1/subtitle-sites/{id}":        subtitles,
	"POST /api/v1/subtitle-sites/{id}/test":     subtitles,

	// The Wishlist: asks, limits, and the messages about asks.
	"GET /api/v1/asks":                               wishlist,
	"GET /api/v1/asks/{id}":                          wishlist,
	"POST /api/v1/asks":                              wishlist,
	"GET /api/v1/asks/limit":                         wishlist,
	"DELETE /api/v1/asks/{id}":                       wishlist,
	"POST /api/v1/asks/{id}/approve":                 wishlist,
	"POST /api/v1/asks/{id}/turn-down":               wishlist,
	"GET /api/v1/ask-limits":                         wishlist,
	"PUT /api/v1/ask-limits/{id}":                    wishlist,
	"GET /api/v1/settings/asks":                      wishlist,
	"GET /api/v1/asked-by":                           wishlist,
	"GET /api/v1/my/ask-privacy":                     wishlist,
	"GET /api/v1/my/follows":                         wishlist,
	"GET /api/v1/titles/{id}/follow":                 wishlist,
	"PUT /api/v1/titles/{id}/follow":                 wishlist,
	"DELETE /api/v1/titles/{id}/follow":              wishlist,
	"GET /api/v1/metadata/show/{tmdb_id}/follow":     wishlist,
	"PUT /api/v1/metadata/show/{tmdb_id}/follow":     wishlist,
	"DELETE /api/v1/metadata/show/{tmdb_id}/follow":  wishlist,
	"GET /api/v1/metadata/movie/{tmdb_id}/follow":    wishlist,
	"PUT /api/v1/metadata/movie/{tmdb_id}/follow":    wishlist,
	"DELETE /api/v1/metadata/movie/{tmdb_id}/follow": wishlist,
	"PUT /api/v1/my/ask-privacy":                     wishlist,
	"PUT /api/v1/settings/asks":                      wishlist,
	"GET /api/v1/messages":                           wishlist,
	"POST /api/v1/messages/read":                     wishlist,
}

// Screens gives every page of the web app (its path, with [x] for a
// changing part) its owner. A test fails for a page file or a menu entry
// missing here. The web app reads this table from GET /api/v1/parts.
var Screens = map[string]Owner{
	"/people":                      movies,
	"/people/[id]":                 movies,
	"/library/collections":         movies,
	"/library/collections/[id]":    movies,
	"/library/copies":              movies,
	"/":                            Core,
	"/explore":                     Core,
	"/new":                         Media,
	"/ui":                          Core,
	"/account":                     Core,
	"/welcome":                     Core,
	"/login":                       Core,
	"/invite/[token]":              Core,
	"/downloads":                   Core,
	"/downloads/history":           Core,
	"/downloads/decide":            Media,
	"/library":                     Media,
	"/library/[id]":                ByTitle,
	"/library/[id]/albums/[album]": music,
	"/library/held":                Media,
	"/library/turned-down":         Media,
	"/library/import":              Media,
	"/library/rename":              Media,
	"/library/missing-subtitles":   subtitles,
	"/search":                      Media,
	"/title/[kind]/[tmdb]":         Media,
	"/calendar":                    Media,
	"/wishlist":                    wishlist,
	"/settings":                    Core,
	"/settings/account":            Core,
	"/settings/appearance":         Core,
	"/settings/danger":             Core,
	"/settings/health":             Core,
	"/settings/alerts":             Core,
	"/settings/backups":            Core,
	"/settings/move":               Core,
	"/settings/downloaders":        Core,
	"/settings/library-folders":    Core,
	"/settings/media-servers":      Core,
	"/settings/parts":              Core,
	"/settings/sign-in":            Core,
	"/settings/sources":            Core,
	"/settings/sources/[id]":       Core,
	"/settings/this-app":           Core,
	"/settings/logs":               Core,
	"/settings/schedules":          Core,
	"/settings/api":                Core,
	"/settings/tmdb":               Core,
	"/settings/users":              Core,
	"/settings/naming":             Media,
	"/settings/pick-rules":         Media,
	"/settings/pick-rules/[id]":    Media,
	"/settings/title-defaults":     Media,
	"/settings/outside-lists":      Media,
	"/settings/search":             Media,
	"/settings/music":              music,
	"/settings/subtitles":          subtitles,
	"/settings/wishlist":           wishlist,
}

// Loops gives every supervised background loop and every other stage the
// health page shows (worker pools, guarded work) its owner. A loop of a
// part stops while the part is off.
var Loops = map[string]Owner{
	"backups":          Core,
	"downloads":        Core,
	"schedules":        Core,
	"sessions":         Core,
	"log_secrets":      Core,
	"health":           Core,
	"alerts":           Core,
	"events":           Core,
	"media_servers":    Core,
	"jobs_core":        Core,
	"jobs_io":          Core,
	"jobs_heavy":       Core,
	"stranded_imports": Media,
	"gates":            movies,
	"wishlist":         wishlist,
}

// JobKinds gives every Job kind its owner. A Job of a part that is off
// waits in the queue (it is not lost) and runs once the part is on again;
// a scheduled kind of a part that is off is not queued.
var JobKinds = map[string]Owner{
	"health_check":          Core,
	"check_library_folders": Core,
	// A move checks each part itself (a part that is off is left out).
	"migrate_move": Core,
	"migrate_undo": Core,
	// Shared work: on while any of Movies, Shows and Music is on.
	"search_missing":     Media,
	"feed_check":         Media,
	"grab_held":          Media,
	"sync_outside_lists": Media,
	"rename_files":       Media,
	"clean_holding":      Media,
	"rescan_library":     Media,
	// The title (or Library folder) the Job is about decides.
	"search_slot":         ByTitle,
	"grab":                ByTitle,
	"import_download":     ByTitle,
	"refresh_title":       ByTitle,
	"scan_title":          ByTitle,
	"move_title":          ByTitle,
	"scan_library_folder": ByTitle,
	"adopt_library":       ByTitle,
	// One part.
	"refresh_titles":   movies,
	"refresh_shows":    shows,
	"search_season":    shows,
	"refresh_artists":  music,
	"artist_pictures":  music,
	"search_album":     music,
	"subtitle_search":  subtitles,
	"subtitle_missing": subtitles,
	"subtitle_better":  subtitles,
	"subtitle_look":    subtitles,
}

// ScreenOwner finds the owner of a web path such as /library/12: the
// table row that matches, with [x] standing for one path segment; a row
// with fewer [x] segments wins (/library/held over /library/[id]).
func ScreenOwner(path string) (Owner, bool) {
	got := strings.Split(strings.Trim(path, "/"), "/")
	best, bestWild := Owner(""), -1
	for pattern, o := range Screens {
		want := strings.Split(strings.Trim(pattern, "/"), "/")
		if len(want) != len(got) {
			continue
		}
		wild := 0
		for i := range want {
			switch {
			case strings.HasPrefix(want[i], "["):
				wild++
			case want[i] != got[i]:
				wild = -1
			}
			if wild < 0 {
				break
			}
		}
		if wild >= 0 && (bestWild < 0 || wild < bestWild) {
			best, bestWild = o, wild
		}
	}
	return best, bestWild >= 0
}

// SafeLibrary are the routes that stay reachable in safe mode although
// every part is off: the Library list and one title's pages (read), and
// editing or removing a title. Nothing optional runs through them: Jobs
// they queue wait for the next normal start.
var SafeLibrary = map[string]bool{
	"GET /api/v1/titles":               true,
	"GET /api/v1/library-summary":      true,
	"GET /api/v1/home":                 true,
	"GET /api/v1/artwork/{name}":       true,
	"GET /api/v1/titles/{id}":          true,
	"PUT /api/v1/titles/{id}":          true,
	"POST /api/v1/titles/{id}/delete":  true,
	"GET /api/v1/titles/{id}/seasons":  true,
	"GET /api/v1/titles/{id}/episodes": true,
	"GET /api/v1/titles/{id}/albums":   true,
	"GET /api/v1/albums/{id}":          true,
	"GET /api/v1/albums/{id}/tracks":   true,
}

// SafeScreens are the pages that show in safe mode for the same reason.
var SafeScreens = []string{"/library", "/library/[id]", "/library/[id]/albums/[album]"}
