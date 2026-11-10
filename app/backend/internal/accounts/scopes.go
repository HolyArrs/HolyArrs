package accounts

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// RoutePerms says what each route needs. Routes missing here are for the
// owner only, so a new route is closed until someone decides who may use it.
var RoutePerms = map[string]webserver.Perm{
	"GET /api/v1/data-packs":                            webserver.NeedOwner,
	"GET /api/v1/data-packs/{id}":                       webserver.NeedOwner,
	"GET /api/v1/data-packs/{id}/preview":               webserver.NeedOwner,
	"POST /api/v1/data-packs/validate":                  webserver.NeedOwner,
	"POST /api/v1/data-packs":                           webserver.NeedOwner,
	"POST /api/v1/data-packs/{id}/apply":                webserver.NeedOwner,
	"GET /api/v1/titles/{id}/copies":                    webserver.PermManageLibrary,
	"POST /api/v1/titles/{id}/versions/{vid}/adopt":     webserver.PermManageLibrary,
	"GET /api/v1/people/search":                         webserver.NeedSignedIn,
	"GET /api/v1/people/popular":                        webserver.NeedSignedIn,
	"GET /api/v1/people/shelves":                        webserver.NeedSignedIn,
	"GET /api/v1/people/shelves/{id}":                   webserver.NeedSignedIn,
	"GET /api/v1/people/{id}":                           webserver.NeedSignedIn,
	"GET /api/v1/wiki/{kind}/{id}":                      webserver.NeedSignedIn,
	"POST /api/v1/people/{id}/movies/review":            webserver.NeedSignedIn,
	"GET /api/v1/titles/{id}/people":                    webserver.NeedSignedIn,
	"PUT /api/v1/titles/{id}/files/{fid}/choices":       webserver.PermManageLibrary,
	"POST /api/v1/titles/{id}/files/{fid}/keep-and-add": webserver.PermManageLibrary,
	"PUT /api/v1/downloads/{id}/edition":                webserver.PermManageLibrary,
	"GET /api/v1/collections":                           webserver.NeedSignedIn,
	"GET /api/v1/collections/{id}":                      webserver.NeedSignedIn,
	"POST /api/v1/collections/{id}/remove/preview":      webserver.PermManageLibrary,
	"POST /api/v1/collections/{id}/remove":              webserver.PermManageLibrary,
	"GET /api/v1/library/copies":                        webserver.PermManageLibrary,
	"POST /api/v1/library/copies/search/preview":        webserver.PermManageLibrary,
	"POST /api/v1/library/copies/search":                webserver.PermManageLibrary,

	// The app's pages and files; the pages themselves ask the API.
	"GET /":      webserver.NeedSelf,
	"GET /login": webserver.NeedSelf,

	// Optional outside sign-in. These routes validate their own bounded flow.
	"POST /api/v1/auth/oidc/start":    webserver.NeedPublic,
	"GET /api/v1/auth/oidc/callback":  webserver.NeedPublic,
	"POST /api/v1/auth/oidc/finish":   webserver.NeedPublic,
	"POST /api/v1/auth/forward/login": webserver.NeedPublic,
	// The sign-in page's art: picture addresses only, any rating (like Overseerr).
	"GET /api/v1/public/signin-art": webserver.NeedPublic,
	// Its pictures, same-origin; only the files the answer above holds.
	"GET /api/v1/public/signin-pic/t/p/{size}/{file}": webserver.NeedPublic,

	// The live stream filters each event by audience: managers receive
	// everything, anyone else only events for their own items.
	"GET /live":                  webserver.NeedSignedIn,
	"POST /api/v1/live/interest": webserver.NeedSignedIn,

	// My account.
	"GET /api/v1/auth/me":                       webserver.NeedSelf,
	"POST /api/v1/auth/logout":                  webserver.NeedSelf,
	"POST /api/v1/auth/reauth":                  webserver.NeedSelf,
	"POST /api/v1/auth/setup/done":              webserver.NeedOwner,
	"POST /api/v1/auth/oidc/link":               webserver.NeedSelf,
	"GET /api/v1/auth/sso/links":                webserver.NeedSelf,
	"POST /api/v1/auth/sso/unlink":              webserver.NeedSelf,
	"GET /api/v1/auth/plex/my-link":             webserver.NeedSelf,
	"POST /api/v1/auth/plex/my-link/start":      webserver.NeedSelf,
	"POST /api/v1/auth/plex/my-link/finish":     webserver.NeedSelf,
	"DELETE /api/v1/auth/plex/my-link":          webserver.NeedSelf,
	"POST /api/v1/auth/oidc/link-finish":        webserver.NeedSelf,
	"POST /api/v1/auth/forward/link":            webserver.NeedSelf,
	"POST /api/v1/auth/password":                webserver.NeedSelf,
	"POST /api/v1/auth/two-factor/setup":        webserver.NeedSelf,
	"POST /api/v1/auth/two-factor/enable":       webserver.NeedSelf,
	"POST /api/v1/auth/two-factor/disable":      webserver.NeedSelf,
	"POST /api/v1/auth/two-factor/backup-codes": webserver.NeedSelf,
	"GET /api/v1/auth/sessions":                 webserver.NeedSelf,
	"DELETE /api/v1/auth/sessions":              webserver.NeedSelf,
	"DELETE /api/v1/auth/sessions/{sid}":        webserver.NeedSelf,
	"PUT /api/v1/account/avatar":                webserver.NeedSelf,
	"DELETE /api/v1/account/avatar":             webserver.NeedSelf,
	"GET /api/v1/users/{id}/avatar":             webserver.NeedSignedIn,

	// Looking at the Library and searching for titles.
	"GET /api/v1/titles":                                   webserver.NeedSignedIn,
	"GET /api/v1/titles/owned":                             webserver.NeedSignedIn,
	"GET /api/v1/library/search":                           webserver.NeedSignedIn,
	"GET /api/v1/search/suggest":                           webserver.NeedSignedIn,
	"GET /api/v1/search":                                   webserver.NeedSignedIn,
	"GET /api/v1/home":                                     webserver.NeedSignedIn,
	"GET /api/v1/titles/{id}":                              webserver.NeedSignedIn,
	"GET /api/v1/titles/{id}/search-outcome":               webserver.NeedSignedIn,
	"GET /api/v1/artwork/{name}":                           webserver.NeedSignedIn,
	"GET /api/v1/artwork/tmdb/{size}/{file}":               webserver.NeedSignedIn,
	"GET /api/v1/artwork/img/{kind}/{id}/{file}":           webserver.NeedSignedIn,
	"GET /api/v1/metadata/search":                          webserver.NeedSignedIn,
	"GET /api/v1/metadata/{kind}/{tmdb_id}":                webserver.NeedSignedIn,
	"GET /api/v1/metadata/{kind}/{tmdb_id}/more-from":      webserver.NeedSignedIn,
	"GET /api/v1/metadata/{kind}/{tmdb_id}/reviews":        webserver.NeedSignedIn,
	"GET /api/v1/metadata/{kind}/{tmdb_id}/anime":          webserver.NeedSignedIn,
	"GET /api/v1/extras/{kind}/{id}":                       webserver.NeedSignedIn,
	"GET /api/v1/metadata/show/{tmdb_id}/seasons/{number}": webserver.NeedSignedIn,
	"GET /api/v1/trailer/batch":                            webserver.NeedSignedIn,
	"GET /api/v1/trailer/{kind}/{tmdb_id}":                 webserver.NeedSignedIn,
	"GET /api/v1/trailer/stream/{id}/manifest.m3u8":        webserver.NeedSignedIn,
	"GET /api/v1/trailer/stream/{id}/seg/{n}":              webserver.NeedSignedIn,
	"GET /api/v1/settings/ui":                              webserver.NeedSignedIn,
	"PUT /api/v1/settings/ui":                              webserver.PermSettings,
	"GET /api/v1/discover":                                 webserver.NeedSignedIn,
	"GET /api/v1/watch-services":                           webserver.NeedSignedIn,
	"GET /api/v1/explore/head":                             webserver.NeedSignedIn,
	"GET /api/v1/explore/tail":                             webserver.NeedSignedIn,
	"GET /api/v1/explore/top":                              webserver.NeedSignedIn,
	"GET /api/v1/explore/service-top":                      webserver.NeedSignedIn,
	"GET /api/v1/explore/genre":                            webserver.NeedSignedIn,
	"GET /api/v1/explore/genre-row":                        webserver.NeedSignedIn,
	"GET /api/v1/explore/new":                              webserver.NeedSignedIn,
	"GET /api/v1/explore/new/services":                     webserver.NeedSignedIn,
	"GET /api/v1/explore/new/service":                      webserver.NeedSignedIn,
	"GET /api/v1/explore/new/music":                        webserver.NeedSignedIn,
	"GET /api/v1/metadata/album/{mbid}/follow":             webserver.NeedSignedIn,
	"PUT /api/v1/metadata/album/{mbid}/follow":             webserver.NeedSignedIn,
	"DELETE /api/v1/metadata/album/{mbid}/follow":          webserver.NeedSignedIn,
	"GET /api/v1/explore/browse":                           webserver.NeedSignedIn,
	"GET /api/v1/explore/browse/section":                   webserver.NeedSignedIn,
	"GET /api/v1/explore/browse/category":                  webserver.NeedSignedIn,
	"POST /api/v1/explore/counts":                          webserver.NeedSignedIn,
	"GET /api/v1/explore/strip":                            webserver.NeedSignedIn,
	"POST /api/v1/explore/strip/counts":                    webserver.NeedSignedIn,
	"DELETE /api/v1/explore/strip/counts":                  webserver.NeedSignedIn,
	"GET /api/v1/explore/title-stats":                      webserver.NeedOwner,
	"GET /api/v1/music-trending":                           webserver.NeedSignedIn,
	"GET /api/v1/explore/music":                            webserver.NeedSignedIn,
	"GET /api/v1/explore/music/rising":                     webserver.NeedSignedIn,
	"GET /api/v1/music-trending/art/{kind}/{mbid}":         webserver.NeedSignedIn,
	"GET /api/v1/music/genre/{slug}":                       webserver.NeedSignedIn,
	"GET /api/v1/music/browse/{kind}/{slug}":               webserver.NeedSignedIn,
	"GET /api/v1/music/browse-tiles":                       webserver.NeedSignedIn,
	"GET /api/v1/music/genre-art/{slug}":                   webserver.NeedSignedIn,
	"GET /api/v1/music/artists/{mbid}":                     webserver.NeedSignedIn,
	"GET /api/v1/music/albums/{mbid}":                      webserver.NeedSignedIn,
	"GET /api/v1/metadata/health":                          webserver.NeedSignedIn,
	"GET /api/v1/titles/lookup":                            webserver.NeedSignedIn,
	"GET /api/v1/shows/lookup":                             webserver.NeedSignedIn,
	"GET /api/v1/titles/{id}/seasons":                      webserver.NeedSignedIn,
	"GET /api/v1/titles/{id}/episodes":                     webserver.NeedSignedIn,
	"GET /api/v1/artists/lookup":                           webserver.NeedSignedIn,
	"GET /api/v1/titles/{id}/albums":                       webserver.NeedSignedIn,
	"GET /api/v1/albums/{id}":                              webserver.NeedSignedIn,
	"GET /api/v1/albums/{id}/tracks":                       webserver.NeedSignedIn,
	"GET /api/v1/albums/{id}/cover":                        webserver.NeedSignedIn,
	"GET /api/v1/openapi.json":                             webserver.NeedSignedIn,

	// AI connections: the route checks the key itself; the tools check
	// the person's permissions one by one.
	"POST /api/v1/mcp":        webserver.NeedSignedIn,
	"GET /api/v1/mcp":         webserver.NeedSignedIn,
	"POST /mcp":               webserver.NeedSignedIn,
	"GET /mcp":                webserver.NeedSignedIn,
	"GET /api/v1/settings/ai": webserver.NeedSignedIn,
	"PUT /api/v1/settings/ai": webserver.NeedOwner,
	// AI app sign-in (ai_signin.go): the app's own calls are public; allowing it
	// needs the person's browser session.
	"GET /.well-known/oauth-protected-resource":           webserver.NeedPublic,
	"GET /.well-known/oauth-protected-resource/{path...}": webserver.NeedPublic,
	"GET /.well-known/oauth-authorization-server":         webserver.NeedPublic,
	"POST /oauth/register":                                webserver.NeedPublic,
	"GET /oauth/authorize":                                webserver.NeedPublic,
	"POST /oauth/token":                                   webserver.NeedPublic,
	"GET /api/v1/auth/ai-connect":                         webserver.NeedSignedIn,
	"POST /api/v1/auth/ai-connect":                        webserver.PermSettings, // as for making a key: the app gets one

	// Requests: everyone sees their own asks and messages; asking needs
	// ask, deciding needs approve.
	"GET /api/v1/asks":                               webserver.NeedSignedIn,
	"GET /api/v1/asks/{id}":                          webserver.NeedSignedIn,
	"GET /api/v1/asks/{id}/parts":                    webserver.NeedSignedIn,
	"POST /api/v1/asks":                              webserver.PermAsk,
	"GET /api/v1/asks/limit":                         webserver.PermAsk,
	"DELETE /api/v1/asks/{id}":                       webserver.PermAsk,
	"POST /api/v1/asks/{id}/approve":                 webserver.PermApprove,
	"POST /api/v1/asks/{id}/turn-down":               webserver.PermApprove,
	"GET /api/v1/messages":                           webserver.NeedSignedIn,
	"POST /api/v1/messages/read":                     webserver.NeedSignedIn,
	"GET /api/v1/my/alerts":                          webserver.NeedSignedIn,
	"PUT /api/v1/my/alerts":                          webserver.NeedSignedIn,
	"GET /api/v1/my/plex-watchlist":                  webserver.NeedSignedIn,
	"PUT /api/v1/my/plex-watchlist":                  webserver.NeedSignedIn,
	"POST /api/v1/my/plex-watchlist/check":           webserver.NeedSignedIn,
	"GET /api/v1/asked-by":                           webserver.NeedSignedIn,
	"GET /api/v1/my/profile":                         webserver.NeedSignedIn,
	"PUT /api/v1/my/profile":                         webserver.NeedSignedIn,
	"PUT /api/v1/my/language":                        webserver.NeedSignedIn,
	"GET /api/v1/my/follows":                         webserver.NeedSignedIn,
	"GET /api/v1/titles/{id}/follow":                 webserver.NeedSignedIn,
	"PUT /api/v1/titles/{id}/follow":                 webserver.NeedSignedIn,
	"DELETE /api/v1/titles/{id}/follow":              webserver.NeedSignedIn,
	"GET /api/v1/metadata/show/{tmdb_id}/follow":     webserver.NeedSignedIn,
	"PUT /api/v1/metadata/show/{tmdb_id}/follow":     webserver.NeedSignedIn,
	"DELETE /api/v1/metadata/show/{tmdb_id}/follow":  webserver.NeedSignedIn,
	"GET /api/v1/metadata/movie/{tmdb_id}/follow":    webserver.NeedSignedIn,
	"PUT /api/v1/metadata/movie/{tmdb_id}/follow":    webserver.NeedSignedIn,
	"DELETE /api/v1/metadata/movie/{tmdb_id}/follow": webserver.NeedSignedIn,
	"GET /api/v1/ask-limits":                         webserver.NeedUsers,
	"PUT /api/v1/ask-limits/{id}":                    webserver.NeedUsers,
	"GET /api/v1/settings/asks":                      webserver.PermSettings,
	"PUT /api/v1/settings/asks":                      webserver.PermSettings,
	"GET /api/v1/settings/alerts":                    webserver.PermSettings,
	"GET /api/v1/subtitles/languages":                webserver.PermManageLibrary,
	"GET /api/v1/settings/subtitles":                 webserver.PermSettings,
	"PUT /api/v1/settings/subtitles":                 webserver.PermSettings,
	"GET /api/v1/titles/{id}/subtitles":              webserver.PermManageLibrary,
	"PUT /api/v1/titles/{id}/subtitle-wish":          webserver.PermManageLibrary,
	"POST /api/v1/subtitles/search":                  webserver.PermManageLibrary,
	"GET /api/v1/subtitles/missing":                  webserver.PermManageLibrary,
	"POST /api/v1/subtitles/missing/search":          webserver.PermManageLibrary,
	"POST /api/v1/files/{id}/subtitles/choices":      webserver.PermManageLibrary,
	"POST /api/v1/files/{id}/subtitles/choose":       webserver.PermManageLibrary,
	"POST /api/v1/subtitles/{id}/sync":               webserver.PermManageLibrary,
	"POST /api/v1/subtitles/{id}/turn-down":          webserver.PermManageLibrary,
	"GET /api/v1/subtitle-sites":                     webserver.PermManageLibrary,
	"GET /api/v1/subtitle-sites/kinds":               webserver.PermManageLibrary,
	"POST /api/v1/subtitle-sites":                    webserver.PermSettings,
	"PUT /api/v1/subtitle-sites/{id}":                webserver.PermSettings,
	"DELETE /api/v1/subtitle-sites/{id}":             webserver.PermSettings,
	"POST /api/v1/subtitle-sites/{id}/test":          webserver.PermSettings,
	"PUT /api/v1/settings/alerts":                    webserver.PermSettings,
	"GET /api/v1/settings/url-base":                  webserver.PermSettings,
	"PUT /api/v1/settings/url-base":                  webserver.PermSettings,
	"GET /api/v1/alerts":                             webserver.PermSettings,
	"POST /api/v1/alerts":                            webserver.PermSettings,
	"PUT /api/v1/alerts/{id}":                        webserver.PermSettings,
	"DELETE /api/v1/alerts/{id}":                     webserver.PermSettings,
	"POST /api/v1/alerts/{id}/test":                  webserver.PermSettings,
	"GET /api/v1/media-servers":                      webserver.PermSettings,
	"POST /api/v1/media-servers":                     webserver.PermSettings,
	"PUT /api/v1/media-servers/{id}":                 webserver.PermSettings,
	"DELETE /api/v1/media-servers/{id}":              webserver.PermSettings,
	"POST /api/v1/media-servers/{id}/test":           webserver.PermSettings,

	// Running the Library.
	"POST /api/v1/titles":           webserver.PermManageLibrary,
	"PUT /api/v1/titles/{id}":       webserver.PermManageLibrary,
	"POST /api/v1/titles/{id}/move": webserver.PermManageLibrary,
	// Shows.
	"POST /api/v1/shows":                       webserver.PermManageLibrary,
	"PUT /api/v1/titles/{id}/seasons/{number}": webserver.PermManageLibrary,
	"PUT /api/v1/episodes/{id}":                webserver.PermManageLibrary,
	"PUT /api/v1/settings/shows":               webserver.PermSettings,
	// Music.
	"POST /api/v1/artists":           webserver.PermManageLibrary,
	"PUT /api/v1/titles/{id}/artist": webserver.PermManageLibrary,
	"PUT /api/v1/albums/{id}":        webserver.PermManageLibrary,
	"GET /api/v1/settings/music":     webserver.PermManageLibrary,
	"PUT /api/v1/settings/music":     webserver.PermSettings,
	// Organise.
	"GET /api/v1/titles/{id}/rename":            webserver.PermManageLibrary,
	"POST /api/v1/titles/{id}/rename":           webserver.PermManageLibrary,
	"GET /api/v1/rename":                        webserver.PermManageLibrary,
	"POST /api/v1/rename":                       webserver.PermManageLibrary,
	"GET /api/v1/jobs/{id}":                     webserver.PermManageLibrary,
	"GET /api/v1/jobs/active":                   webserver.PermManageLibrary,
	"GET /api/v1/settings/naming":               webserver.PermManageLibrary,
	"PUT /api/v1/settings/naming":               webserver.PermSettings,
	"POST /api/v1/settings/naming/example":      webserver.PermManageLibrary,
	"GET /api/v1/settings/files":                webserver.PermManageLibrary,
	"PUT /api/v1/settings/files":                webserver.PermSettings,
	"POST /api/v1/titles/{id}/delete":           webserver.PermManageLibrary,
	"POST /api/v1/holding/clean":                webserver.PermManageLibrary,
	"GET /api/v1/holding":                       webserver.PermManageLibrary,
	"POST /api/v1/holding/restore":              webserver.PermManageLibrary,
	"POST /api/v1/library-folders/{id}/scan":    webserver.PermManageLibrary,
	"POST /api/v1/library-folders/{id}/adopt":   webserver.PermManageLibrary,
	"POST /api/v1/manual-import/scan":           webserver.PermSettings,
	"POST /api/v1/manual-import":                webserver.PermSettings,
	"GET /api/v1/settings/import-folders":       webserver.PermSettings,
	"PUT /api/v1/settings/import-folders":       webserver.NeedUsers, // widens what import by hand may read
	"POST /api/v1/titles/{id}/versions":         webserver.PermManageLibrary,
	"PUT /api/v1/titles/{id}/versions/{vid}":    webserver.PermManageLibrary,
	"DELETE /api/v1/titles/{id}/versions/{vid}": webserver.PermManageLibrary,
	"POST /api/v1/titles/{id}/refresh":          webserver.PermManageLibrary,
	"POST /api/v1/titles/refresh":               webserver.PermManageLibrary,
	"POST /api/v1/titles/{id}/search":           webserver.PermManageLibrary,
	"GET /api/v1/titles/{id}/releases":          webserver.PermManageLibrary,
	// Searching.
	"POST /api/v1/releases/{id}/choose":                          webserver.PermManageLibrary,
	"POST /api/v1/releases/{id}/turn-down":                       webserver.PermManageLibrary,
	"GET /api/v1/held":                                           webserver.PermManageLibrary,
	"DELETE /api/v1/held":                                        webserver.PermManageLibrary,
	"DELETE /api/v1/held/{id}":                                   webserver.PermManageLibrary,
	"GET /api/v1/turned-down":                                    webserver.PermManageLibrary,
	"DELETE /api/v1/turned-down":                                 webserver.PermManageLibrary,
	"DELETE /api/v1/turned-down/{id}":                            webserver.PermManageLibrary,
	"POST /api/v1/search/missing":                                webserver.PermManageLibrary,
	"POST /api/v1/search/better":                                 webserver.PermManageLibrary,
	"POST /api/v1/search/feed-check":                             webserver.PermManageLibrary,
	"GET /api/v1/searches":                                       webserver.PermManageLibrary,
	"GET /api/v1/settings/search":                                webserver.PermSettings,
	"GET /api/v1/bridges":                                        webserver.PermManageLibrary,
	"POST /api/v1/bridges":                                       webserver.PermSettings,
	"POST /api/v1/bridges/find":                                  webserver.PermSettings,
	"POST /api/v1/bridges/test":                                  webserver.PermSettings,
	"POST /api/v1/bridges/{id}/check":                            webserver.PermSettings,
	"DELETE /api/v1/bridges/{id}":                                webserver.PermSettings,
	"PUT /api/v1/settings/search":                                webserver.PermSettings,
	"GET /api/v1/calendar":                                       webserver.NeedSignedIn,
	"GET /api/v1/library-folders":                                webserver.PermManageLibrary,
	"GET /api/v1/pick-rules":                                     webserver.PermManageLibrary,
	"GET /api/v1/settings/status":                                webserver.PermSettings,
	"GET /api/v1/settings/title-defaults":                        webserver.PermManageLibrary,
	"PUT /api/v1/settings/title-defaults":                        webserver.PermSettings,
	"GET /api/v1/pick-rules/{id}":                                webserver.PermManageLibrary,
	"POST /api/v1/pick-rules":                                    webserver.PermSettings,
	"PUT /api/v1/pick-rules/{id}":                                webserver.PermSettings,
	"DELETE /api/v1/pick-rules/{id}":                             webserver.PermSettings,
	"GET /api/v1/own-traits":                                     webserver.PermManageLibrary,
	"POST /api/v1/own-traits":                                    webserver.PermSettings,
	"PUT /api/v1/own-traits/{id}":                                webserver.PermSettings,
	"DELETE /api/v1/own-traits/{id}":                             webserver.PermSettings,
	"GET /api/v1/trash/profiles":                                 webserver.PermSettings,
	"POST /api/v1/pick-rules/{id}/trash":                         webserver.PermSettings,
	"GET /api/v1/never-add":                                      webserver.PermManageLibrary,
	"POST /api/v1/never-add":                                     webserver.PermManageLibrary,
	"DELETE /api/v1/never-add/{id}":                              webserver.PermManageLibrary,
	"GET /api/v1/settings/release-gate":                          webserver.PermManageLibrary,
	"GET /api/v1/settings/shows":                                 webserver.PermManageLibrary,
	"GET /api/v1/downloads":                                      webserver.PermManageLibrary,
	"GET /api/v1/imports":                                        webserver.PermManageLibrary,
	"GET /api/v1/events":                                         webserver.PermManageLibrary,
	"GET /api/v1/jobs":                                           webserver.PermManageLibrary,
	"GET /api/v1/sources":                                        webserver.PermManageLibrary,
	"GET /api/v1/sources/{id}/stats":                             webserver.PermManageLibrary,
	"POST /api/v1/sources":                                       webserver.PermSettings,
	"PUT /api/v1/sources/{id}":                                   webserver.PermSettings,
	"DELETE /api/v1/sources/{id}":                                webserver.PermSettings,
	"POST /api/v1/sources/{id}/test":                             webserver.PermSettings,
	"POST /api/v1/sources/{id}/pause":                            webserver.PermSettings,
	"POST /api/v1/sources/{id}/resume":                           webserver.PermSettings,
	"POST /api/v1/sources/{id}/try":                              webserver.PermSettings,
	"POST /api/v1/sources/check":                                 webserver.PermSettings,
	"GET /api/v1/downloaders":                                    webserver.PermManageLibrary,
	"GET /api/v1/tags":                                           webserver.PermManageLibrary,
	"POST /api/v1/downloaders":                                   webserver.PermSettings,
	"PUT /api/v1/downloaders/{id}":                               webserver.PermSettings,
	"DELETE /api/v1/downloaders/{id}":                            webserver.PermSettings,
	"POST /api/v1/downloaders/{id}/test":                         webserver.PermSettings,
	"POST /api/v1/downloaders/{id}/pause":                        webserver.PermSettings,
	"POST /api/v1/downloaders/{id}/resume":                       webserver.PermSettings,
	"GET /api/v1/downloaders/{id}/attempts":                      webserver.PermSettings,
	"POST /api/v1/downloaders/{id}/attempts/{attemptID}/abandon": webserver.PermSettings,
	"POST /api/v1/downloads/{id}/mark-failed":                    webserver.PermManageLibrary,
	"GET /api/v1/downloads/view":                                 webserver.NeedSignedIn, // everyone; non-managers see only their own asks
	"POST /api/v1/downloads/{id}/remove":                         webserver.PermManageLibrary,
	"POST /api/v1/downloads/{id}/pause":                          webserver.PermManageLibrary,
	"POST /api/v1/downloads/{id}/resume":                         webserver.PermManageLibrary,
	"POST /api/v1/downloads/{id}/top":                            webserver.PermManageLibrary,
	"POST /api/v1/downloads/{id}/recheck":                        webserver.PermManageLibrary,
	"POST /api/v1/downloads/{id}/reannounce":                     webserver.PermManageLibrary,
	"GET /api/v1/downloads/{id}/swarm":                           webserver.PermManageLibrary,
	"GET /api/v1/downloads/{id}/detail":                          webserver.PermManageLibrary,
	"GET /api/v1/downloads/{id}/files":                           webserver.PermManageLibrary,
	"PUT /api/v1/downloads/{id}/files":                           webserver.PermManageLibrary,
	"GET /api/v1/downloads/clients":                              webserver.PermManageLibrary,
	"POST /api/v1/downloads/pause-all":                           webserver.PermManageLibrary,
	"POST /api/v1/downloads/resume-all":                          webserver.PermManageLibrary,
	"PUT /api/v1/downloaders/{id}/speed-limit":                   webserver.PermManageLibrary,
	"POST /api/v1/downloaders/{id}/items/{hash}/pause":           webserver.PermManageLibrary,
	"POST /api/v1/downloaders/{id}/items/{hash}/resume":          webserver.PermManageLibrary,
	"POST /api/v1/downloaders/{id}/items/{hash}/top":             webserver.PermManageLibrary,
	"POST /api/v1/downloaders/{id}/items/{hash}/up":              webserver.PermManageLibrary,
	"POST /api/v1/downloaders/{id}/items/{hash}/down":            webserver.PermManageLibrary,
	"POST /api/v1/downloaders/{id}/items/{hash}/bottom":          webserver.PermManageLibrary,
	"POST /api/v1/downloaders/{id}/items/{hash}/priority":        webserver.PermManageLibrary,
	"POST /api/v1/downloaders/{id}/items/{hash}/recheck":         webserver.PermManageLibrary,
	"POST /api/v1/downloaders/{id}/items/{hash}/reannounce":      webserver.PermManageLibrary,
	"POST /api/v1/downloaders/{id}/items/{hash}/remove":          webserver.PermManageLibrary, // outside torrents also need PermSettings (checked in the handler)
	"POST /api/v1/downloaders/{id}/items/{hash}/retry":           webserver.PermManageLibrary, // outside Usenet jobs also need PermSettings (checked in the handler)
	"GET /api/v1/downloaders/{id}/items/{hash}/usenet":           webserver.PermManageLibrary,
	"GET /api/v1/downloads/{id}/usenet":                          webserver.PermManageLibrary,
	"GET /api/v1/downloads/matches":                              webserver.PermManageLibrary,
	"POST /api/v1/downloads/matches/run":                         webserver.PermManageLibrary,
	"POST /api/v1/downloads/matches/{id}/pick":                   webserver.PermManageLibrary,
	"GET /api/v1/downloaders/{id}/items/{hash}/detail":           webserver.PermManageLibrary,
	"POST /api/v1/downloaders/{id}/torrents":                     webserver.PermManageLibrary,
	"POST /api/v1/downloaders/{id}/bulk":                         webserver.PermManageLibrary, // removing outside torrents also needs PermSettings (checked in the handler)
	"POST /api/v1/downloaders/{id}/share-limits":                 webserver.PermManageLibrary, // a remove action on outside torrents also needs PermSettings
	"POST /api/v1/downloaders/{id}/slow-mode":                    webserver.PermManageLibrary,
	"POST /api/v1/downloaders/{id}/location":                     webserver.PermManageLibrary, // full paths need PermSettings (clientFolder)
	"POST /api/v1/downloaders/{id}/speed-limits":                 webserver.PermManageLibrary,
	"POST /api/v1/downloaders/{id}/items/{hash}/trackers":        webserver.PermManageLibrary,
	"PUT /api/v1/downloaders/{id}/items/{hash}/trackers":         webserver.PermManageLibrary,
	"POST /api/v1/downloaders/{id}/items/{hash}/trackers/remove": webserver.PermManageLibrary,
	"GET /api/v1/downloaders/{id}/items/{hash}/swarm":            webserver.PermManageLibrary,
	"GET /api/v1/downloaders/{id}/items/{hash}/files":            webserver.PermManageLibrary,
	"PUT /api/v1/downloaders/{id}/items/{hash}/files":            webserver.PermManageLibrary,
	"GET /api/v1/needs-decision":                                 webserver.PermManageLibrary,
	"POST /api/v1/needs-decision/{id}/decide":                    webserver.PermManageLibrary,
	"GET /api/v1/settings/downloaders":                           webserver.PermSettings,
	"PUT /api/v1/settings/downloaders":                           webserver.PermSettings,
	"GET /api/v1/health":                                         webserver.PermManageLibrary,
	"GET /api/v1/health/checks":                                  webserver.PermManageLibrary,
	"GET /api/v1/health/disks":                                   webserver.PermManageLibrary,
	"POST /api/v1/health/checks/run":                             webserver.PermManageLibrary,
	"GET /api/v1/health/states":                                  webserver.PermManageLibrary,
	"GET /api/v1/settings/health":                                webserver.PermSettings,
	"PUT /api/v1/settings/health":                                webserver.PermSettings,
	"POST /api/v1/library-folders/{id}/check":                    webserver.PermManageLibrary,

	// Settings.
	"POST /api/v1/library-folders":         webserver.PermSettings,
	"PUT /api/v1/library-folders/{id}":     webserver.PermSettings,
	"DELETE /api/v1/library-folders/{id}":  webserver.PermSettings,
	"PUT /api/v1/settings/release-gate":    webserver.PermSettings,
	"GET /api/v1/settings/trakt-key":       webserver.PermSettings,
	"PUT /api/v1/settings/trakt-key":       webserver.PermSettings,
	"GET /api/v1/settings/trakt-token":     webserver.NeedOwner,
	"PUT /api/v1/settings/trakt-token":     webserver.NeedOwner,
	"GET /api/v1/outside-lists":            webserver.PermSettings,
	"POST /api/v1/outside-lists":           webserver.PermSettings,
	"PUT /api/v1/outside-lists/{id}":       webserver.PermSettings,
	"DELETE /api/v1/outside-lists/{id}":    webserver.PermSettings,
	"POST /api/v1/outside-lists/{id}/sync": webserver.PermSettings,
	"GET /api/v1/backups":                  webserver.PermSettings,
	"POST /api/v1/backups":                 webserver.PermSettings,
	"PUT /api/v1/backups/settings":         webserver.PermSettings,
	"GET /api/v1/backups/{name}":           webserver.NeedOwner, // the whole database, password hashes included
	"DELETE /api/v1/backups/{name}":        webserver.NeedOwner,
	"POST /api/v1/system/key-kit":          webserver.NeedOwner, // the master key itself
	"GET /api/v1/auth/keys":                webserver.PermSettings,
	"POST /api/v1/auth/keys":               webserver.PermSettings,
	"DELETE /api/v1/auth/keys/{id}":        webserver.PermSettings,
	"GET /api/v1/auth/settings":            webserver.PermSettings,

	// People.
	"GET /api/v1/users":                        webserver.NeedUsers,
	"PUT /api/v1/users/{id}":                   webserver.NeedUsers,
	"POST /api/v1/users/{id}/two-factor/reset": webserver.NeedUsers,
	"GET /api/v1/users/{id}/sessions":          webserver.NeedUsers,
	"POST /api/v1/users/{id}/password":         webserver.NeedUsers,
	"POST /api/v1/users/{id}/sessions/end-all": webserver.NeedUsers,
	"DELETE /api/v1/users/{id}/sessions/{sid}": webserver.NeedUsers,
	"GET /api/v1/users/invites":                webserver.NeedUsers,
	"POST /api/v1/users/invites":               webserver.NeedUsers,
	"DELETE /api/v1/users/invites/{id}":        webserver.NeedUsers,

	// Owner only: how people sign in, and putting a backup back.
	"PUT /api/v1/auth/settings": webserver.NeedOwner,
	// Single sign-on setup and unlinking someone else: the owner only.
	"GET /api/v1/auth/sso":               webserver.NeedOwner,
	"PUT /api/v1/auth/sso":               webserver.NeedOwner,
	"POST /api/v1/auth/sso/test":         webserver.NeedOwner,
	"POST /api/v1/users/{id}/sso/unlink": webserver.NeedOwner,
	// Setting up outside sign-in: link the owner's Plex, test an address, import people.
	"POST /api/v1/auth/plex/link/start":          webserver.NeedOwner,
	"POST /api/v1/auth/plex/link/finish":         webserver.NeedOwner,
	"DELETE /api/v1/auth/plex/link":              webserver.NeedOwner,
	"POST /api/v1/auth/media-server/{kind}/test": webserver.NeedOwner,
	"POST /api/v1/auth/import/{kind}/preview":    webserver.NeedOwner,
	"POST /api/v1/auth/import/{kind}":            webserver.NeedOwner,
	// The secrets file and the arr keys it saves are the owner's.
	"POST /api/v1/settings/import-secrets": webserver.NeedOwner,
	"GET /api/v1/migrate/saved-keys":       webserver.NeedOwner,
	"DELETE /api/v1/migrate/saved-keys":    webserver.NeedOwner,
	"POST /api/v1/backups/restore":         webserver.NeedOwner,

	// Parts and controls (3.7.6, 3.7.7): everyone signed in sees which
	// parts are on (the screens hide the others); switching and the
	// controls are for the owner and admins.
	"GET /api/v1/parts":                      webserver.NeedSignedIn,
	"POST /api/v1/parts/{part}/on":           webserver.NeedUsers,
	"POST /api/v1/parts/{part}/off":          webserver.NeedUsers,
	"POST /api/v1/parts/{part}/restart":      webserver.NeedUsers,
	"POST /api/v1/stages/{name}/pause":       webserver.NeedUsers,
	"POST /api/v1/stages/{name}/resume":      webserver.NeedUsers,
	"POST /api/v1/stages/{name}/restart":     webserver.NeedUsers,
	"POST /api/v1/links/{kind}/{id}/restart": webserver.NeedUsers,
	"POST /api/v1/app/restart":               webserver.NeedUsers,
	"POST /api/v1/app/safe-mode":             webserver.NeedUsers,
	"POST /api/v1/app/shutdown":              webserver.NeedUsers,

	// This app (3.5.7, 3.5.9): where things are kept needs settings; the
	// log is for the owner and admins only (it is masked, but it says a
	// lot about the home network).
	"GET /api/v1/app/storage":   webserver.PermSettings,
	"GET /api/v1/app/about":     webserver.PermSettings,
	"GET /api/v1/logs":          webserver.NeedUsers,
	"GET /api/v1/logs/download": webserver.NeedUsers,
	// The support bundle holds the same log, so the same people.
	"GET /api/v1/support/bundle": webserver.NeedUsers,

	// Schedules (3.5.8): staff who run the Library see them and run one
	// now (the Job's part must be on); the intervals are settings.
	"GET /api/v1/app/update":            webserver.PermManageLibrary,
	"POST /api/v1/app/update/check":     webserver.PermSettings,
	"GET /api/v1/schedules":             webserver.PermManageLibrary,
	"POST /api/v1/schedules/{kind}/run": webserver.PermManageLibrary,
	"PUT /api/v1/schedules/{kind}":      webserver.PermSettings,

	// The arr API: one row per method on each prefix;
	// every route behind them reads or changes the Library.
	"GET /sonarr/":    webserver.PermManageLibrary,
	"POST /sonarr/":   webserver.PermManageLibrary,
	"PUT /sonarr/":    webserver.PermManageLibrary,
	"DELETE /sonarr/": webserver.PermManageLibrary,
	"GET /radarr/":    webserver.PermManageLibrary,
	"POST /radarr/":   webserver.PermManageLibrary,
	"PUT /radarr/":    webserver.PermManageLibrary,
	"DELETE /radarr/": webserver.PermManageLibrary,
	// Its settings: the switch, and one key per side.
	"GET /api/v1/settings/old-app-api":                webserver.PermSettings,
	"PUT /api/v1/settings/old-app-api":                webserver.PermSettings,
	"POST /api/v1/settings/old-app-api/keys/{side}":   webserver.PermSettings,
	"DELETE /api/v1/settings/old-app-api/keys/{side}": webserver.PermSettings,

	// The move tool: reading an old app's setup reaches other
	// machines with keys the owner types and keeps what it read, so it is
	// for the owner and admins with a password sign-in and the admin scope.
	"POST /api/v1/migrate/probe":            webserver.NeedUsers,
	"POST /api/v1/migrate/read":             webserver.NeedUsers,
	"GET /api/v1/migrate/snapshots":         webserver.NeedUsers,
	"GET /api/v1/migrate/snapshots/{id}":    webserver.NeedUsers,
	"DELETE /api/v1/migrate/snapshots/{id}": webserver.NeedUsers,
	"POST /api/v1/migrate/plan":             webserver.NeedUsers,
	"POST /api/v1/migrate/dry-run":          webserver.NeedUsers,
	"GET /api/v1/migrate/moves":             webserver.NeedUsers,
	"POST /api/v1/migrate/moves":            webserver.NeedUsers,
	"GET /api/v1/migrate/moves/{id}":        webserver.NeedUsers,
	"GET /api/v1/migrate/moves/{id}/report": webserver.NeedUsers,
	"POST /api/v1/migrate/moves/{id}/undo":  webserver.NeedUsers,

	// Reports: anyone signed in reports a problem and sees their own;
	// fixing and opening again needs manage_library.
	"GET /api/v1/reports":              webserver.NeedSignedIn,
	"POST /api/v1/reports":             webserver.NeedSignedIn,
	"POST /api/v1/reports/{id}/fix":    webserver.PermManageLibrary,
	"POST /api/v1/reports/{id}/reopen": webserver.PermManageLibrary,
}

// Scopes lists the scopes a key may be given.
var Scopes = []string{webserver.ScopeHealthRead, webserver.ScopeRead, webserver.ScopeWrite, webserver.ScopeAdmin, webserver.ScopeControl, webserver.ScopeAll, webserver.ScopeMCP, webserver.ScopeCompat}

// ScopeNotes says in plain words what each scope reaches; the OpenAPI
// spec publishes it.
var ScopeNotes = map[string]string{
	webserver.ScopeHealthRead: "Health states only, for monitoring companions.",
	webserver.ScopeRead:       "Look: the GET routes that hand out no secrets (backups, keys, tokens, sign-in settings and the people list need admin).",
	webserver.ScopeWrite:      "Change: every route that is not a GET, except the admin ones and the controls.",
	webserver.ScopeAdmin:      "People, invites, ask limits, API keys, password and two-factor sign-in, sign-in settings, backups (list, download, restore) and the outside-service key settings: every route only the owner or an admin may use, the key and sign-in routes of My account, and every GET route not marked read. Making or promoting an admin also needs the control scope.",
	webserver.ScopeControl:    "The controls: part switches, stage pause, resume and restart, reconnecting one linked app, whole-app restart, restart in safe mode and shut down.",
	webserver.ScopeAll:        "Read, write and admin together (every route but the controls). Every key made before scopes existed holds this one.",
	webserver.ScopeMCP:        "An AI connection only: the AI tools route and nothing else. The tools act as the key's person, within their permissions.",
	webserver.ScopeCompat:     "The arr API only (/sonarr/ and /radarr/, for Seerr and Overseerr). Only the keys made for it (POST /api/v1/settings/old-app-api/keys/{side}) hold this scope; an API key cannot be made with it.",
}

// Gate checks every signed-in request against RoutePerms. It must sit
// between Middleware and the mux.
func Gate(m *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		who, ok := webserver.From(r.Context())
		if !ok {
			m.ServeHTTP(w, r) // public route; Middleware already decided
			return
		}
		_, pattern := m.Handler(r)
		if pattern == "" {
			m.ServeHTTP(w, r) // no such route: let the mux answer 404 or 405
			return
		}
		need, known := RoutePerms[pattern]
		if !known {
			need = webserver.NeedOwner
		}
		if who.MustTwoFactor && need != webserver.NeedSelf {
			fail(w, http.StatusForbidden, "turn on two-factor sign-in in My account first")
			return
		}
		if scope := RouteScope(pattern); !who.HasScope(scope) {
			fail(w, http.StatusForbidden, "this API key's scope does not cover this route; make a key with the "+scope+" scope")
			return
		}
		if needsRecentLocal(pattern, who) && !RecentLocal(r) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "confirm your local password again before using this route", "code": "reauth"})
			return
		}
		if !who.Has(need) {
			if r.Method == http.MethodGet && !strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/live" && ArrSide(r.URL.Path) == "" {
				http.Redirect(w, r, "/", http.StatusSeeOther)
				return
			}
			fail(w, http.StatusForbidden, "your account may not do this; ask the owner")
			return
		}
		m.ServeHTTP(w, r)
	})
}

func validPerm(p webserver.Perm) bool { return slices.Contains(webserver.AllPerms, p) }

// parsePerms reads a JSON permission list, keeping only known ones.
func parsePerms(s string) []webserver.Perm {
	var raw []webserver.Perm
	_ = json.Unmarshal([]byte(s), &raw)
	out := []webserver.Perm{}
	for _, p := range raw {
		if validPerm(p) && !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out
}

func permsJSON(ps []webserver.Perm) string {
	if ps == nil {
		ps = []webserver.Perm{}
	}
	b, _ := json.Marshal(ps)
	return string(b)
}
