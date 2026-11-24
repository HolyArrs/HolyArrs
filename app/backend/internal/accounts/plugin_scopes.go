package accounts

import (
	_ "embed"
	"encoding/json"
	"errors"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"regexp"
	"slices"
	"strings"
)

// plugin_scopes.json is the one list of plugin scopes: the route map, the
// closed routes, the scopes with no route yet and the 8 install groups. The
// plugin catalog's checks read the same file.
//
//go:embed plugin_scopes.json
var pluginScopesJSON []byte

// PluginGroup is one install-screen line and the scope areas it covers.
type PluginGroup struct {
	ID    string   `json:"id"`
	Areas []string `json:"areas"`
}

// Scope names: <area>[.<sub>...].<read|change|control|other verb>. A name
// is a tree path; the last part is the verb.
var pluginScopeRe = regexp.MustCompile(`^[a-z][a-z-]*(\.[a-z][a-z-]*)+$`)

var (
	// PluginScopes maps a route pattern to the scope a plugin needs for it.
	// A route not in it can't be called by a plugin.
	PluginScopes = map[string]string{}
	// PluginClosed are the routes no plugin scope ever reaches: the plugin
	// system, accounts and sign-in, connections that hold secrets, and
	// server settings and controls.
	PluginClosed = map[string]bool{}

	pluginGroups []PluginGroup
	pluginList   []string
)

func init() {
	var f struct {
		Groups []PluginGroup     `json:"groups"`
		New    []string          `json:"new"`
		Routes map[string]string `json:"routes"`
		Closed []string          `json:"closed"`
	}
	if err := json.Unmarshal(pluginScopesJSON, &f); err != nil {
		panic("plugin_scopes.json: " + err.Error())
	}
	PluginScopes = f.Routes
	for _, r := range f.Closed {
		PluginClosed[r] = true
	}
	pluginGroups = f.Groups
	seen := map[string]bool{}
	for _, s := range f.Routes {
		seen[s] = true
	}
	for _, s := range f.New {
		seen[s] = true
	}
	for s := range seen {
		pluginList = append(pluginList, s)
	}
	slices.Sort(pluginList)
}

// PluginScopeList is every named plugin scope, sorted.
func PluginScopeList() []string { return slices.Clone(pluginList) }

// PluginGroups are the 8 install-screen groups.
func PluginGroups() []PluginGroup { return slices.Clone(pluginGroups) }

// PluginScopeGroup is the install group of a scope, or "".
func PluginScopeGroup(scope string) string {
	area, _, _ := strings.Cut(scope, ".")
	for _, g := range pluginGroups {
		if slices.Contains(g.Areas, area) {
			return g.ID
		}
	}
	return ""
}

// ValidPluginScope says whether a manifest may ask for scope: a named scope,
// or a parent that covers at least one (files.read covers files.folders.read).
// Closed routes have no scope, so no name reaches them.
func ValidPluginScope(scope string) bool {
	if !pluginScopeRe.MatchString(scope) {
		return false
	}
	for _, s := range pluginList {
		if scopeCovers(scope, s) {
			return true
		}
	}
	return false
}

// PluginScopesCover says whether held covers need: the same scope, or a
// parent with the same verb (library.read covers library.music.read).
func PluginScopesCover(held []string, need string) bool {
	if need == "" {
		return false
	}
	for _, h := range held {
		if scopeCovers(h, need) {
			return true
		}
	}
	return false
}

func scopeCovers(held, need string) bool {
	if held == need {
		return true
	}
	hi := strings.LastIndexByte(held, '.')
	ni := strings.LastIndexByte(need, '.')
	if hi < 0 || ni < 0 || held[hi:] != need[ni:] {
		return false
	}
	return strings.HasPrefix(need[:ni], held[:hi]+".")
}

// PluginScopeRoutes are the routes scope (or any child of it) opens, sorted.
func PluginScopeRoutes(scope string) []string {
	var out []string
	for r, s := range PluginScopes {
		if scopeCovers(scope, s) {
			out = append(out, r)
		}
	}
	slices.Sort(out)
	return out
}

// CallerCoversPluginScope says whether a person may themselves do all that
// scope opens: every route of it passes their role and permissions. A scope
// with no route yet needs the owner. A plugin tool that uses scope is shown
// and run only for such a person.
func CallerCoversPluginScope(who webserver.Caller, scope string) bool {
	routes := PluginScopeRoutes(scope)
	if len(routes) == 0 {
		return who.Has(webserver.NeedOwner)
	}
	for _, r := range routes {
		need, ok := RoutePerms[r]
		if !ok {
			need = webserver.NeedOwner
		}
		if !who.Has(need) {
			return false
		}
	}
	return true
}

// PluginBridgeAllowed decides one call a plugin page makes for the signed-in
// person who: the route's plugin scope must be held by the plugin (held)
// and the route's own permission by the person, so the call runs with the
// overlap of the two. Closed and unknown routes are refused.
func PluginBridgeAllowed(held []string, who webserver.Caller, pattern string) error {
	need := PluginScopes[pattern]
	if need == "" || PluginClosed[pattern] {
		return errors.New("plugin pages can't use this route")
	}
	if !PluginScopesCover(held, need) {
		return errors.New("this plugin was not given the " + need + " permission")
	}
	perm, ok := RoutePerms[pattern]
	if !ok {
		perm = webserver.NeedOwner
	}
	if !who.Has(perm) {
		return errors.New("your account may not do this; ask the owner")
	}
	return nil
}
