package accounts

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// keyNewPeople is the sign-in setting with the permissions a new member
// starts with. Unset means a member's default, so nothing changes until the
// owner picks.
const keyNewPeople = "new_people_perms"

// newPeopleChoices are the permissions the owner may give everyone who
// joins. Change settings is left out: an import keeps it for a confirmed
// admin, and here it would go to anyone the sign-in way lets in.
var newPeopleChoices = []webserver.Perm{webserver.PermAsk, webserver.PermAutoApprove,
	webserver.PermAutoApproveFourK, webserver.PermApprove, webserver.PermManageLibrary}

// kidMay says whether a Kid may hold p: only asking, as any other
// permission opens routes the age-rating limit does not cover.
func kidMay(p webserver.Perm) bool {
	return p == webserver.PermAsk || p == webserver.PermAutoApprove || p == webserver.PermAutoApproveFourK
}

// readNewPeople checks a list the owner sends: known permissions a new
// member may start with, each once.
func readNewPeople(raw []string) ([]webserver.Perm, bool) {
	out := []webserver.Perm{}
	for _, s := range raw {
		p := webserver.Perm(s)
		if !slices.Contains(newPeopleChoices, p) {
			return nil, false
		}
		if !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out, true
}

// newPeoplePerms reads what a new member starts with. A value that cannot
// be read falls back to a member's default rather than to more.
func newPeoplePerms(ctx context.Context, q database.Querier) []webserver.Perm {
	v, found, err := database.SettingFound(ctx, q, settingsGroup, signInPrefix+keyNewPeople)
	if err != nil || !found {
		return webserver.DefaultPerms(webserver.RoleMember)
	}
	var raw []string
	if json.Unmarshal([]byte(v), &raw) != nil {
		return webserver.DefaultPerms(webserver.RoleMember)
	}
	out, ok := readNewPeople(raw)
	if !ok {
		return webserver.DefaultPerms(webserver.RoleMember)
	}
	return out
}

// startPerms are what a new person of role starts with when nobody named
// their permissions: an admin's default, or the owner's choice for members.
// A Kid keeps only what a Kid may hold.
func startPerms(ctx context.Context, q database.Querier, role string, kid bool) []webserver.Perm {
	if role != webserver.RoleMember {
		return webserver.DefaultPerms(role)
	}
	perms := newPeoplePerms(ctx, q)
	if kid {
		perms = slices.DeleteFunc(perms, func(p webserver.Perm) bool { return !kidMay(p) })
	}
	return perms
}
