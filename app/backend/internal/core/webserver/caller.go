package webserver

import (
	"context"
	"net/http"
	"slices"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// API key scopes. A key holds one or more; a session holds them all. Every
// route needs exactly one scope, so the route table decides it and the
// OpenAPI spec publishes it.
const (
	ScopeHealthRead = "health_read" // only the health state feed, for monitoring companions
	ScopeRead       = "read"        // look: the GET routes that hand out no secrets, plus the health state feed
	ScopeWrite      = "write"       // change: every other route but the admin ones and the controls
	ScopeAdmin      = "admin"       // people, invites, ask limits, API keys, password and two-factor, sign-in settings, restore
	ScopeControl    = "control"     // the controls: part switches, stages, linked apps, whole-app restart and shut down
	// ScopeAll is what every key made before scopes holds: read, write and
	// admin together (every route but the controls).
	ScopeAll = "all"
	// ScopeMCP is an AI key: it reaches only the AI connection route, and all
	// does not include it.
	ScopeMCP = "mcp"
	// ScopeCompat is held only by arr API keys; no other key can be made
	// with it, and all does not include it.
	ScopeCompat = "compat"
)

// Caller is the signed-in person behind a request, with how they got in.
type Caller struct {
	UserID int64
	Name   string
	Via    string // session, api_key, local_network
	CSRF   string
	Role   string
	Perms  []Perm
	// TwoFactor says two-factor sign-in is on for this person.
	TwoFactor bool
	// MustTwoFactor is set for an admin without two-factor sign-in when the
	// owner requires it; they may only reach My account until they turn it on.
	MustTwoFactor bool
	// AuthMethod records how the browser session was established. LocalRecent
	// is true only for a non-future local proof less than ten minutes old.
	AuthMethod  string `json:"auth_method"`
	LocalRecent bool   `json:"local_recent"`
	// OutsideRecent is true for a Plex, Jellyfin or Emby sign-in less than
	// ten minutes old: the proof a person with no password yet can give.
	OutsideRecent bool `json:"outside_recent"`
	// SetupRecent is true for the session first-run owner setup started,
	// until the first-run wizard is done; it then counts as LocalRecent.
	SetupRecent bool `json:"setup_recent"`
	// Scopes are an API key's scopes; empty for a session, which holds every
	// scope.
	Scopes []string
	// MaxRating is a Kid's age-rating limit ("" for no limit) and
	// RatingCountry the country whose ratings apply. Both are read from the
	// users row on every request, so a changed limit holds at once.
	MaxRating     string
	RatingCountry string
	// KeyID and KeyLabel name the API key a request came with (0 and "" for a
	// session); AI connections write them into History.
	KeyID    int64
	KeyLabel string
}

type callerKey struct{}

// From returns who made the request, if anyone.
func From(ctx context.Context) (Caller, bool) {
	c, ok := ctx.Value(callerKey{}).(Caller)
	return c, ok
}

// ContextWith puts c on ctx as the signed-in person, for in-process callers
// and tests; requests get it from the sign-in middleware.
func ContextWith(ctx context.Context, c Caller) context.Context {
	return context.WithValue(ctx, callerKey{}, c)
}

// Actor is the request's actor for "who and when".
func Actor(ctx context.Context) database.Actor {
	if c, ok := From(ctx); ok {
		return database.Actor{Kind: "user", ID: c.UserID}
	}
	return database.System
}

// RequestActor is the signed-in person on r; without sign-in (tests) it is
// the owner.
func RequestActor(r *http.Request) database.Actor {
	if _, ok := From(r.Context()); ok {
		return Actor(r.Context())
	}
	return database.Actor{Kind: "user", ID: 1}
}

// Strong reports whether the request uses a browser session or an API key.
// Requests let in by the local-network skip are not strong: they may not make
// API keys, change sign-in or security settings, read or replace secrets, or
// restore backups.
func Strong(ctx context.Context) bool {
	c, ok := From(ctx)
	return ok && (c.Via == "session" || c.Via == "api_key")
}

// RequireStrong writes 403 and returns false when the request is not Strong.
func RequireStrong(w http.ResponseWriter, r *http.Request) bool {
	if _, ok := From(r.Context()); !ok {
		return true // no sign-in in front, as in tests
	}
	if !Strong(r.Context()) {
		FailNoStore(w, http.StatusForbidden, "sign in with your password for this")
		return false
	}
	return true
}

// HasScope reports whether the request's key holds scope; a session or the
// local network holds every scope. ScopeAll covers read, write and admin.
func (c Caller) HasScope(scope string) bool {
	if c.Via != "api_key" {
		return true
	}
	return ScopesCover(c.Scopes, scope)
}

// ScopesCover reports whether a key holding held may use scope.
func ScopesCover(held []string, scope string) bool {
	if slices.Contains(held, scope) {
		return true
	}
	if scope == ScopeHealthRead && slices.Contains(held, ScopeRead) {
		return true
	}
	return scope != ScopeControl && scope != ScopeMCP && scope != ScopeCompat && slices.Contains(held, ScopeAll)
}

// MaySeeAudit reports whether the person may read audit events (sign-ins,
// API keys, settings changes): the owner, admins and people with the
// settings permission, and of API keys only admin-scoped ones.
func (c Caller) MaySeeAudit() bool {
	return (c.Has(NeedUsers) || c.Has(PermSettings)) && c.HasScope(ScopeAdmin)
}
