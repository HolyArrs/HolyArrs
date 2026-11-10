package accounts

import (
	"net/http"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// RouteScopes names routes whose scope is not the one the method and
// RoutePerms give them (see RouteScope).
var RouteScopes = map[string]string{
	"GET /api/v1/trash/profiles":                                 webserver.ScopeAdmin,
	"GET /api/v1/downloads/{id}/swarm":                           webserver.ScopeAdmin,
	"GET /api/v1/downloads/{id}/detail":                          webserver.ScopeAdmin,
	"GET /api/v1/downloads/{id}/files":                           webserver.ScopeAdmin,
	"PUT /api/v1/downloads/{id}/files":                           webserver.ScopeAdmin,
	"GET /api/v1/downloaders/{id}/items/{hash}/detail":           webserver.ScopeAdmin,
	"GET /api/v1/downloaders/{id}/items/{hash}/swarm":            webserver.ScopeAdmin,
	"GET /api/v1/downloaders/{id}/items/{hash}/files":            webserver.ScopeAdmin,
	"PUT /api/v1/downloaders/{id}/items/{hash}/files":            webserver.ScopeAdmin,
	"GET /api/v1/downloaders/{id}/items/{hash}/usenet":           webserver.ScopeAdmin,
	"GET /api/v1/downloads/{id}/usenet":                          webserver.ScopeAdmin,
	"GET /api/v1/downloaders/{id}/attempts":                      webserver.ScopeAdmin,
	"GET /api/v1/explore/title-stats":                            webserver.ScopeAdmin,
	"POST /api/v1/downloaders/{id}/attempts/{attemptID}/abandon": webserver.ScopeAdmin,
	"POST /api/v1/auth/oidc/start":                               webserver.ScopeAdmin,
	"POST /api/v1/auth/oidc/finish":                              webserver.ScopeAdmin,
	"POST /api/v1/auth/forward/login":                            webserver.ScopeAdmin,
	"POST /api/v1/people/{id}/movies/review":                     webserver.ScopeRead,
	"GET /api/v1/health/states":                                  webserver.ScopeHealthRead,
	"GET /api/v1/data-packs":                                     webserver.ScopeAdmin,
	"GET /api/v1/data-packs/{id}":                                webserver.ScopeAdmin,
	"GET /api/v1/data-packs/{id}/preview":                        webserver.ScopeAdmin,
	"POST /api/v1/parts/{part}/on":                               webserver.ScopeControl,
	"POST /api/v1/parts/{part}/off":                              webserver.ScopeControl,
	"POST /api/v1/parts/{part}/restart":                          webserver.ScopeControl,
	"POST /api/v1/stages/{name}/pause":                           webserver.ScopeControl,
	"POST /api/v1/stages/{name}/resume":                          webserver.ScopeControl,
	"POST /api/v1/stages/{name}/restart":                         webserver.ScopeControl,
	"POST /api/v1/links/{kind}/{id}/restart":                     webserver.ScopeControl,
	"POST /api/v1/app/restart":                                   webserver.ScopeControl,
	"POST /api/v1/app/safe-mode":                                 webserver.ScopeControl,
	"POST /api/v1/app/shutdown":                                  webserver.ScopeControl,
	"POST /api/v1/mcp":                                           webserver.ScopeMCP,
	"GET /api/v1/mcp":                                            webserver.ScopeMCP,
	"POST /mcp":                                                  webserver.ScopeMCP,
	"GET /mcp":                                                   webserver.ScopeMCP,
	"GET /api/v1/settings/ai":                                    webserver.ScopeAdmin,
	"GET /.well-known/oauth-protected-resource":                  webserver.ScopeAdmin, // public; a key never needs it
	"GET /.well-known/oauth-protected-resource/{path...}":        webserver.ScopeAdmin, // public; a key never needs it
	"GET /.well-known/oauth-authorization-server":                webserver.ScopeAdmin, // public; a key never needs it
	"POST /oauth/register":                                       webserver.ScopeAdmin, // public; a key never needs it
	"GET /oauth/authorize":                                       webserver.ScopeAdmin, // public; a key never needs it
	"POST /oauth/token":                                          webserver.ScopeAdmin, // public; a key never needs it
	"POST /api/v1/auth/ai-connect":                               webserver.ScopeAdmin, // browser session only
	"GET /sonarr/":                                               webserver.ScopeCompat,
	"POST /sonarr/":                                              webserver.ScopeCompat,
	"PUT /sonarr/":                                               webserver.ScopeCompat,
	"DELETE /sonarr/":                                            webserver.ScopeCompat,
	"GET /radarr/":                                               webserver.ScopeCompat,
	"POST /radarr/":                                              webserver.ScopeCompat,
	"PUT /radarr/":                                               webserver.ScopeCompat,
	"DELETE /radarr/":                                            webserver.ScopeCompat,
	"GET /api/v1/settings/old-app-api":                           webserver.ScopeAdmin,
	"POST /api/v1/settings/old-app-api/keys/{side}":              webserver.ScopeAdmin,
	"DELETE /api/v1/settings/old-app-api/keys/{side}":            webserver.ScopeAdmin,

	// My account's sign-in and the keys themselves.
	"GET /api/v1/auth/keys":                     webserver.ScopeAdmin,
	"POST /api/v1/auth/keys":                    webserver.ScopeAdmin,
	"POST /api/v1/auth/reauth":                  webserver.ScopeAdmin,
	"POST /api/v1/auth/setup/done":              webserver.ScopeAdmin,
	"POST /api/v1/auth/oidc/link":               webserver.ScopeAdmin,
	"GET /api/v1/auth/sso":                      webserver.ScopeAdmin,
	"PUT /api/v1/auth/sso":                      webserver.ScopeAdmin,
	"POST /api/v1/auth/sso/test":                webserver.ScopeAdmin,
	"POST /api/v1/auth/sso/unlink":              webserver.ScopeAdmin,
	"POST /api/v1/auth/plex/my-link/start":      webserver.ScopeAdmin,
	"POST /api/v1/auth/plex/my-link/finish":     webserver.ScopeAdmin,
	"DELETE /api/v1/auth/plex/my-link":          webserver.ScopeAdmin,
	"POST /api/v1/users/{id}/sso/unlink":        webserver.ScopeAdmin,
	"POST /api/v1/auth/oidc/link-finish":        webserver.ScopeAdmin,
	"POST /api/v1/auth/forward/link":            webserver.ScopeAdmin,
	"DELETE /api/v1/auth/keys/{id}":             webserver.ScopeAdmin,
	"POST /api/v1/auth/password":                webserver.ScopeAdmin,
	"POST /api/v1/auth/two-factor/setup":        webserver.ScopeAdmin,
	"POST /api/v1/auth/two-factor/enable":       webserver.ScopeAdmin,
	"POST /api/v1/auth/two-factor/disable":      webserver.ScopeAdmin,
	"POST /api/v1/auth/two-factor/backup-codes": webserver.ScopeAdmin,
	"GET /api/v1/auth/sessions":                 webserver.ScopeAdmin,
	"DELETE /api/v1/auth/sessions":              webserver.ScopeAdmin,
	"DELETE /api/v1/auth/sessions/{sid}":        webserver.ScopeAdmin,

	// GET routes that hand out the database, keys, tokens, sign-in
	// settings or the people list: never the read scope.
	"GET /api/v1/backups":                             webserver.ScopeAdmin,
	"GET /api/v1/backups/{name}":                      webserver.ScopeAdmin, // the whole database, password hashes included
	"GET /api/v1/auth/settings":                       webserver.ScopeAdmin,
	"GET /api/v1/settings/trakt-key":                  webserver.ScopeAdmin,
	"GET /api/v1/settings/trakt-token":                webserver.ScopeAdmin,
	"GET /api/v1/users":                               webserver.ScopeAdmin,
	"GET /api/v1/users/invites":                       webserver.ScopeAdmin,
	"GET /api/v1/app/storage":                         webserver.ScopeAdmin,
	"GET /api/v1/logs":                                webserver.ScopeAdmin,
	"GET /api/v1/logs/download":                       webserver.ScopeAdmin,
	"GET /api/v1/support/bundle":                      webserver.ScopeAdmin,
	"GET /api/v1/users/{id}/sessions":                 webserver.ScopeAdmin,
	"GET /api/v1/ask-limits":                          webserver.ScopeAdmin,
	"GET /api/v1/auth/state":                          webserver.ScopeAdmin, // public; a key never needs it
	"GET /api/v1/auth/invites/{token}":                webserver.ScopeAdmin, // public; a key never needs it
	"GET /api/v1/auth/setup/discover":                 webserver.ScopeAdmin, // public, first run only; a key never needs it
	"GET /api/v1/auth/oidc/callback":                  webserver.ScopeAdmin, // public; a key never needs it
	"GET /api/v1/public/signin-art":                   webserver.ScopeAdmin, // public; a key never needs it
	"GET /api/v1/public/signin-pic/t/p/{size}/{file}": webserver.ScopeAdmin, // public; a key never needs it

	// The move tool: what old apps hold (people, emails, links) is admin.
	"POST /api/v1/migrate/probe":            webserver.ScopeAdmin,
	"POST /api/v1/migrate/read":             webserver.ScopeAdmin,
	"GET /api/v1/migrate/snapshots":         webserver.ScopeAdmin,
	"GET /api/v1/migrate/snapshots/{id}":    webserver.ScopeAdmin,
	"DELETE /api/v1/migrate/snapshots/{id}": webserver.ScopeAdmin,
	"GET /api/v1/migrate/saved-keys":        webserver.ScopeAdmin,
	"DELETE /api/v1/migrate/saved-keys":     webserver.ScopeAdmin,
	"POST /api/v1/settings/import-secrets":  webserver.ScopeAdmin,
	"PUT /api/v1/settings/import-folders":   webserver.ScopeAdmin, // widens what import by hand may read
	"POST /api/v1/migrate/plan":             webserver.ScopeAdmin,
	"POST /api/v1/migrate/dry-run":          webserver.ScopeAdmin,
	"GET /api/v1/migrate/moves":             webserver.ScopeAdmin,
	"POST /api/v1/migrate/moves":            webserver.ScopeAdmin,
	"GET /api/v1/migrate/moves/{id}":        webserver.ScopeAdmin,
	"GET /api/v1/migrate/moves/{id}/report": webserver.ScopeAdmin,
	"POST /api/v1/migrate/moves/{id}/undo":  webserver.ScopeAdmin,
}

// ReadRoutes are the GET routes a read key may use. A GET route listed
// neither here nor in RouteScopes needs the admin scope (fail closed), and
// TestEveryGETHasAnExplicitScope fails until someone decides. Secrets in
// these answers are masked.
var ReadRoutes = map[string]bool{}

// RouteScope is the scope an API key needs for a route pattern: the
// RouteScopes entry, else admin for a route only the owner or admins may
// use, else read for a GET listed in ReadRoutes, admin for any other GET,
// and write for every other method.
func RouteScope(pattern string) string {
	if sc := RouteScopes[pattern]; sc != "" {
		return sc
	}
	if need, ok := RoutePerms[pattern]; !ok || need == webserver.NeedUsers || need == webserver.NeedOwner {
		return webserver.ScopeAdmin
	}
	method, _, _ := strings.Cut(pattern, " ")
	if method == http.MethodGet || method == http.MethodHead {
		if ReadRoutes[pattern] {
			return webserver.ScopeRead
		}
		return webserver.ScopeAdmin // no decision yet: fail closed
	}
	return webserver.ScopeWrite
}

func init() {
	for _, r := range []string{
		"GET /api/v1/titles/{id}/copies",
		"GET /api/v1/people/search",
		"GET /api/v1/people/popular",
		"GET /api/v1/people/shelves",
		"GET /api/v1/people/shelves/{id}",
		"GET /api/v1/people/{id}",
		"GET /api/v1/wiki/{kind}/{id}",
		"GET /api/v1/titles/{id}/people",
		"GET /api/v1/collections",
		"GET /api/v1/collections/{id}",
		"GET /api/v1/library/copies",
		"GET /api/v1/reports",
		// The Allow page reads what is asking; allowing it needs a fresh password.
		"GET /api/v1/auth/ai-connect",

		"GET /", "GET /login", "GET /live",
		"GET /api/v1/albums/{id}", "GET /api/v1/albums/{id}/tracks", "GET /api/v1/albums/{id}/cover", "GET /api/v1/alerts",
		"GET /api/v1/artists/lookup", "GET /api/v1/artwork/{name}", "GET /api/v1/artwork/tmdb/{size}/{file}", "GET /api/v1/artwork/img/{kind}/{id}/{file}", "GET /api/v1/asks",
		"GET /api/v1/asks/limit", "GET /api/v1/asks/{id}", "GET /api/v1/asks/{id}/parts", "GET /api/v1/auth/me", "GET /api/v1/auth/sso/links", "GET /api/v1/auth/plex/my-link",
		"GET /api/v1/calendar", "GET /api/v1/downloaders", "GET /api/v1/tags",
		"GET /api/v1/downloads", "GET /api/v1/downloads/view", "GET /api/v1/downloads/matches", "GET /api/v1/downloads/clients", "GET /api/v1/events",
		"GET /api/v1/health", "GET /api/v1/health/checks", "GET /api/v1/health/disks", "GET /api/v1/app/about",
		"GET /api/v1/held", "GET /api/v1/holding", "GET /api/v1/imports", "GET /api/v1/jobs", "GET /api/v1/jobs/{id}", "GET /api/v1/jobs/active",
		"GET /api/v1/library-folders", "GET /api/v1/home", "GET /api/v1/media-servers", "GET /api/v1/messages",
		"GET /api/v1/metadata/search", "GET /api/v1/metadata/health", "GET /api/v1/metadata/{kind}/{tmdb_id}", "GET /api/v1/metadata/{kind}/{tmdb_id}/more-from", "GET /api/v1/metadata/{kind}/{tmdb_id}/reviews", "GET /api/v1/metadata/{kind}/{tmdb_id}/anime", "GET /api/v1/extras/{kind}/{id}", "GET /api/v1/metadata/show/{tmdb_id}/seasons/{number}", "GET /api/v1/trailer/{kind}/{tmdb_id}", "GET /api/v1/trailer/batch", "GET /api/v1/trailer/stream/{id}/manifest.m3u8", "GET /api/v1/trailer/stream/{id}/seg/{n}", "GET /api/v1/settings/ui", "GET /api/v1/discover", "GET /api/v1/watch-services", "GET /api/v1/explore/head", "GET /api/v1/explore/tail", "GET /api/v1/explore/top", "GET /api/v1/explore/service-top", "GET /api/v1/explore/genre", "GET /api/v1/explore/genre-row", "GET /api/v1/explore/new", "GET /api/v1/explore/new/services", "GET /api/v1/explore/new/service", "GET /api/v1/explore/new/music", "GET /api/v1/metadata/album/{mbid}/follow", "GET /api/v1/explore/browse", "GET /api/v1/explore/browse/section", "GET /api/v1/explore/browse/category", "GET /api/v1/explore/strip", "GET /api/v1/music-trending", "GET /api/v1/music-trending/art/{kind}/{mbid}", "GET /api/v1/music/genre/{slug}", "GET /api/v1/music/browse/{kind}/{slug}", "GET /api/v1/music/browse-tiles", "GET /api/v1/music/genre-art/{slug}", "GET /api/v1/music/artists/{mbid}", "GET /api/v1/music/albums/{mbid}", "GET /api/v1/explore/music", "GET /api/v1/explore/music/rising", "GET /api/v1/my/alerts", "GET /api/v1/my/plex-watchlist", "GET /api/v1/asked-by", "GET /api/v1/my/profile", "GET /api/v1/my/follows", "GET /api/v1/titles/{id}/follow", "GET /api/v1/metadata/show/{tmdb_id}/follow", "GET /api/v1/metadata/movie/{tmdb_id}/follow", "GET /api/v1/needs-decision",
		"GET /api/v1/never-add", "GET /api/v1/openapi.json", "GET /api/v1/outside-lists",
		"GET /api/v1/parts", "GET /api/v1/pick-rules", "GET /api/v1/pick-rules/{id}", "GET /api/v1/own-traits", "GET /api/v1/rename", "GET /api/v1/searches",
		"GET /api/v1/settings/alerts", "GET /api/v1/settings/url-base", "GET /api/v1/settings/asks", "GET /api/v1/settings/downloaders",
		"GET /api/v1/settings/files", "GET /api/v1/settings/import-folders", "GET /api/v1/settings/health", "GET /api/v1/settings/music",
		"GET /api/v1/settings/naming", "GET /api/v1/settings/release-gate", "GET /api/v1/settings/search", "GET /api/v1/bridges",
		"GET /api/v1/settings/shows", "GET /api/v1/settings/status", "GET /api/v1/settings/subtitles", "GET /api/v1/settings/title-defaults", "GET /api/v1/shows/lookup",
		"GET /api/v1/sources", "GET /api/v1/sources/{id}/stats", "GET /api/v1/subtitle-sites",
		"GET /api/v1/subtitle-sites/kinds", "GET /api/v1/subtitles/languages", "GET /api/v1/subtitles/missing",
		"GET /api/v1/titles", "GET /api/v1/titles/owned", "GET /api/v1/library/search", "GET /api/v1/search/suggest", "GET /api/v1/search", "GET /api/v1/titles/lookup", "GET /api/v1/titles/{id}",
		"GET /api/v1/titles/{id}/albums", "GET /api/v1/titles/{id}/episodes", "GET /api/v1/titles/{id}/releases",
		"GET /api/v1/titles/{id}/rename", "GET /api/v1/titles/{id}/seasons", "GET /api/v1/titles/{id}/subtitles", "GET /api/v1/titles/{id}/search-outcome",
		"GET /api/v1/turned-down", "GET /api/v1/users/{id}/avatar", "GET /api/v1/schedules", "GET /api/v1/app/update",
	} {
		ReadRoutes[r] = true
	}
}
