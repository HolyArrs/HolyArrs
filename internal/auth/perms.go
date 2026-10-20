package auth

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
)

// Roles.
const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"
)

// Perm is one thing a person may do. The owner may do everything.
type Perm string

// Permissions a user can be given.
const (
	PermAsk           Perm = "ask"            // ask for titles (Wishlist)
	PermAutoApprove   Perm = "auto_approve"   // their asks are approved without waiting
	PermApprove       Perm = "approve"        // approve or turn down other people's asks
	PermManageLibrary Perm = "manage_library" // add, edit, search and download titles
	PermSettings      Perm = "settings"       // change settings
)

// AllPerms lists the permissions in the order screens show them.
var AllPerms = []Perm{PermAsk, PermAutoApprove, PermApprove, PermManageLibrary, PermSettings}

// Route needs besides the permissions above.
const (
	needSignedIn Perm = "signed_in" // anyone signed in
	// NeedPublic marks a route that needs no sign-in at all because it
	// checks a key of its own (the iCal feed). Middleware lets it through.
	NeedPublic Perm = "public"
	needSelf   Perm = "self"  // anyone signed in, even before they turn on a two-factor sign-in they must have
	needUsers  Perm = "users" // the owner or an admin
	needOwner  Perm = "owner" // the owner only
)

// DefaultPerms are what a new user of a role starts with.
func DefaultPerms(role string) []Perm {
	switch role {
	case RoleAdmin:
		return []Perm{PermAsk, PermAutoApprove, PermApprove, PermManageLibrary}
	case RoleMember:
		return []Perm{PermAsk}
	}
	return slices.Clone(AllPerms)
}

func validPerm(p Perm) bool { return slices.Contains(AllPerms, p) }

// parsePerms reads a JSON permission list, keeping only known ones.
func parsePerms(s string) []Perm {
	var raw []Perm
	_ = json.Unmarshal([]byte(s), &raw)
	out := []Perm{}
	for _, p := range raw {
		if validPerm(p) && !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out
}

func permsJSON(ps []Perm) string {
	if ps == nil {
		ps = []Perm{}
	}
	b, _ := json.Marshal(ps)
	return string(b)
}

// Has reports whether the person may do p.
func (w Who) Has(p Perm) bool {
	switch {
	case w.Role == RoleOwner:
		return true
	case p == needSignedIn || p == needSelf || p == NeedPublic:
		return true
	case p == needUsers:
		return w.Role == RoleAdmin
	case p == needOwner:
		return false
	}
	return slices.Contains(w.Perms, p)
}

// Effective is the full list of permissions the person holds.
func (w Who) Effective() []Perm {
	out := []Perm{}
	for _, p := range AllPerms {
		if w.Has(p) {
			out = append(out, p)
		}
	}
	return out
}

// RoutePerms says what each route needs. Routes missing here are for the
// owner only, so a new route is closed until someone decides who may use it.
var RoutePerms = map[string]Perm{
	"GET /api/v1/data-packs":                            needOwner,
	"GET /api/v1/data-packs/{id}":                       needOwner,
	"GET /api/v1/data-packs/{id}/preview":               needOwner,
	"POST /api/v1/data-packs/validate":                  needOwner,
	"POST /api/v1/data-packs":                           needOwner,
	"POST /api/v1/data-packs/{id}/apply":                needOwner,
	"GET /api/v1/titles/{id}/copies":                    PermManageLibrary,
	"POST /api/v1/titles/{id}/versions/{vid}/adopt":     PermManageLibrary,
	"GET /api/v1/people/search":                         needSignedIn,
	"GET /api/v1/people/{id}":                           needSignedIn,
	"POST /api/v1/people/{id}/movies/review":            needSignedIn,
	"GET /api/v1/titles/{id}/people":                    needSignedIn,
	"PUT /api/v1/titles/{id}/files/{fid}/choices":       PermManageLibrary,
	"POST /api/v1/titles/{id}/files/{fid}/keep-and-add": PermManageLibrary,
	"PUT /api/v1/downloads/{id}/edition":                PermManageLibrary,
	"GET /api/v1/collections":                           needSignedIn,
	"GET /api/v1/collections/{id}":                      needSignedIn,
	"POST /api/v1/collections/{id}/remove/preview":      PermManageLibrary,
	"POST /api/v1/collections/{id}/remove":              PermManageLibrary,
	"GET /api/v1/library/copies":                        PermManageLibrary,
	"POST /api/v1/library/copies/search/preview":        PermManageLibrary,
	"POST /api/v1/library/copies/search":                PermManageLibrary,

	// The app's pages and files; the pages themselves ask the API.
	"GET /":      needSelf,
	"GET /login": needSelf,

	// Optional outside sign-in. These routes validate their own bounded flow.
	"POST /api/v1/auth/oidc/start":    NeedPublic,
	"GET /api/v1/auth/oidc/callback":  NeedPublic,
	"POST /api/v1/auth/oidc/finish":   NeedPublic,
	"POST /api/v1/auth/forward/login": NeedPublic,

	// My account.
	"GET /api/v1/auth/me":                       needSelf,
	"POST /api/v1/auth/logout":                  needSelf,
	"POST /api/v1/auth/reauth":                  needSelf,
	"POST /api/v1/auth/oidc/link":               needSelf,
	"POST /api/v1/auth/oidc/link-finish":        needSelf,
	"POST /api/v1/auth/forward/link":            needSelf,
	"POST /api/v1/auth/password":                needSelf,
	"POST /api/v1/auth/two-factor/setup":        needSelf,
	"POST /api/v1/auth/two-factor/enable":       needSelf,
	"POST /api/v1/auth/two-factor/disable":      needSelf,
	"POST /api/v1/auth/two-factor/backup-codes": needSelf,
	"GET /api/v1/auth/sessions":                 needSelf,
	"DELETE /api/v1/auth/sessions":              needSelf,
	"DELETE /api/v1/auth/sessions/{sid}":        needSelf,

	// Looking at the Library and searching for titles.
	"GET /api/v1/titles":                                   needSignedIn,
	"GET /api/v1/library-summary":                          needSignedIn,
	"GET /api/v1/home":                                     needSignedIn,
	"GET /api/v1/titles/{id}":                              needSignedIn,
	"GET /api/v1/artwork/{name}":                           needSignedIn,
	"GET /api/v1/artwork/tmdb/{size}/{file}":               needSignedIn,
	"GET /api/v1/metadata/search":                          needSignedIn,
	"GET /api/v1/metadata/{kind}/{tmdb_id}":                needSignedIn,
	"GET /api/v1/metadata/{kind}/{tmdb_id}/more-from":      needSignedIn,
	"GET /api/v1/metadata/{kind}/{tmdb_id}/reviews":        needSignedIn,
	"GET /api/v1/metadata/show/{tmdb_id}/seasons/{number}": needSignedIn,
	"GET /api/v1/trailer/batch":                            needSignedIn,
	"GET /api/v1/trailer/{kind}/{tmdb_id}":                 needSignedIn,
	"GET /api/v1/trailer/stream/{id}/manifest.m3u8":        needSignedIn,
	"GET /api/v1/trailer/stream/{id}/seg/{n}":              needSignedIn,
	"GET /api/v1/settings/ui":                              needSignedIn,
	"PUT /api/v1/settings/ui":                              PermSettings,
	"GET /api/v1/discover":                                 needSignedIn,
	"GET /api/v1/watch-services":                           needSignedIn,
	"GET /api/v1/top10":                                    needSignedIn,
	"GET /api/v1/explore/head":                             needSignedIn,
	"GET /api/v1/explore/tail":                             needSignedIn,
	"GET /api/v1/explore/top":                              needSignedIn,
	"GET /api/v1/explore/service-top":                      needSignedIn,
	"GET /api/v1/explore/genre":                            needSignedIn,
	"GET /api/v1/explore/genre-row":                        needSignedIn,
	"POST /api/v1/explore/counts":                          needSignedIn,
	"GET /api/v1/explore/title-stats":                      needOwner,
	"GET /api/v1/music-trending":                           needSignedIn,
	"GET /api/v1/explore/music":                            needSignedIn,
	"GET /api/v1/music-trending/art/{kind}/{mbid}":         needSignedIn,
	"GET /api/v1/metadata/health":                          needSignedIn,
	"GET /api/v1/titles/lookup":                            needSignedIn,
	"GET /api/v1/shows/lookup":                             needSignedIn,
	"GET /api/v1/titles/{id}/seasons":                      needSignedIn,
	"GET /api/v1/titles/{id}/episodes":                     needSignedIn,
	"GET /api/v1/artists/lookup":                           needSignedIn,
	"GET /api/v1/titles/{id}/albums":                       needSignedIn,
	"GET /api/v1/albums/{id}":                              needSignedIn,
	"GET /api/v1/albums/{id}/tracks":                       needSignedIn,
	"GET /api/v1/albums/{id}/cover":                        needSignedIn,
	"GET /api/v1/openapi.json":                             needSignedIn,

	// The Wishlist: everyone sees their own asks and messages; asking needs
	// ask, deciding needs approve.
	"GET /api/v1/asks":                          needSignedIn,
	"GET /api/v1/asks/{id}":                     needSignedIn,
	"POST /api/v1/asks":                         PermAsk,
	"GET /api/v1/asks/limit":                    PermAsk,
	"DELETE /api/v1/asks/{id}":                  PermAsk,
	"POST /api/v1/asks/{id}/approve":            PermApprove,
	"POST /api/v1/asks/{id}/turn-down":          PermApprove,
	"GET /api/v1/messages":                      needSignedIn,
	"POST /api/v1/messages/read":                needSignedIn,
	"GET /api/v1/my/alerts":                     needSignedIn,
	"PUT /api/v1/my/alerts":                     needSignedIn,
	"GET /api/v1/asked-by":                      needSignedIn,
	"GET /api/v1/my/ask-privacy":                needSignedIn,
	"GET /api/v1/my/follows":                    needSignedIn,
	"GET /api/v1/titles/{id}/follow":            needSignedIn,
	"PUT /api/v1/titles/{id}/follow":            needSignedIn,
	"DELETE /api/v1/titles/{id}/follow":         needSignedIn,
	"PUT /api/v1/my/ask-privacy":                needSignedIn,
	"GET /api/v1/ask-limits":                    needUsers,
	"PUT /api/v1/ask-limits/{id}":               needUsers,
	"GET /api/v1/settings/asks":                 PermSettings,
	"PUT /api/v1/settings/asks":                 PermSettings,
	"GET /api/v1/settings/alerts":               PermSettings,
	"GET /api/v1/subtitles/languages":           PermManageLibrary,
	"GET /api/v1/settings/subtitles":            PermSettings,
	"PUT /api/v1/settings/subtitles":            PermSettings,
	"GET /api/v1/titles/{id}/subtitles":         PermManageLibrary,
	"PUT /api/v1/titles/{id}/subtitle-wish":     PermManageLibrary,
	"POST /api/v1/subtitles/search":             PermManageLibrary,
	"GET /api/v1/subtitles/missing":             PermManageLibrary,
	"POST /api/v1/subtitles/missing/search":     PermManageLibrary,
	"POST /api/v1/files/{id}/subtitles/choices": PermManageLibrary,
	"POST /api/v1/files/{id}/subtitles/choose":  PermManageLibrary,
	"POST /api/v1/subtitles/{id}/sync":          PermManageLibrary,
	"POST /api/v1/subtitles/{id}/turn-down":     PermManageLibrary,
	"GET /api/v1/subtitle-sites":                PermManageLibrary,
	"GET /api/v1/subtitle-sites/kinds":          PermManageLibrary,
	"POST /api/v1/subtitle-sites":               PermSettings,
	"PUT /api/v1/subtitle-sites/{id}":           PermSettings,
	"DELETE /api/v1/subtitle-sites/{id}":        PermSettings,
	"POST /api/v1/subtitle-sites/{id}/test":     PermSettings,
	"PUT /api/v1/settings/alerts":               PermSettings,
	"GET /api/v1/alerts":                        PermSettings,
	"POST /api/v1/alerts":                       PermSettings,
	"PUT /api/v1/alerts/{id}":                   PermSettings,
	"DELETE /api/v1/alerts/{id}":                PermSettings,
	"POST /api/v1/alerts/{id}/test":             PermSettings,
	"GET /api/v1/media-servers":                 PermSettings,
	"POST /api/v1/media-servers":                PermSettings,
	"PUT /api/v1/media-servers/{id}":            PermSettings,
	"DELETE /api/v1/media-servers/{id}":         PermSettings,
	"POST /api/v1/media-servers/{id}/test":      PermSettings,

	// Running the Library.
	"POST /api/v1/titles":           PermManageLibrary,
	"PUT /api/v1/titles/{id}":       PermManageLibrary,
	"POST /api/v1/titles/{id}/move": PermManageLibrary,
	// Shows (stage 2.2).
	"POST /api/v1/shows":                       PermManageLibrary,
	"PUT /api/v1/titles/{id}/seasons/{number}": PermManageLibrary,
	"PUT /api/v1/episodes/{id}":                PermManageLibrary,
	"PUT /api/v1/settings/shows":               PermSettings,
	// Music (stage 2.3).
	"POST /api/v1/artists":           PermManageLibrary,
	"PUT /api/v1/titles/{id}/artist": PermManageLibrary,
	"PUT /api/v1/albums/{id}":        PermManageLibrary,
	"GET /api/v1/settings/music":     PermManageLibrary,
	"PUT /api/v1/settings/music":     PermSettings,
	// Organise (stage 5.1).
	"GET /api/v1/titles/{id}/rename":            PermManageLibrary,
	"POST /api/v1/titles/{id}/rename":           PermManageLibrary,
	"GET /api/v1/rename":                        PermManageLibrary,
	"POST /api/v1/rename":                       PermManageLibrary,
	"GET /api/v1/jobs/{id}":                     PermManageLibrary,
	"GET /api/v1/settings/naming":               PermManageLibrary,
	"PUT /api/v1/settings/naming":               PermSettings,
	"POST /api/v1/settings/naming/example":      PermManageLibrary,
	"GET /api/v1/settings/files":                PermManageLibrary,
	"PUT /api/v1/settings/files":                PermSettings,
	"POST /api/v1/titles/{id}/delete":           PermManageLibrary,
	"POST /api/v1/holding/clean":                PermManageLibrary,
	"POST /api/v1/library-folders/{id}/scan":    PermManageLibrary,
	"POST /api/v1/library-folders/{id}/adopt":   PermManageLibrary,
	"POST /api/v1/library/rescan":               PermManageLibrary,
	"POST /api/v1/titles/{id}/versions":         PermManageLibrary,
	"PUT /api/v1/titles/{id}/versions/{vid}":    PermManageLibrary,
	"DELETE /api/v1/titles/{id}/versions/{vid}": PermManageLibrary,
	"POST /api/v1/titles/{id}/refresh":          PermManageLibrary,
	"POST /api/v1/titles/refresh":               PermManageLibrary,
	"POST /api/v1/titles/{id}/search":           PermManageLibrary,
	"GET /api/v1/titles/{id}/releases":          PermManageLibrary,
	// Searching (stage 3.1).
	"POST /api/v1/releases/{id}/choose":    PermManageLibrary,
	"POST /api/v1/releases/{id}/turn-down": PermManageLibrary,
	"GET /api/v1/held":                     PermManageLibrary,
	"DELETE /api/v1/held":                  PermManageLibrary,
	"DELETE /api/v1/held/{id}":             PermManageLibrary,
	"GET /api/v1/turned-down":              PermManageLibrary,
	"DELETE /api/v1/turned-down":           PermManageLibrary,
	"DELETE /api/v1/turned-down/{id}":      PermManageLibrary,
	"POST /api/v1/search/missing":          PermManageLibrary,
	"POST /api/v1/search/better":           PermManageLibrary,
	"POST /api/v1/search/feed-check":       PermManageLibrary,
	"GET /api/v1/searches":                 PermManageLibrary,
	"GET /api/v1/settings/search":          PermSettings,
	"PUT /api/v1/settings/search":          PermSettings,
	"GET /api/v1/calendar":                 PermManageLibrary,
	"GET /api/v1/calendar/feed-key":        PermManageLibrary,
	"POST /api/v1/calendar/feed-key":       PermSettings,
	"DELETE /api/v1/calendar/feed-key":     PermSettings,
	// The iCal feed is public (phones cannot sign in) and checks its own key.
	"GET /api/v1/calendar/feed.ics":                              NeedPublic,
	"GET /api/v1/library-folders":                                PermManageLibrary,
	"GET /api/v1/pick-rules":                                     PermManageLibrary,
	"GET /api/v1/settings/status":                                PermSettings,
	"GET /api/v1/settings/title-defaults":                        PermManageLibrary,
	"PUT /api/v1/settings/title-defaults":                        PermSettings,
	"GET /api/v1/pick-rules/{id}":                                PermManageLibrary,
	"POST /api/v1/pick-rules":                                    PermSettings,
	"PUT /api/v1/pick-rules/{id}":                                PermSettings,
	"DELETE /api/v1/pick-rules/{id}":                             PermSettings,
	"GET /api/v1/never-add":                                      PermManageLibrary,
	"POST /api/v1/never-add":                                     PermManageLibrary,
	"DELETE /api/v1/never-add/{id}":                              PermManageLibrary,
	"GET /api/v1/settings/release-gate":                          PermManageLibrary,
	"GET /api/v1/settings/shows":                                 PermManageLibrary,
	"GET /api/v1/downloads":                                      PermManageLibrary,
	"GET /api/v1/imports":                                        PermManageLibrary,
	"GET /api/v1/events":                                         PermManageLibrary,
	"GET /api/v1/jobs":                                           PermManageLibrary,
	"GET /api/v1/sources":                                        PermManageLibrary,
	"GET /api/v1/sources/{id}/stats":                             PermManageLibrary,
	"POST /api/v1/sources":                                       PermSettings,
	"PUT /api/v1/sources/{id}":                                   PermSettings,
	"DELETE /api/v1/sources/{id}":                                PermSettings,
	"POST /api/v1/sources/{id}/test":                             PermSettings,
	"POST /api/v1/sources/{id}/pause":                            PermSettings,
	"POST /api/v1/sources/{id}/resume":                           PermSettings,
	"GET /api/v1/downloaders":                                    PermManageLibrary,
	"POST /api/v1/downloaders":                                   PermSettings,
	"PUT /api/v1/downloaders/{id}":                               PermSettings,
	"DELETE /api/v1/downloaders/{id}":                            PermSettings,
	"POST /api/v1/downloaders/{id}/test":                         PermSettings,
	"POST /api/v1/downloaders/{id}/pause":                        PermSettings,
	"POST /api/v1/downloaders/{id}/resume":                       PermSettings,
	"GET /api/v1/downloaders/{id}/attempts":                      PermSettings,
	"POST /api/v1/downloaders/{id}/attempts/{attemptID}/abandon": PermSettings,
	"POST /api/v1/downloads/{id}/mark-failed":                    PermManageLibrary,
	"GET /api/v1/downloads/view":                                 PermManageLibrary,
	"POST /api/v1/downloads/{id}/remove":                         PermManageLibrary,
	"GET /api/v1/needs-decision":                                 PermManageLibrary,
	"POST /api/v1/needs-decision/{id}/decide":                    PermManageLibrary,
	"GET /api/v1/settings/downloaders":                           PermSettings,
	"PUT /api/v1/settings/downloaders":                           PermSettings,
	"GET /api/v1/health":                                         PermManageLibrary,
	"GET /api/v1/health/checks":                                  PermManageLibrary,
	"POST /api/v1/health/checks/run":                             PermManageLibrary,
	"GET /api/v1/health/states":                                  PermManageLibrary,
	"GET /api/v1/settings/health":                                PermSettings,
	"PUT /api/v1/settings/health":                                PermSettings,
	"GET /live":                                                  PermManageLibrary,
	"POST /api/v1/library-folders/{id}/check":                    PermManageLibrary,

	// Settings.
	"POST /api/v1/library-folders":         PermSettings,
	"PUT /api/v1/library-folders/{id}":     PermSettings,
	"DELETE /api/v1/library-folders/{id}":  PermSettings,
	"PUT /api/v1/settings/release-gate":    PermSettings,
	"GET /api/v1/settings/trakt-key":       PermSettings,
	"PUT /api/v1/settings/trakt-key":       PermSettings,
	"GET /api/v1/settings/trakt-token":     needOwner,
	"PUT /api/v1/settings/trakt-token":     needOwner,
	"GET /api/v1/settings/tvdb-key":        PermSettings,
	"PUT /api/v1/settings/tvdb-key":        PermSettings,
	"GET /api/v1/settings/theaudiodb-key":  PermSettings,
	"PUT /api/v1/settings/theaudiodb-key":  PermSettings,
	"GET /api/v1/outside-lists":            PermSettings,
	"POST /api/v1/outside-lists":           PermSettings,
	"PUT /api/v1/outside-lists/{id}":       PermSettings,
	"DELETE /api/v1/outside-lists/{id}":    PermSettings,
	"POST /api/v1/outside-lists/{id}/sync": PermSettings,
	"GET /api/v1/backups":                  PermSettings,
	"POST /api/v1/backups":                 PermSettings,
	"PUT /api/v1/backups/settings":         PermSettings,
	"GET /api/v1/backups/{name}":           PermSettings,
	"DELETE /api/v1/backups/{name}":        PermSettings,
	"GET /api/v1/auth/keys":                PermSettings,
	"POST /api/v1/auth/keys":               PermSettings,
	"DELETE /api/v1/auth/keys/{id}":        PermSettings,
	"GET /api/v1/auth/settings":            PermSettings,

	// People.
	"GET /api/v1/users":                        needUsers,
	"PUT /api/v1/users/{id}":                   needUsers,
	"POST /api/v1/users/{id}/two-factor/reset": needUsers,
	"GET /api/v1/users/{id}/sessions":          needUsers,
	"POST /api/v1/users/{id}/sessions/end-all": needUsers,
	"DELETE /api/v1/users/{id}/sessions/{sid}": needUsers,
	"GET /api/v1/users/invites":                needUsers,
	"POST /api/v1/users/invites":               needUsers,
	"DELETE /api/v1/users/invites/{id}":        needUsers,

	// Owner only: how people sign in, and putting a backup back.
	"PUT /api/v1/auth/settings": needOwner,
	// The secrets file and the old-app keys it saves are the owner's.
	"POST /api/v1/settings/import-secrets": needOwner,
	"GET /api/v1/migrate/saved-keys":       needOwner,
	"DELETE /api/v1/migrate/saved-keys":    needOwner,
	"POST /api/v1/backups/restore":         needOwner,

	// Parts and controls (3.7.6, 3.7.7): everyone signed in sees which
	// parts are on (the screens hide the others); switching and the
	// controls are for the owner and admins.
	"GET /api/v1/parts":                      needSignedIn,
	"POST /api/v1/parts/{part}/on":           needUsers,
	"POST /api/v1/parts/{part}/off":          needUsers,
	"POST /api/v1/parts/{part}/restart":      needUsers,
	"POST /api/v1/stages/{name}/pause":       needUsers,
	"POST /api/v1/stages/{name}/resume":      needUsers,
	"POST /api/v1/stages/{name}/restart":     needUsers,
	"POST /api/v1/links/{kind}/{id}/restart": needUsers,
	"POST /api/v1/app/restart":               needUsers,
	"POST /api/v1/app/safe-mode":             needUsers,
	"POST /api/v1/app/shutdown":              needUsers,

	// This app (3.5.7, 3.5.9): where things are kept needs settings; the
	// log is for the owner and admins only (it is masked, but it says a
	// lot about the home network).
	"GET /api/v1/app/storage":   PermSettings,
	"GET /api/v1/logs":          needUsers,
	"GET /api/v1/logs/download": needUsers,

	// Schedules (3.5.8): staff who run the Library see them and run one
	// now (the Job's part must be on); the intervals are settings.
	"GET /api/v1/app/update":            PermManageLibrary,
	"POST /api/v1/app/update/check":     PermSettings,
	"GET /api/v1/schedules":             PermManageLibrary,
	"POST /api/v1/schedules/{kind}/run": PermManageLibrary,
	"PUT /api/v1/schedules/{kind}":      PermSettings,

	// The move tool (6.1): reading an old app's setup reaches other
	// machines with keys the owner types and keeps what it read, so it is
	// for the owner and admins with a password sign-in and the admin scope.
	"POST /api/v1/migrate/probe":            needUsers,
	"POST /api/v1/migrate/read":             needUsers,
	"GET /api/v1/migrate/snapshots":         needUsers,
	"GET /api/v1/migrate/snapshots/{id}":    needUsers,
	"DELETE /api/v1/migrate/snapshots/{id}": needUsers,
	"POST /api/v1/migrate/plan":             needUsers,
	"POST /api/v1/migrate/dry-run":          needUsers,
	"GET /api/v1/migrate/moves":             needUsers,
	"POST /api/v1/migrate/moves":            needUsers,
	"GET /api/v1/migrate/moves/{id}":        needUsers,
	"GET /api/v1/migrate/moves/{id}/report": needUsers,
	"POST /api/v1/migrate/moves/{id}/undo":  needUsers,
}

// API key scopes. A key holds one or more; a session holds them all.
// Every route needs exactly one scope (RouteScope), so the route table
// decides it and the OpenAPI spec publishes it (x-holyarrs-scope).
const (
	ScopeHealthRead = "health_read" // only the health state feed; intended for monitoring companions
	ScopeRead       = "read"        // look: the GET routes in ReadRoutes, including the health state feed
	ScopeWrite      = "write"       // change: every other route but the admin ones and the controls
	ScopeAdmin      = "admin"       // people, invites, ask limits, API keys, password and two-factor, sign-in settings, restore
	ScopeControl    = "control"     // the controls: part switches, stages, linked apps, whole-app restart and shut down
	// ScopeAll is what every key made before scopes holds: read, write
	// and admin together (every route but the controls).
	ScopeAll = "all"
)

// Scopes lists the scopes a key may be given.
var Scopes = []string{ScopeHealthRead, ScopeRead, ScopeWrite, ScopeAdmin, ScopeControl, ScopeAll}

// ScopeNotes says in plain words what each scope reaches; the OpenAPI
// spec publishes it.
var ScopeNotes = map[string]string{
	ScopeHealthRead: "Health states only, for monitoring companions.",
	ScopeRead:       "Look: the GET routes that hand out no secrets (backups, keys, tokens, sign-in settings and the people list need admin).",
	ScopeWrite:      "Change: every route that is not a GET, except the admin ones and the controls.",
	ScopeAdmin:      "People, invites, ask limits, API keys, password and two-factor sign-in, sign-in settings, backups (list, download, restore) and the outside-service key settings: every route only the owner or an admin may use, the key and sign-in routes of My account, and every GET route not marked read. Making or promoting an admin also needs the control scope.",
	ScopeControl:    "The controls: part switches, stage pause, resume and restart, reconnecting one linked app, whole-app restart, restart in safe mode and shut down.",
	ScopeAll:        "Read, write and admin together (every route but the controls). Every key made before scopes existed holds this one.",
}

// RouteScopes names routes whose scope is not the one the method and
// RoutePerms give them (see RouteScope).
var RouteScopes = map[string]string{
	"GET /api/v1/downloaders/{id}/attempts":                      ScopeAdmin,
	"GET /api/v1/explore/title-stats":                            ScopeAdmin,
	"POST /api/v1/downloaders/{id}/attempts/{attemptID}/abandon": ScopeAdmin,
	"POST /api/v1/auth/oidc/start":                               ScopeAdmin,
	"POST /api/v1/auth/oidc/finish":                              ScopeAdmin,
	"POST /api/v1/auth/forward/login":                            ScopeAdmin,
	"POST /api/v1/people/{id}/movies/review":                     ScopeRead,
	"GET /api/v1/health/states":                                  ScopeHealthRead,
	"GET /api/v1/data-packs":                                     ScopeAdmin,
	"GET /api/v1/data-packs/{id}":                                ScopeAdmin,
	"GET /api/v1/data-packs/{id}/preview":                        ScopeAdmin,
	"POST /api/v1/parts/{part}/on":                               ScopeControl,
	"POST /api/v1/parts/{part}/off":                              ScopeControl,
	"POST /api/v1/parts/{part}/restart":                          ScopeControl,
	"POST /api/v1/stages/{name}/pause":                           ScopeControl,
	"POST /api/v1/stages/{name}/resume":                          ScopeControl,
	"POST /api/v1/stages/{name}/restart":                         ScopeControl,
	"POST /api/v1/links/{kind}/{id}/restart":                     ScopeControl,
	"POST /api/v1/app/restart":                                   ScopeControl,
	"POST /api/v1/app/safe-mode":                                 ScopeControl,
	"POST /api/v1/app/shutdown":                                  ScopeControl,

	// My account's sign-in and the keys themselves.
	"GET /api/v1/auth/keys":                     ScopeAdmin,
	"POST /api/v1/auth/keys":                    ScopeAdmin,
	"POST /api/v1/auth/reauth":                  ScopeAdmin,
	"POST /api/v1/auth/oidc/link":               ScopeAdmin,
	"POST /api/v1/auth/oidc/link-finish":        ScopeAdmin,
	"POST /api/v1/auth/forward/link":            ScopeAdmin,
	"DELETE /api/v1/auth/keys/{id}":             ScopeAdmin,
	"POST /api/v1/auth/password":                ScopeAdmin,
	"POST /api/v1/auth/two-factor/setup":        ScopeAdmin,
	"POST /api/v1/auth/two-factor/enable":       ScopeAdmin,
	"POST /api/v1/auth/two-factor/disable":      ScopeAdmin,
	"POST /api/v1/auth/two-factor/backup-codes": ScopeAdmin,
	"GET /api/v1/auth/sessions":                 ScopeAdmin,
	"DELETE /api/v1/auth/sessions":              ScopeAdmin,
	"DELETE /api/v1/auth/sessions/{sid}":        ScopeAdmin,

	// GET routes that hand out the database, keys, tokens, sign-in
	// settings or the people list: never the read scope.
	"GET /api/v1/backups":                 ScopeAdmin,
	"GET /api/v1/backups/{name}":          ScopeAdmin, // the whole database, password hashes included
	"GET /api/v1/auth/settings":           ScopeAdmin,
	"GET /api/v1/settings/trakt-key":      ScopeAdmin,
	"GET /api/v1/settings/trakt-token":    ScopeAdmin,
	"GET /api/v1/settings/tvdb-key":       ScopeAdmin,
	"GET /api/v1/settings/theaudiodb-key": ScopeAdmin,
	"GET /api/v1/calendar/feed-key":       ScopeAdmin,
	"GET /api/v1/users":                   ScopeAdmin,
	"GET /api/v1/users/invites":           ScopeAdmin,
	"GET /api/v1/app/storage":             ScopeAdmin,
	"GET /api/v1/logs":                    ScopeAdmin,
	"GET /api/v1/logs/download":           ScopeAdmin,
	"GET /api/v1/users/{id}/sessions":     ScopeAdmin,
	"GET /api/v1/ask-limits":              ScopeAdmin,
	"GET /api/v1/auth/state":              ScopeAdmin, // public; a key never needs it
	"GET /api/v1/auth/invites/{token}":    ScopeAdmin, // public; a key never needs it
	"GET /api/v1/auth/oidc/callback":      ScopeAdmin, // public; a key never needs it

	// The move tool: what old apps hold (people, emails, links) is admin.
	"POST /api/v1/migrate/probe":            ScopeAdmin,
	"POST /api/v1/migrate/read":             ScopeAdmin,
	"GET /api/v1/migrate/snapshots":         ScopeAdmin,
	"GET /api/v1/migrate/snapshots/{id}":    ScopeAdmin,
	"DELETE /api/v1/migrate/snapshots/{id}": ScopeAdmin,
	"GET /api/v1/migrate/saved-keys":        ScopeAdmin,
	"DELETE /api/v1/migrate/saved-keys":     ScopeAdmin,
	"POST /api/v1/settings/import-secrets":  ScopeAdmin,
	"POST /api/v1/migrate/plan":             ScopeAdmin,
	"POST /api/v1/migrate/dry-run":          ScopeAdmin,
	"GET /api/v1/migrate/moves":             ScopeAdmin,
	"POST /api/v1/migrate/moves":            ScopeAdmin,
	"GET /api/v1/migrate/moves/{id}":        ScopeAdmin,
	"GET /api/v1/migrate/moves/{id}/report": ScopeAdmin,
	"POST /api/v1/migrate/moves/{id}/undo":  ScopeAdmin,
}

// ReadRoutes are the GET routes a read key may use. A GET route listed
// neither here nor in RouteScopes needs the admin scope (fail closed), and
// TestEveryGETHasAnExplicitScope fails until someone decides. Secrets in
// these answers are masked.
var ReadRoutes = map[string]bool{}

func init() {
	for _, r := range []string{
		"GET /api/v1/titles/{id}/copies",
		"GET /api/v1/people/search",
		"GET /api/v1/people/{id}",
		"GET /api/v1/titles/{id}/people",
		"GET /api/v1/collections",
		"GET /api/v1/collections/{id}",
		"GET /api/v1/library/copies",

		"GET /", "GET /login", "GET /live",
		"GET /api/v1/albums/{id}", "GET /api/v1/albums/{id}/tracks", "GET /api/v1/albums/{id}/cover", "GET /api/v1/alerts",
		"GET /api/v1/artists/lookup", "GET /api/v1/artwork/{name}", "GET /api/v1/artwork/tmdb/{size}/{file}", "GET /api/v1/asks",
		"GET /api/v1/asks/limit", "GET /api/v1/asks/{id}", "GET /api/v1/auth/me",
		"GET /api/v1/calendar", "GET /api/v1/calendar/feed.ics", "GET /api/v1/downloaders",
		"GET /api/v1/downloads", "GET /api/v1/downloads/view", "GET /api/v1/events",
		"GET /api/v1/health", "GET /api/v1/health/checks",
		"GET /api/v1/held", "GET /api/v1/imports", "GET /api/v1/jobs", "GET /api/v1/jobs/{id}",
		"GET /api/v1/library-folders", "GET /api/v1/library-summary", "GET /api/v1/home", "GET /api/v1/media-servers", "GET /api/v1/messages",
		"GET /api/v1/metadata/search", "GET /api/v1/metadata/health", "GET /api/v1/metadata/{kind}/{tmdb_id}", "GET /api/v1/metadata/{kind}/{tmdb_id}/more-from", "GET /api/v1/metadata/{kind}/{tmdb_id}/reviews", "GET /api/v1/metadata/show/{tmdb_id}/seasons/{number}", "GET /api/v1/trailer/{kind}/{tmdb_id}", "GET /api/v1/trailer/batch", "GET /api/v1/trailer/stream/{id}/manifest.m3u8", "GET /api/v1/trailer/stream/{id}/seg/{n}", "GET /api/v1/settings/ui", "GET /api/v1/discover", "GET /api/v1/watch-services", "GET /api/v1/top10", "GET /api/v1/explore/head", "GET /api/v1/explore/tail", "GET /api/v1/explore/top", "GET /api/v1/explore/service-top", "GET /api/v1/explore/genre", "GET /api/v1/explore/genre-row", "GET /api/v1/music-trending", "GET /api/v1/music-trending/art/{kind}/{mbid}", "GET /api/v1/explore/music", "GET /api/v1/my/alerts", "GET /api/v1/asked-by", "GET /api/v1/my/ask-privacy", "GET /api/v1/my/follows", "GET /api/v1/titles/{id}/follow", "GET /api/v1/needs-decision",
		"GET /api/v1/never-add", "GET /api/v1/openapi.json", "GET /api/v1/outside-lists",
		"GET /api/v1/parts", "GET /api/v1/pick-rules", "GET /api/v1/pick-rules/{id}", "GET /api/v1/rename", "GET /api/v1/searches",
		"GET /api/v1/settings/alerts", "GET /api/v1/settings/asks", "GET /api/v1/settings/downloaders",
		"GET /api/v1/settings/files", "GET /api/v1/settings/health", "GET /api/v1/settings/music",
		"GET /api/v1/settings/naming", "GET /api/v1/settings/release-gate", "GET /api/v1/settings/search",
		"GET /api/v1/settings/shows", "GET /api/v1/settings/status", "GET /api/v1/settings/subtitles", "GET /api/v1/settings/title-defaults", "GET /api/v1/shows/lookup",
		"GET /api/v1/sources", "GET /api/v1/sources/{id}/stats", "GET /api/v1/subtitle-sites",
		"GET /api/v1/subtitle-sites/kinds", "GET /api/v1/subtitles/languages", "GET /api/v1/subtitles/missing",
		"GET /api/v1/titles", "GET /api/v1/titles/lookup", "GET /api/v1/titles/{id}",
		"GET /api/v1/titles/{id}/albums", "GET /api/v1/titles/{id}/episodes", "GET /api/v1/titles/{id}/releases",
		"GET /api/v1/titles/{id}/rename", "GET /api/v1/titles/{id}/seasons", "GET /api/v1/titles/{id}/subtitles",
		"GET /api/v1/turned-down", "GET /api/v1/schedules", "GET /api/v1/app/update",
	} {
		ReadRoutes[r] = true
	}
}

// HasScope reports whether the request's key holds scope; a session (or
// the local network) holds every scope. ScopeAll covers read, write and
// admin.
func (w Who) HasScope(scope string) bool {
	if w.Via != "api_key" {
		return true
	}
	return ScopesCover(w.Scopes, scope)
}

// ScopesCover reports whether a key holding held may use scope.
func ScopesCover(held []string, scope string) bool {
	if slices.Contains(held, scope) {
		return true
	}
	if scope == ScopeHealthRead && slices.Contains(held, ScopeRead) {
		return true
	}
	return scope != ScopeControl && slices.Contains(held, ScopeAll)
}

// RouteScope is the scope an API key needs for a route pattern: the
// RouteScopes entry, else admin for a route only the owner or admins may
// use, else read for a GET listed in ReadRoutes, admin for any other GET,
// and write for every other method.
func RouteScope(pattern string) string {
	if sc := RouteScopes[pattern]; sc != "" {
		return sc
	}
	if need, ok := RoutePerms[pattern]; !ok || need == needUsers || need == needOwner {
		return ScopeAdmin
	}
	method, _, _ := strings.Cut(pattern, " ")
	if method == http.MethodGet || method == http.MethodHead {
		if ReadRoutes[pattern] {
			return ScopeRead
		}
		return ScopeAdmin // no decision yet: fail closed
	}
	return ScopeWrite
}

// Gate checks every signed-in request against RoutePerms. It must sit
// between Middleware and the mux.
func Gate(m *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		who, ok := From(r.Context())
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
			need = needOwner
		}
		if who.MustTwoFactor && need != needSelf {
			fail(w, http.StatusForbidden, "turn on two-factor sign-in in My account first")
			return
		}
		if scope := RouteScope(pattern); !who.HasScope(scope) {
			fail(w, http.StatusForbidden, "this API key's scope does not cover this route; make a key with the "+scope+" scope")
			return
		}
		if who.Via != "api_key" && (RouteScope(pattern) == ScopeAdmin || RouteScope(pattern) == ScopeControl) &&
			!RecentLocal(r) && !recentLocalExempt(pattern, who) {
			fail(w, http.StatusForbidden, "confirm your local password again before using this route")
			return
		}
		if !who.Has(need) {
			if r.Method == http.MethodGet && !strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/live" {
				http.Redirect(w, r, "/", http.StatusSeeOther)
				return
			}
			fail(w, http.StatusForbidden, "your account may not do this; ask the owner")
			return
		}
		m.ServeHTTP(w, r)
	})
}
