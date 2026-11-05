package auth

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	AuthMethodLegacy               = "legacy"
	AuthMethodLocalPassword string = "local_password"
	AuthMethodLocalSetup           = "local_setup"
	localRecentFor                 = 10 * time.Minute
	// INTENTIONAL (Luis, 2026-11-05, "No check during setup"): first-run setup
	// has no password re-check. The session the owner setup itself starts
	// (setup() and the Plex, Jellyfin and Emby setup routes) counts as a
	// recent local proof for its first hour, so the first-run steps (such as
	// connecting Prowlarr, which makes old-app keys) never ask. Only that
	// setup sign-in: every other sign-in, and the setup one after the hour,
	// is unchanged. A required two-factor enrollment still wins.
	setupRecentFor = 60 * time.Minute
	// setupFamily starts the family id of that setup session. A '.' never
	// comes out of token()'s base64url, so no other family can start so; the
	// id is kept across rotation and replaced by any later sign-in or reauth.
	setupFamily = "setup."
)

// RecentLocal reports whether this request is an authenticated browser
// session with a recent local password proof (or a first-run setup session
// in its first hour, see setupRecentFor) and no unfinished two-factor
// enrollment requirement. Other outside sign-ins and API keys never satisfy it.
func RecentLocal(r *http.Request) bool {
	who, ok := From(r.Context())
	return ok && who.Via == "session" && (who.LocalRecent || who.SetupRecent) && !who.MustTwoFactor
}

// riskyRoutes are the routes a browser session may use only with a local
// password proof from the last localRecentFor. Opening pages and ordinary
// settings never ask (Luis, 2026-11-04): passwords and two-factor, API and
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
	// API keys, AI keys and old-app API keys.
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

// needsRecentLocal says whether a signed-in request must show a recent local
// password proof first. A browser session needs one only on a risky route.
// Every other kind of sign-in without an API key (the trusted-network skip)
// can never show one, so it keeps off every admin and control route.
func needsRecentLocal(pattern string, who Who) bool {
	if who.Via == "api_key" || recentLocalExempt(pattern, who) {
		return false
	}
	if who.Via == "session" {
		return riskyRoutes[pattern]
	}
	sc := RouteScope(pattern)
	return sc == ScopeAdmin || sc == ScopeControl
}

// recentLocalExempt keeps the recovery path open when a browser session has
// no recent local proof. Two-factor setup and enable are open only while an
// administrator is required to enroll, so that policy cannot lock them out.
func recentLocalExempt(pattern string, who Who) bool {
	switch pattern {
	case "GET /api/v1/auth/me", "POST /api/v1/auth/logout", "POST /api/v1/auth/reauth",
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
	// Someone who joined through a media server has no password to confirm;
	// a fresh sign-in there lets them set a first one (changePassword then
	// refuses anyone who already has a password without a local proof).
	case "POST /api/v1/auth/password":
		return who.OutsideRecent
	default:
		return false
	}
}

type reauthBody struct {
	Password string `json:"password"`
	Code     string `json:"code,omitempty"`
}

// reauth replaces the complete current session family after checking the
// local password and, when enrolled, a current two-factor or backup code.
func (a *Auth) reauth(w http.ResponseWriter, r *http.Request) {
	who, ok := From(r.Context())
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
	var name, hash string
	err := a.s.Read().QueryRowContext(r.Context(),
		"SELECT name, password_hash FROM users WHERE id = ? AND state = 'active'", who.UserID).Scan(&name, &hash)
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
	var family string
	err := tx.QueryRow("SELECT family FROM sessions WHERE hash = ?", h).Scan(&family)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if family == "" {
		_, err = tx.Exec("DELETE FROM sessions WHERE hash = ?", h)
		return err
	}
	_, err = tx.Exec("DELETE FROM sessions WHERE family = ?", family)
	return err
}
