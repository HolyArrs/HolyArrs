// Package mcpkeys sorts every route of the app into a few plain permission
// groups, so the owner can tick, per MCP key, what an AI app may do. A route
// the rules do not know is refused.
package mcpkeys

import (
	"net/http"
	"slices"
	"strings"
)

// Group is one tick box on an MCP key.
type Group string

// The groups, in the order a screen shows them.
const (
	See      Group = "see"      // see library and activity
	Add      Group = "add"      // add and request
	Control  Group = "control"  // control downloads, searches and imports
	Settings Group = "settings" // change settings
	Delete   Group = "delete"   // delete or remove anything
	Admin    Group = "admin"    // users, keys, sign-in, backups, updates, the whole app
	Approve  Group = "approve"  // approve or decline pending requests; Admin covers it too
	Plugins  Group = "plugins"  // use tools that plugins add; off until the owner ticks it
)

// Set is the groups one key holds.
type Set []Group

// All lists every group.
var All = []Group{See, Add, Control, Settings, Delete, Admin, Approve, Plugins}

// Default is what a new AI key holds, and what every key held when the tick
// boxes came: all but Delete and Admin.
var Default = Set{See, Add, Control, Settings}

// labels are the tick-box names a person reads.
var labels = map[Group]string{See: "See", Add: "Add", Control: "Control", Settings: "Settings", Delete: "Delete", Admin: "Admin", Approve: "Approve requests", Plugins: "Plugin tools"}

// Label is the tick-box name of g, as a screen and an error say it.
func (g Group) Label() string { return labels[g] }

// Parse keeps the known groups of names, in the order of All, once each.
// ok is false when a name is not a group.
func Parse(names []string) (set Set, ok bool) {
	ok = true
	for _, n := range names {
		if !slices.Contains(All, Group(n)) {
			ok = false
		}
	}
	for _, g := range All {
		if slices.Contains(names, string(g)) {
			set = append(set, g)
		}
	}
	return set, ok
}

// overrides pin single routes whose path alone says the wrong thing.
var overrides = map[string]Group{
	"GET /api/v1/auth/me":                          See,
	"POST /api/v1/explore/counts":                  See, // a read sent as POST
	"POST /api/v1/explore/strip/counts":            See,
	"POST /api/v1/live/interest":                   See,
	"POST /api/v1/messages/read":                   See,
	"POST /api/v1/settings/naming/example":         See,
	"POST /api/v1/collections/{id}/remove/preview": See, // shows what a removal would do; changes nothing
	"GET /mcp":                 See, // the AI route itself; tools are checked one by one
	"POST /mcp":                See,
	"GET /api/v1/mcp":          See,
	"POST /api/v1/mcp":         See,
	"GET /healthz":             See,
	"HEAD /healthz":            See,
	"GET /readyz":              See,
	"HEAD /readyz":             See,
	"GET /ping":                See,
	"HEAD /ping":               See,
	"GET /api/v1/plugin-pages": Plugins,
	"POST /api/v1/plugin-pages/{plugin}/{page}/open": Plugins,
	"POST /api/v1/plugin-pages/{plugin}/call":        Plugins,
	"POST /api/v1/plugin-pages/{plugin}/play":        Plugins,
	"POST /api/v1/asks/{id}/approve":                 Approve,
	"POST /api/v1/asks/{id}/turn-down":               Approve,
	"POST /api/v1/downloaders":                       Settings, // adding a download app is setup
	"POST /api/v1/manual-import/folders":             Admin,    // lists folders on the server's disks
	"PUT /api/v1/downloaders/{id}":                   Settings,
	"POST /api/v1/downloaders/{id}/test":             Settings,
	"POST /api/v1/downloaders/{id}/category":         Settings,
	"GET /api/v1/downloaders/{id}/categories":        Settings,
}

// wildOverrides are the override patterns with a {wildcard}, sorted so the
// match never depends on map order.
var wildOverrides = func() []string {
	var out []string
	for k := range overrides {
		if strings.Contains(k, "{") {
			out = append(out, k)
		}
	}
	slices.Sort(out)
	return out
}()

// roots are the first path words the app serves. Anything else is unknown.
var roots = []string{
	"", "api", "sonarr", "radarr", "mcp", "oauth", ".well-known", "login", "plex-done", "live", "invite", "healthz", "readyz", "ping",
	// the old-app API, below /sonarr/ and /radarr/
	"calendar", "command", "config", "customformat", "delayprofile", "episode", "exclusions", "importlistexclusion",
	"indexer", "languageprofile", "mediacover", "movie", "notification", "qualitydefinition", "qualityprofile",
	"queue", "rootfolder", "series", "system", "tag", "wanted", "health", "history", "diskspace", "customfilter", "episodefile", "seasonpass",
	"release", "blocklist", "log", "update", "moviefile",
}

// adminPrefixes cover sign-in, people, keys and the whole app.
var adminPrefixes = []string{
	"/api/v1/auth", "/api/v1/users", "/api/v1/backups", "/api/v1/app", "/api/v1/system", "/api/v1/data-packs", "/api/v1/plugins",
	"/api/v1/plugin-images", "/api/v1/migrate", "/api/v1/logs", "/api/v1/support", "/api/v1/parts", "/api/v1/stages", "/api/v1/links",
	"/api/v1/settings/old-app-api/keys", "/api/v1/settings/import-secrets", "/api/v1/settings/url-base",
	"/oauth", "/.well-known", "/invite", "/login", "/plex-done", "/system", "/health",
}

// controlPrefixes cover running downloads, searches and imports.
var controlPrefixes = []string{
	"/api/v1/downloads", "/api/v1/downloaders/", "/api/v1/holding", "/api/v1/needs-decision", "/api/v1/releases",
	"/api/v1/manual-import", "/api/v1/rename", "/api/v1/imports", "/api/v1/search/", "/api/v1/jobs",
	"/api/v1/subtitles/", "/api/v1/library/copies/search", "/command", "/queue",
}

// settingsPrefixes cover how the app is set up and each person's own setup.
var settingsPrefixes = []string{
	"/api/v1/settings", "/api/v1/downloaders", "/api/v1/sources", "/api/v1/bridges", "/api/v1/media-servers",
	"/api/v1/library-folders", "/api/v1/subtitle-sites", "/api/v1/pick-rules", "/api/v1/own-traits",
	"/api/v1/schedules", "/api/v1/alerts", "/api/v1/ask-limits", "/api/v1/outside-lists", "/api/v1/my",
	"/api/v1/account", "/api/v1/health/checks/run", "/api/v1/tags", "/api/v1/watch-services",
	"/config", "/customformat", "/delayprofile", "/indexer", "/notification", "/qualitydefinition",
	"/qualityprofile", "/rootfolder", "/tag", "/languageprofile",
}

// deleteWords in a write path mean it takes something away.
var deleteWords = []string{"remove", "delete", "trash", "clean", "clear-now", "abandon"}

// GroupFor returns the group of a route pattern ("METHOD /path", as
// http.ServeMux writes it). It is false for a pattern it does not know.
func GroupFor(method, pattern string) (Group, bool) {
	method = strings.ToUpper(method)
	if m, p, ok := strings.Cut(pattern, " "); ok {
		if !strings.EqualFold(m, method) {
			return "", false
		}
		pattern = p
	}
	if !strings.HasPrefix(pattern, "/") || !knownRoot(pattern) {
		return "", false
	}
	if g, ok := override(method, pattern); ok {
		return g, true
	}
	read := false
	switch method {
	case http.MethodGet, http.MethodHead:
		read = true
	case http.MethodDelete, http.MethodPost, http.MethodPut, http.MethodPatch:
	default:
		return "", false
	}
	// Every verb on an admin route is Admin, deletes included: a key with
	// only Delete ticked must never remove people or keys.
	if hasPrefix(pattern, adminPrefixes) {
		return Admin, true
	}
	if method == http.MethodDelete {
		return Delete, true
	}
	if read {
		// Settings answers can carry addresses and keys.
		if strings.HasPrefix(pattern, "/api/v1/settings") {
			return Settings, true
		}
		return See, true
	}
	for _, w := range deleteWords {
		for _, seg := range strings.Split(pattern, "/") {
			if seg == w {
				return Delete, true
			}
		}
	}
	switch {
	case hasPrefix(pattern, controlPrefixes), strings.HasSuffix(pattern, "/search"), strings.HasSuffix(pattern, "/rescan"):
		return Control, true
	case hasPrefix(pattern, settingsPrefixes):
		return Settings, true
	}
	return Add, true
}

// Allows says whether a key holding set may call the route pattern. Unknown
// routes are refused. Admin also covers Approve, so a key that had Admin
// before the Approve box came keeps approving.
func Allows(set Set, method, pattern string) bool {
	g, ok := GroupFor(method, pattern)
	if !ok {
		return false
	}
	return slices.Contains(set, g) || (g == Approve && slices.Contains(set, Admin))
}

// override finds the pinned group of a pattern, or of a real path such as
// /api/v1/asks/3/approve that a {wildcard} pattern matches.
func override(method, path string) (Group, bool) {
	if g, ok := overrides[method+" "+path]; ok {
		return g, true
	}
	segs := strings.Split(path, "/")
	for _, k := range wildOverrides {
		g := overrides[k]
		m, p, _ := strings.Cut(k, " ")
		if m != method {
			continue
		}
		ps := strings.Split(p, "/")
		if len(ps) != len(segs) {
			continue
		}
		match := true
		for i := range ps {
			if ps[i] != segs[i] && !(strings.HasPrefix(ps[i], "{") && segs[i] != "") {
				match = false
				break
			}
		}
		if match {
			return g, true
		}
	}
	return "", false
}

func knownRoot(path string) bool {
	if strings.HasPrefix(path, "/api/") {
		return strings.HasPrefix(path, "/api/v1/")
	}
	first, _, _ := strings.Cut(strings.TrimPrefix(path, "/"), "/")
	return slices.Contains(roots, first)
}

func hasPrefix(path string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}
