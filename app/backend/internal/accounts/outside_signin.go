package accounts

import (
	"errors"
	"net/http"
)

// Sign-in through an outside account: Plex (a PIN that the person confirms
// on Plex's own site) and Jellyfin or Emby (name and password checked by the
// owner's server). A first sign-in makes a member when the owner lets them
// join; later sign-ins find the linked user.

// Sign-in setting keys.
const (
	keyRequireAdmin2FA = "require_two_factor_admins"
	keyClientID        = "client_id"
)

// Providers.
const (
	ProviderPlex     = "plex"
	ProviderJellyfin = "jellyfin"
	ProviderEmby     = "emby"
)

// Join rules for people who are not linked yet.
const (
	JoinNobody   = "nobody"   // only people already linked
	JoinList     = "list"     // only the names or emails on the owner's list
	JoinEveryone = "everyone" // everyone the server lets in
)

var errNotAllowed = errors.New("not allowed to join")

var errDisabled = errors.New("account disabled")

var errKidSkipLocal = errors.New("kid join while skipping local sign-in")

func (a *Auth) outsideRoutes(m *http.ServeMux) {
	m.HandleFunc("POST /api/v1/auth/plex/start", a.plexStart)
	m.HandleFunc("POST /api/v1/auth/plex/finish", a.plexFinish)
	m.HandleFunc("POST /api/v1/auth/media-server/{kind}", a.mediaServerLogin)
	a.setupOutsideRoutes(m)
	a.inviteOutsideRoutes(m)
}
