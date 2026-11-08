package accounts

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

const (
	AuthMethodLocalPassword string = "local_password"
	AuthMethodLocalSetup           = "local_setup"
	localRecentFor                 = 10 * time.Minute
	// First-run setup has no password re-check. The session the owner setup
	// itself starts (the password, Plex, Jellyfin and Emby setup routes)
	// counts as a recent local proof until the first-run wizard is finished
	// (POST /api/v1/auth/setup/done), capped at setupRecentFor, so the
	// first-run steps (connecting Prowlarr, Import's Move) never ask. Only
	// that setup sign-in gets this: every other sign-in, and the setup one
	// after the wizard or the cap, is unchanged. A required two-factor
	// enrollment still wins.
	setupRecentFor = 24 * time.Hour
	// settingFirstRunDone (this_app group) records that the wizard finished.
	settingFirstRunDone = "first_run_done"
	// setupFamily starts the family id of that setup session. A '.' never
	// comes out of token()'s base64url, so no other family can start so; the
	// id is kept across rotation and replaced by any later sign-in or reauth.
	setupFamily = "setup."
)

// riskyRoutes are the routes a browser session may use only with a local
// password proof from the last localRecentFor. Opening pages and ordinary
// settings never ask: passwords and two-factor, API and
// AI keys, accounts and roles, backups, single sign-on and Plex links, the
// trusted-network sign-in skip, and answers that carry a secret (the
// backup file, a new key, invite link or backup codes) do.
// Single sign-on and Plex link handlers also check on their own.
var riskyRoutes = map[string]bool{
	// Passwords and two-factor sign-in, mine and other people's.
	"POST /api/v1/auth/password":                true,
	"POST /api/v1/auth/two-factor/setup":        true,
	"POST /api/v1/auth/two-factor/enable":       true,
	"POST /api/v1/auth/two-factor/disable":      true,
	"POST /api/v1/auth/two-factor/backup-codes": true,
	"POST /api/v1/users/{id}/password":          true,
	"POST /api/v1/users/{id}/two-factor/reset":  true,
	// API keys, AI keys and arr API keys.
	"POST /api/v1/auth/keys":                          true,
	"DELETE /api/v1/auth/keys/{id}":                   true,
	"POST /api/v1/auth/ai-connect":                    true,
	"POST /api/v1/settings/old-app-api/keys/{side}":   true,
	"DELETE /api/v1/settings/old-app-api/keys/{side}": true,
	// Accounts: role and state changes, new accounts (an invite, people
	// brought in from a media server or by a Move from the old apps).
	"PUT /api/v1/users/{id}":          true,
	"POST /api/v1/users/invites":      true,
	"POST /api/v1/auth/import/{kind}": true,
	"POST /api/v1/migrate/moves":      true,
	// Backups: the file holds the whole database.
	"GET /api/v1/backups/{name}":    true,
	"DELETE /api/v1/backups/{name}": true,
	"POST /api/v1/backups/restore":  true,
	"POST /api/v1/system/key-kit":   true, // the master key itself
	// Single sign-on, trusted proxy and Plex links.
	"GET /api/v1/auth/sso":                  true,
	"PUT /api/v1/auth/sso":                  true,
	"POST /api/v1/auth/sso/test":            true,
	"POST /api/v1/auth/sso/unlink":          true,
	"POST /api/v1/users/{id}/sso/unlink":    true,
	"POST /api/v1/auth/oidc/link":           true,
	"POST /api/v1/auth/oidc/link-finish":    true,
	"POST /api/v1/auth/forward/link":        true,
	"POST /api/v1/auth/plex/link/start":     true,
	"POST /api/v1/auth/plex/link/finish":    true,
	"DELETE /api/v1/auth/plex/link":         true,
	"POST /api/v1/auth/plex/my-link/start":  true,
	"POST /api/v1/auth/plex/my-link/finish": true,
	"DELETE /api/v1/auth/plex/my-link":      true,
	// Sign-in settings, the trusted-network skip among them.
	"PUT /api/v1/auth/settings": true,
}

type reauthBody struct {
	Password string `json:"password"`
	Code     string `json:"code,omitempty"`
}

// RecentLocal reports whether this request is an authenticated browser
// session with a recent local password proof (or a first-run setup session
// before the wizard is done, see setupRecentFor) and no unfinished two-factor
// enrollment requirement. Other outside sign-ins and API keys never satisfy it.
func RecentLocal(r *http.Request) bool {
	who, ok := webserver.From(r.Context())
	return ok && who.Via == "session" && (who.LocalRecent || who.SetupRecent) && !who.MustTwoFactor
}

// needsRecentLocal says whether a signed-in request must show a recent local
// password proof first. A browser session needs one only on a risky route.
// Every other kind of sign-in without an API key (the trusted-network skip)
// can never show one, so it keeps off every admin and control route.
func needsRecentLocal(pattern string, who webserver.Caller) bool {
	if who.Via == "api_key" || recentLocalExempt(pattern, who) {
		return false
	}
	if who.Via == "session" {
		return riskyRoutes[pattern]
	}
	sc := RouteScope(pattern)
	return sc == webserver.ScopeAdmin || sc == webserver.ScopeControl
}

// recentLocalExempt keeps the recovery path open when a browser session has
// no recent local proof. Two-factor setup and enable are open only while an
// administrator is required to enroll, so that policy cannot lock them out.
func recentLocalExempt(pattern string, who webserver.Caller) bool {
	switch pattern {
	case "GET /api/v1/auth/me", "POST /api/v1/auth/logout", "POST /api/v1/auth/reauth", "POST /api/v1/auth/setup/done",
		"GET /api/v1/auth/sessions", "DELETE /api/v1/auth/sessions", "DELETE /api/v1/auth/sessions/{sid}":
		return true
	// The Downloads detail panel: admin scope keeps them off read-only API keys,
	// but a signed-in manager reading a torrent needs no fresh password.
	case "GET /api/v1/downloads/{id}/swarm", "GET /api/v1/downloads/{id}/detail",
		"GET /api/v1/downloads/{id}/files",
		"GET /api/v1/downloaders/{id}/items/{hash}/detail", "GET /api/v1/downloaders/{id}/items/{hash}/swarm",
		"GET /api/v1/downloaders/{id}/items/{hash}/files",
		"GET /api/v1/downloaders/{id}/items/{hash}/usenet", "GET /api/v1/downloads/{id}/usenet":
		return true
	case "POST /api/v1/auth/two-factor/setup", "POST /api/v1/auth/two-factor/enable":
		return who.MustTwoFactor
	// Someone who joined through a media server has no password to confirm,
	// so their own sign-in may set a first one. changePassword still asks
	// for a local proof and the current password when one already exists.
	case "POST /api/v1/auth/password":
		return true
	default:
		return false
	}
}

// reauth replaces the complete current session family after checking the
// local password and, when enrolled, a current two-factor or backup code.
func (a *Auth) reauth(w http.ResponseWriter, r *http.Request) {
	who, ok := webserver.From(r.Context())
	if !ok || who.Via != "session" {
		fail(w, http.StatusForbidden, "sign in in the browser before confirming your password")
		return
	}
	var body reauthBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, `send {"password":"...","code":"..."}`)
		return
	}
	key := "reauth:" + itoa(who.UserID)
	a.mu.Lock()
	wait := a.waitFor(key, freeTries)
	if wait == 0 {
		a.noteFail(key)
	}
	a.mu.Unlock()
	if wait > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		fail(w, http.StatusTooManyRequests, "too many failed confirmations; wait a little and try again")
		return
	}

	select {
	case a.checks <- struct{}{}:
		defer func() { <-a.checks }()
	case <-r.Context().Done():
		return
	}
	name, hash, err := reauthRow(r.Context(), a.s.Read(), who.UserID)
	if err != nil || hash == "" {
		hash = a.dummy
		err = errors.Join(err, errNoLocalSignIn)
	}
	if !CheckPassword(hash, body.Password) || err != nil {
		a.failedEvent(r, who.Name, "password check")
		fail(w, http.StatusUnauthorized, "the password or code is wrong")
		return
	}
	if who.TwoFactor {
		if strings.TrimSpace(body.Code) == "" {
			fail(w, http.StatusUnauthorized, "the password or code is wrong")
			return
		}
		good, err := a.checkCode(r.Context(), who.UserID, body.Code)
		if err != nil {
			log.Printf("auth: reauthentication two-factor: %v", err)
		}
		if !good {
			a.failedEvent(r, who.Name, "two-factor check")
			fail(w, http.StatusUnauthorized, "the password or code is wrong")
			return
		}
	}
	a.mu.Lock()
	delete(a.fails, key)
	a.mu.Unlock()
	a.startSessionAs(w, r, who.UserID, name, http.StatusOK, AuthMethodLocalPassword)
}

func deletePresentedFamily(tx *sql.Tx, rawCookie string) error {
	if rawCookie == "" {
		return nil
	}
	h := sum(rawCookie)
	family, err := sessionFamily(context.Background(), tx, h)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if family == "" {
		return deleteSession(context.Background(), tx, h)
	}
	return deleteSessionFamily(context.Background(), tx, family)
}

// setupDone records that the first-run wizard is finished and ends the setup
// mark of the caller's session: its family loses the setup prefix, so the
// session (and its rotations) is an ordinary one from here on.
func (a *Auth) setupDone(w http.ResponseWriter, r *http.Request) {
	who, ok := webserver.From(r.Context())
	if !ok || who.Via != "session" {
		fail(w, http.StatusForbidden, "sign in in the browser first")
		return
	}
	if who.Role != webserver.RoleOwner {
		fail(w, http.StatusForbidden, "only the owner finishes first-run setup")
		return
	}
	ck, err := r.Cookie(CookieName)
	if err != nil {
		fail(w, http.StatusForbidden, "sign in in the browser first")
		return
	}
	now := a.now()
	err = a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
		if err := database.PutSetting(tx, settingsGroup, settingFirstRunDone, "1", domain.Actor{Kind: "user", ID: who.UserID}, now); err != nil {
			return err
		}
		return markSetupDone(r.Context(), tx, len(setupFamily)+1, sum(ck.Value))
	})
	if err != nil {
		log.Printf("auth: setup done: %v", err)
		fail(w, http.StatusInternalServerError, "something went wrong; see the log")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"done": true})
}
