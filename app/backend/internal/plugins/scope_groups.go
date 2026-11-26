package plugins

import (
	"slices"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
)

// ScopeGroup is one install-screen line: an area, whether it may change
// things there, the exact scopes behind it and the red warnings it carries.
type ScopeGroup struct {
	ID       string   `json:"id"`
	Change   bool     `json:"change"`
	Scopes   []string `json:"scopes"`
	Warnings []string `json:"warnings,omitempty"`
}

// scopeWarning names the red install line of scopes that reach more than
// they seem to, or "".
func scopeWarning(scope string) string {
	switch {
	case scope == "network.forward" || strings.HasPrefix(scope, "network."):
		return "forward"
	case strings.HasPrefix(scope, "sources.downloader"):
		return "downloader"
	case scope == "pages.title-section":
		return "sections"
	case scope == "library.artwork.set":
		return "artwork"
	case scope == "alerts.webhooks.send":
		return "notify"
	case scope == "sources.metadata":
		return "ratings"
	case strings.HasSuffix(scope, ".delete"):
		return "delete"
	case strings.HasPrefix(scope, "files.") && !strings.HasSuffix(scope, ".read"):
		return "files"
	case strings.HasPrefix(scope, "requests.approve."):
		return "approve"
	case strings.HasPrefix(scope, "requests.limits.") && !strings.HasSuffix(scope, ".read"):
		return "limits"
	}
	return ""
}

// scopeGroups sorts a's scopes into the install groups, in group order.
func scopeGroups(a *Access) []ScopeGroup {
	if a == nil || len(a.Actions) == 0 {
		return nil
	}
	var out []ScopeGroup
	for _, g := range accounts.PluginGroups() {
		sg := ScopeGroup{ID: g.ID}
		for _, sc := range a.Actions {
			if accounts.PluginScopeGroup(sc) != g.ID {
				continue
			}
			sg.Scopes = append(sg.Scopes, sc)
			sg.Change = sg.Change || !strings.HasSuffix(sc, ".read")
			if w := scopeWarning(sc); w != "" && !slices.Contains(sg.Warnings, w) {
				sg.Warnings = append(sg.Warnings, w)
			}
		}
		if len(sg.Scopes) > 0 {
			slices.Sort(sg.Scopes)
			out = append(out, sg)
		}
	}
	return out
}
