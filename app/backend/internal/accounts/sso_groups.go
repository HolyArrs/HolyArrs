package accounts

import (
	"errors"
	"sort"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// Roles from single sign-on groups.
//
// - The owner names the claim that carries groups (often "groups") and which
//   group gives which role. Both empty means roles are set only in HolyArrs.
// - Each sign-in sets the role from the groups: the highest mapped role
//   wins, and someone in none of the mapped groups is a member. Taking a
//   person out of the admin group makes them a member at their next sign-in.
// - A group never makes anyone owner and the owner's role never changes.
// - When the provider sends no such claim, the role stays as it is, so a
//   provider that drops the claim cannot demote everyone.

const maxGroupRoles = 50

var errGroupRoles = errors.New(`group roles must be "group=admin" or "group=member", comma separated`)

// groupRole is the role the claim asks for, or "" to leave the role alone.
func groupRole(claim string, roles map[string]string, raw map[string]any) string {
	if claim == "" || len(roles) == 0 {
		return ""
	}
	var groups []string
	switch v := raw[claim].(type) {
	case string:
		// A lone string counts only under a groups-like name: elsewhere it
		// may be text the person can change themselves.
		if claim != "groups" && claim != "roles" {
			return ""
		}
		groups = []string{v}
	case []any:
		for _, g := range v {
			if s, ok := g.(string); ok {
				groups = append(groups, s)
			}
		}
	case []string:
		groups = v
	default:
		return ""
	}
	role := webserver.RoleMember
	for _, g := range groups {
		if roles[strings.TrimSpace(g)] == webserver.RoleAdmin {
			role = webserver.RoleAdmin
		}
	}
	return role
}

// parseGroupRoles reads "admins=admin, family=member".
func parseGroupRoles(s string) (map[string]string, error) {
	out := map[string]string{}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		group, role, ok := strings.Cut(part, "=")
		group, role = strings.TrimSpace(group), strings.TrimSpace(role)
		if !ok || !ssoText(group, 200) || role != webserver.RoleAdmin && role != webserver.RoleMember {
			return nil, errGroupRoles
		}
		out[group] = role
	}
	if len(out) > maxGroupRoles {
		return nil, errGroupRoles
	}
	return out, nil
}

// formatGroupRoles writes the map back in a fixed order.
func formatGroupRoles(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + "=" + m[k]
	}
	return strings.Join(parts, ", ")
}

// selfEditedClaims are claims people can usually change themselves, so
// they never decide a role.
var selfEditedClaims = map[string]bool{"sub": true, "email": true, "name": true, "preferred_username": true,
	"nickname": true, "given_name": true, "family_name": true}

// validGroupsClaim allows a plain claim name, or none.
func validGroupsClaim(s string) bool {
	return s == "" || ssoText(s, 100) && !strings.ContainsAny(s, " ,") && !selfEditedClaims[strings.ToLower(s)]
}
