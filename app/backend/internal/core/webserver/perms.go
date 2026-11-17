package webserver

import (
	"net/http"
	"slices"
)

// Roles a person can hold.
const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"
)

// Perm is one thing a person may do. The owner may do everything.
type Perm string

// Permissions a person can be given.
const (
	PermAsk         Perm = "ask"          // ask for titles
	PermAutoApprove Perm = "auto_approve" // their asks are approved without waiting
	// PermAutoApproveFourK: their 4K asks are approved without waiting too
	// (with PermAutoApprove).
	PermAutoApproveFourK Perm = "auto_approve_four_k"
	PermApprove          Perm = "approve"        // approve or turn down other people's asks
	PermManageLibrary    Perm = "manage_library" // add, edit, search and download titles
	PermSettings         Perm = "settings"       // change settings
)

// Needs a route can declare besides the permissions above.
const (
	NeedSignedIn Perm = "signed_in" // anyone signed in
	// NeedPublic marks a route that needs no sign-in because it checks a key
	// of its own, so the sign-in middleware lets it through.
	NeedPublic Perm = "public"
	// NeedSelf is for My account: open even to someone who must still turn on
	// the two-factor sign-in the owner requires.
	NeedSelf  Perm = "self"
	NeedUsers Perm = "users" // the owner or an admin
	NeedOwner Perm = "owner" // the owner only
)

// AllPerms lists the permissions in the order screens show them.
var AllPerms = []Perm{PermAsk, PermAutoApprove, PermAutoApproveFourK, PermApprove, PermManageLibrary, PermSettings}

// Authenticator turns a request into its Caller. Accounts implements it, so
// no module has to import accounts to ask who is calling.
type Authenticator interface {
	Session(r *http.Request) (Caller, error)
}

// DefaultPerms are what a new person of a role starts with.
func DefaultPerms(role string) []Perm {
	switch role {
	case RoleAdmin:
		return []Perm{PermAsk, PermAutoApprove, PermApprove, PermManageLibrary}
	case RoleMember:
		return []Perm{PermAsk}
	}
	return slices.Clone(AllPerms)
}

// Has reports whether the caller may do p.
func (c Caller) Has(p Perm) bool {
	switch {
	case c.Role == RoleOwner:
		return true
	case p == NeedSignedIn || p == NeedSelf || p == NeedPublic:
		return true
	case p == NeedUsers:
		return c.Role == RoleAdmin
	case p == NeedOwner:
		return false
	}
	return slices.Contains(c.Perms, p)
}

// Effective is the full list of permissions the caller holds.
func (c Caller) Effective() []Perm {
	out := []Perm{}
	for _, p := range AllPerms {
		if c.Has(p) {
			out = append(out, p)
		}
	}
	return out
}
