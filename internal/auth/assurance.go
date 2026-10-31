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
)

// RecentLocal reports whether this request is an authenticated browser
// session with a recent local password proof and no unfinished two-factor
// enrollment requirement. Outside sign-in and API keys never satisfy it.
func RecentLocal(r *http.Request) bool {
	who, ok := From(r.Context())
	return ok && who.Via == "session" && who.LocalRecent && !who.MustTwoFactor
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
	// but a signed-in manager reading or steering a torrent needs no fresh password.
	case "GET /api/v1/downloads/{id}/swarm", "GET /api/v1/downloads/{id}/detail",
		"GET /api/v1/downloads/{id}/files", "PUT /api/v1/downloads/{id}/files",
		"GET /api/v1/downloaders/{id}/items/{hash}/detail", "GET /api/v1/downloaders/{id}/items/{hash}/swarm",
		"GET /api/v1/downloaders/{id}/items/{hash}/files", "PUT /api/v1/downloaders/{id}/items/{hash}/files":
		return true
	case "POST /api/v1/auth/two-factor/setup", "POST /api/v1/auth/two-factor/enable":
		return who.MustTwoFactor
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
