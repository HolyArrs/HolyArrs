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
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// Single sign-on (OpenID Connect) set up from Settings, and unlinking
// single sign-on and trusted proxy identities.
//
// - When HOLYARRS_OIDC_ENABLED is present at all, the environment owns
//   single sign-on: Settings shows it and refuses changes.
// - Trusted proxy sign-in stays environment-only. Editing its trusted
//   addresses from a browser would let a stolen session add its own address
//   and send identity headers for anyone, the owner included.
// - Saving asks for the owner's password (and code) in the request itself.
// - The client secret is never shown and never follows a new issuer or
//   client id: changing either needs the secret typed again.
// - Changing the issuer or client id removes the links made through the
//   old one and ends the sessions single sign-on made, so a new client
//   never signs in as people the old one let in.

const (
	ssoKeyOn       = "oidc_on"
	ssoKeyIssuer   = "oidc_issuer"
	ssoKeyClient   = "oidc_client_id"
	ssoKeyRedirect = "oidc_redirect_url"
	ssoKeyJoin     = "oidc_join"
	ssoKeyVaultRef = "oidc_vault_ref" // the client secret's vault id; "" is a public client
)

var errNoWayIn = errors.New("no other way in")

// savedOIDC is single sign-on as saved in Settings.
type savedOIDC struct {
	On          bool
	Issuer      string
	ClientID    string
	RedirectURL string
	AllowJoin   bool
	SecretID    int64
}

// SSOView is the single sign-on setup as Settings shows it. Never a secret.
type SSOView struct {
	Source      string        `json:"source"` // "settings" or "environment"
	On          bool          `json:"on"`
	Running     bool          `json:"running"`
	Issuer      string        `json:"issuer"`
	ClientID    string        `json:"client_id"`
	SecretSet   bool          `json:"secret_set"`
	RedirectURL string        `json:"redirect_url"`
	Suggested   string        `json:"suggested_redirect_url"`
	AllowJoin   bool          `json:"allow_join"`
	Links       int           `json:"links"` // people linked through this issuer
	LocalOK     bool          `json:"local_network_allowed"`
	Forward     ForwardStatus `json:"forward"`
}

// ForwardStatus is trusted proxy sign-in, read-only: it is set only in the environment.
type ForwardStatus struct {
	On        bool   `json:"on"`
	Issuer    string `json:"issuer,omitempty"`
	AllowJoin bool   `json:"allow_join"`
	Links     int    `json:"links"`
}

type ssoSaveBody struct {
	Password        string `json:"password"`
	Code            string `json:"code"`
	On              bool   `json:"on"`
	Issuer          string `json:"issuer"`
	ClientID        string `json:"client_id"`
	ClientSecret    string `json:"client_secret"` // blank keeps the saved one, only while issuer and client id stay
	PublicClient    bool   `json:"public_client"` // no secret at all
	RedirectURL     string `json:"redirect_url"`  // keeps the saved callback when it is the same; anything else takes this browser's address
	AllowJoin       bool   `json:"allow_join"`
	ConfirmEveryone bool   `json:"confirm_everyone"`
}

// SSOLinkView is one single sign-on or trusted proxy identity linked to a person.
type SSOLinkView struct {
	Kind      string `json:"kind"` // "oidc" or "forward_auth"
	Issuer    string `json:"issuer"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

type unlinkBody struct {
	Kind    string `json:"kind"`
	Issuer  string `json:"issuer"`
	NoWayIn bool   `json:"confirm_no_way_in"` // the owner confirms a person keeps no way to sign in
}

func (a *Auth) savedOIDCNow(ctx context.Context) savedOIDC {
	id, _ := strconv.ParseInt(a.signInSetting(ctx, ssoKeyVaultRef), 10, 64)
	return savedOIDC{
		On:          a.signInSetting(ctx, ssoKeyOn) == "true",
		Issuer:      a.signInSetting(ctx, ssoKeyIssuer),
		ClientID:    a.signInSetting(ctx, ssoKeyClient),
		RedirectURL: a.signInSetting(ctx, ssoKeyRedirect),
		AllowJoin:   a.signInSetting(ctx, ssoKeyJoin) == "true",
		SecretID:    id,
	}
}

func (a *Auth) oidcOptions(s savedOIDC, secret string) OIDCOptions {
	return OIDCOptions{Issuer: s.Issuer, ClientID: s.ClientID, ClientSecret: secret, RedirectURL: s.RedirectURL,
		AllowJoin: s.AllowJoin, AllowLocal: a.ssoOpt.SavedLimits.AllowLocal, AllowLoopbackHTTP: a.ssoOpt.SavedLimits.AllowLoopbackHTTP}
}

// loadSavedOIDC starts single sign-on from Settings at startup. A setup that
// no longer works turns off only single sign-on, with a log line.
func (a *Auth) loadSavedOIDC(ctx context.Context) {
	s := a.savedOIDCNow(ctx)
	if !s.On {
		return
	}
	secret := ""
	if s.SecretID != 0 {
		if a.vault == nil {
			log.Print("auth: saved single sign-on needs the master key; single sign-on is off")
			return
		}
		var err error
		if secret, err = a.vault.Get(ctx, s.SecretID); err != nil {
			log.Print("auth: saved single sign-on secret cannot be read; single sign-on is off")
			return
		}
	}
	o, err := newOIDCRuntime(a.oidcOptions(s, secret))
	if err != nil {
		log.Print("auth: saved single sign-on is not valid any more; single sign-on is off")
		return
	}
	a.swapOIDC(o)
}

func (a *Auth) ssoSettingsRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /api/v1/auth/sso", a.getSSO)
	m.HandleFunc("PUT /api/v1/auth/sso", a.putSSO)
	m.HandleFunc("POST /api/v1/auth/sso/test", a.testSSO)
	m.HandleFunc("GET /api/v1/auth/sso/links", a.mySSOLinks)
	m.HandleFunc("POST /api/v1/auth/sso/unlink", a.unlinkMySSO)
	m.HandleFunc("POST /api/v1/users/{id}/sso/unlink", a.unlinkUserSSO)
}

// ssoOwner lets only the owner, in a browser session with a recent local
// password proof, reach the single sign-on setup. API keys never do.
func ssoOwner(w http.ResponseWriter, r *http.Request) (webserver.Caller, bool) {
	who, ok := webserver.From(r.Context())
	if !ok || who.Role != webserver.RoleOwner {
		fail(w, http.StatusForbidden, "only the owner sets up single sign-on")
		return webserver.Caller{}, false
	}
	if !RecentLocal(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "confirm your local password again before using this route", "code": "reauth"})
		return webserver.Caller{}, false
	}
	return who, true
}

// suggestedRedirect is the callback address as this browser reaches
// HolyArrs; the owner registers it at the provider.
func (a *Auth) suggestedRedirect(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || a.behindHTTPS(r) {
		scheme = "https"
	}
	return scheme + "://" + r.Host + webserver.URLBaseFrom(r.Context()) + "/api/v1/auth/oidc/callback"
}

func (a *Auth) ssoView(r *http.Request) SSOView {
	ctx := r.Context()
	now := a.ssoNow()
	v := SSOView{Source: "settings", Suggested: a.suggestedRedirect(r), LocalOK: a.ssoOpt.SavedLimits.AllowLocal, Running: now.oidc != nil}
	if a.ssoOpt.OIDCFromEnv {
		v.Source = "environment"
		if now.oidc != nil {
			o := now.oidc.opt
			v.On, v.Issuer, v.ClientID, v.SecretSet, v.RedirectURL, v.AllowJoin = true, o.Issuer, o.ClientID, o.ClientSecret != "", o.RedirectURL, o.AllowJoin
			v.LocalOK = o.AllowLocal
		}
	} else {
		s := a.savedOIDCNow(ctx)
		v.On, v.Issuer, v.ClientID, v.SecretSet, v.RedirectURL, v.AllowJoin = s.On, s.Issuer, s.ClientID, s.SecretID != 0, s.RedirectURL, s.AllowJoin
	}
	if v.Issuer != "" {
		v.Links, _ = countOIDCLinks(ctx, a.s.Read(), v.Issuer)
	}
	if now.forward != nil {
		v.Forward = ForwardStatus{On: true, Issuer: now.forward.opt.Issuer, AllowJoin: now.forward.opt.AllowJoin}
		v.Forward.Links, _ = countForwardLinks(ctx, a.s.Read(), v.Forward.Issuer)
	}
	return v
}

func (a *Auth) getSSO(w http.ResponseWriter, r *http.Request) {
	if _, ok := ssoOwner(w, r); !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, a.ssoView(r))
}

// checkOwnerProof checks the password (and two-factor code) sent with a
// single sign-on change, with the same back-off as confirming a sign-in.
func (a *Auth) checkOwnerProof(w http.ResponseWriter, r *http.Request, who webserver.Caller, password, code string) bool {
	key := "sso-proof:" + itoa(who.UserID)
	a.mu.Lock()
	wait := a.waitFor(key, freeTries)
	if wait == 0 {
		a.noteFail(key)
	}
	a.mu.Unlock()
	if wait > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		fail(w, http.StatusTooManyRequests, "too many wrong passwords; wait a little and try again")
		return false
	}
	hash, err := activePasswordHash(r.Context(), a.s.Read(), who.UserID)
	if err != nil || hash == "" {
		hash, err = a.dummy, errors.Join(err, errNoLocalSignIn)
	}
	if !CheckPassword(hash, password) || err != nil {
		a.failedEvent(r, who.Name, "password check")
		wrongProof(w)
		return false
	}
	if who.TwoFactor {
		good, err := a.checkCode(r.Context(), who.UserID, code)
		if err != nil {
			log.Printf("auth: single sign-on change two-factor: %v", err)
		}
		if strings.TrimSpace(code) == "" || !good {
			a.failedEvent(r, who.Name, "two-factor check")
			wrongProof(w)
			return false
		}
	}
	a.mu.Lock()
	delete(a.fails, key)
	a.mu.Unlock()
	return true
}

// wrongProof answers a wrong password or code with 403, not 401: the web app
// reads 401 as a signed-out session and would leave the page.
func wrongProof(w http.ResponseWriter) {
	writeJSON(w, http.StatusForbidden, map[string]string{"error": "the password or code is wrong", "code": "wrong_proof"})
}

// sameIssuer compares issuers as typed, ignoring a trailing slash, so
// retyping the same one does not count as a new provider.
func sameIssuer(a, b string) bool { return strings.TrimRight(a, "/") == strings.TrimRight(b, "/") }

func (a *Auth) putSSO(w http.ResponseWriter, r *http.Request) {
	who, ok := ssoOwner(w, r)
	if !ok {
		return
	}
	if a.ssoOpt.OIDCFromEnv {
		fail(w, http.StatusConflict, "single sign-on is set in the environment (HOLYARRS_OIDC_ENABLED); change it there")
		return
	}
	if a.vault == nil {
		fail(w, http.StatusServiceUnavailable, "single sign-on needs the master key; see the log")
		return
	}
	var body ssoSaveBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, `send {"password", "on", "issuer", "client_id", "client_secret", "allow_join"}`)
		return
	}
	if !a.checkOwnerProof(w, r, who, body.Password, body.Code) {
		return
	}
	ctx := r.Context()
	cur := a.savedOIDCNow(ctx)
	// The callback is never typed: it is the saved one, or this browser's
	// address for HolyArrs, so a session cannot send codes to another host.
	next := savedOIDC{On: body.On, Issuer: strings.TrimSpace(body.Issuer), ClientID: strings.TrimSpace(body.ClientID),
		RedirectURL: a.suggestedRedirect(r), AllowJoin: body.AllowJoin, SecretID: cur.SecretID}
	if cur.RedirectURL != "" && strings.TrimSpace(body.RedirectURL) == cur.RedirectURL {
		next.RedirectURL = cur.RedirectURL
	}
	if cur.Issuer != "" && sameIssuer(next.Issuer, cur.Issuer) {
		next.Issuer = cur.Issuer // keep links working when only a slash differs
	}
	// A saved secret with no issuer yet counts too: it never goes to whatever issuer comes next.
	changed := (cur.Issuer != "" || cur.SecretID != 0) && (next.Issuer != cur.Issuer || next.ClientID != cur.ClientID)
	newSecret := body.ClientSecret
	if len(newSecret) > 4096 {
		fail(w, http.StatusBadRequest, "the client secret is too long")
		return
	}
	switch {
	case newSecret != "":
	case body.PublicClient:
		next.SecretID = 0
	case changed && cur.SecretID != 0:
		// Never send the saved secret to a new provider or client.
		fail(w, http.StatusBadRequest, "type the client secret again: a saved secret never goes to a new issuer or client id")
		return
	}
	if next.On || next.Issuer != "" {
		opt := a.oidcOptions(next, newSecret)
		if opt.ClientSecret == "" && next.SecretID != 0 {
			opt.ClientSecret = "kept" // only the shape is checked here
		}
		if _, _, err := validateOIDCOptions(opt); err != nil {
			fail(w, http.StatusBadRequest, "check the issuer, client id and callback address: the issuer and callback must be https:// addresses, and the callback must end in /api/v1/auth/oidc/callback")
			return
		}
		if !strings.HasSuffix(next.RedirectURL, webserver.URLBaseFrom(ctx)+"/api/v1/auth/oidc/callback") {
			fail(w, http.StatusBadRequest, "the callback address does not match this HolyArrs's URL base")
			return
		}
	}
	if next.On && next.AllowJoin && !(cur.On && cur.AllowJoin && !changed) && !body.ConfirmEveryone {
		fail(w, http.StatusConflict, "letting everyone this provider signs in join needs confirming: send confirm_everyone")
		return
	}
	// Turning it off or moving to another provider ends what single sign-on started.
	endAll := changed || (cur.On && !next.On)
	// No single sign-on finish runs while this changes (see ssoMu), and when
	// it ends, the running setup stops before the purge, not after.
	a.ssoMu.Lock()
	defer a.ssoMu.Unlock()
	if endAll {
		a.swapOIDC(nil)
	}
	var linksGone, sessionsGone int64
	by := webserver.Actor(ctx)
	err := a.s.WriteCore(ctx, func(tx *sql.Tx) error {
		if newSecret != "" || body.PublicClient {
			if cur.SecretID != 0 {
				if err := a.vault.DeleteTx(ctx, tx, cur.SecretID); err != nil {
					return err
				}
			}
			next.SecretID = 0
			if newSecret != "" {
				id, err := a.vault.PutTx(tx, "oidc_client_secret", newSecret, by)
				if err != nil {
					return err
				}
				next.SecretID = id
			}
		}
		secretRef := ""
		if next.SecretID != 0 {
			secretRef = strconv.FormatInt(next.SecretID, 10)
		}
		for k, v := range map[string]string{ssoKeyOn: strconv.FormatBool(next.On), ssoKeyIssuer: next.Issuer, ssoKeyClient: next.ClientID,
			ssoKeyRedirect: next.RedirectURL, ssoKeyJoin: strconv.FormatBool(next.AllowJoin), ssoKeyVaultRef: secretRef} {
			if err := database.PutSetting(tx, settingsGroup, signInPrefix+k, v, by, a.now()); err != nil {
				return err
			}
		}
		if changed {
			var err error
			if linksGone, err = deleteOIDCLinks(ctx, tx, cur.Issuer); err != nil {
				return err
			}
		}
		if endAll {
			var err error
			if sessionsGone, err = deleteOIDCSessions(ctx, tx); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		serverError(w, "single sign-on settings", err)
		return
	}
	if endAll {
		a.dropTickets(func(t *ticket) bool { return t.method == "oidc" })
	}
	// The running setup is built only from what was saved.
	if !next.On {
		a.swapOIDC(nil)
	} else {
		secret := newSecret // just written: no need to read it back
		if secret == "" && next.SecretID != 0 {
			if secret, err = a.vault.Get(context.WithoutCancel(ctx), next.SecretID); err != nil {
				// Never leave the old setup running against what was saved.
				a.swapOIDC(nil)
				serverError(w, "single sign-on secret", err)
				return
			}
		}
		o, err := newOIDCRuntime(a.oidcOptions(next, secret))
		if err != nil {
			a.swapOIDC(nil)
			fail(w, http.StatusBadRequest, "single sign-on was saved but could not start; check the settings")
			return
		}
		a.swapOIDC(o)
	}
	a.event(r, by, "settings_changed", byName(r)+" changed single sign-on", map[string]any{"area": "sign-in", "action": "changed", "what": "single sign-on",
		"on": next.On, "issuer": next.Issuer, "old_issuer": cur.Issuer, "client_id": next.ClientID, "old_client_id": cur.ClientID,
		"allow_join": next.AllowJoin, "links_removed": linksGone, "sessions_ended": sessionsGone})
	log.Printf("auth: single sign-on changed by user %d (on %t, links removed %d, sessions ended %d)", who.UserID, next.On, linksGone, sessionsGone)
	writeJSON(w, http.StatusOK, map[string]any{"sso": a.ssoView(r), "links_removed": linksGone, "sessions_ended": sessionsGone})
}

// testSSO checks that an issuer answers like an OpenID Connect provider. It
// never sends a secret, answers only "ok" or one plain failure, and uses a
// test identity of its own so failed tests cannot slow the live provider.
func (a *Auth) testSSO(w http.ResponseWriter, r *http.Request) {
	who, ok := ssoOwner(w, r)
	if !ok {
		return
	}
	var body struct {
		Issuer string `json:"issuer"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, `send {"issuer": "https://..."}`)
		return
	}
	key := "sso-test:" + itoa(who.UserID)
	a.mu.Lock()
	wait := a.waitFor(key, freeTries)
	if wait == 0 {
		a.noteFail(key) // every test counts: a few a minute is plenty
	}
	a.mu.Unlock()
	if wait > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		fail(w, http.StatusTooManyRequests, "too many tests; wait a little and try again")
		return
	}
	opt := a.oidcOptions(savedOIDC{Issuer: strings.TrimSpace(body.Issuer), ClientID: "holyarrs-settings-test", RedirectURL: a.suggestedRedirect(r)}, "")
	o, err := newOIDCRuntime(opt)
	if err != nil {
		fail(w, http.StatusBadRequest, "the issuer must be an https:// address")
		return
	}
	defer o.close()
	tctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	if err := o.ensureProvider(tctx); err != nil {
		fail(w, http.StatusBadGateway, "no OpenID Connect provider answered at this issuer")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *Auth) ssoLinksOf(ctx context.Context, userID int64) []SSOLinkView {
	out, err := ssoLinksOfUser(ctx, a.s.Read(), userID)
	if err != nil {
		return []SSOLinkView{}
	}
	return out
}

func (a *Auth) mySSOLinks(w http.ResponseWriter, r *http.Request) {
	who, ok := webserver.From(r.Context())
	if !ok || who.UserID == 0 {
		fail(w, http.StatusForbidden, "sign in first")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"links": a.ssoLinksOf(r.Context(), who.UserID)})
}

// unlinkSSO removes one identity from userID and, in the same write, ends
// the sessions it could have made; pending two-factor steps and finished
// callbacks go too. self says the person unlinks their own identity, which
// needs a HolyArrs password to stay; the owner may unlink anyone's after
// confirming when it is their only way in.
func (a *Auth) unlinkSSO(w http.ResponseWriter, r *http.Request, userID int64, self bool, body unlinkBody) {
	if body.Kind != "oidc" && body.Kind != "forward_auth" || body.Issuer == "" {
		fail(w, http.StatusBadRequest, `send {"kind": "oidc" or "forward_auth", "issuer": "..."}`)
		return
	}
	var ended int64
	a.ssoMu.Lock() // no finish runs meanwhile: see ssoMu
	defer a.ssoMu.Unlock()
	err := a.s.WriteCore(r.Context(), func(tx *sql.Tx) error {
		hash, err := passwordHashOf(r.Context(), tx, userID)
		if err != nil {
			return err
		}
		others, err := waysInBesidesSSO(r.Context(), tx, userID, body.Kind, body.Issuer)
		if err != nil {
			return err
		}
		if hash == "" && others == 0 && (self || !body.NoWayIn) {
			return errNoWayIn
		}
		n, err := deleteSSOLink(r.Context(), tx, userID, body.Kind, body.Issuer)
		if err != nil {
			return err
		}
		if n == 0 {
			return sql.ErrNoRows
		}
		ended, err = deleteSessionsByMethod(r.Context(), tx, userID, body.Kind)
		return err
	})
	switch {
	case errors.Is(err, sql.ErrNoRows):
		fail(w, http.StatusNotFound, "no such linked sign-in")
		return
	case errors.Is(err, errNoWayIn) && self:
		fail(w, http.StatusConflict, "this is your only way to sign in; set a HolyArrs password first")
		return
	case errors.Is(err, errNoWayIn):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "this is their only way to sign in; confirm to leave them with none", "code": "no_way_in"})
		return
	case err != nil:
		serverError(w, "unlink", err)
		return
	}
	a.dropTickets(func(t *ticket) bool { return t.userID == userID && t.method == body.Kind })
	if body.Kind == "oidc" {
		a.ssoNow().oidc.forget()
	}
	word := map[string]string{"oidc": "single sign-on", "forward_auth": "trusted proxy"}[body.Kind]
	summary := byName(r) + " unlinked their " + word + " sign-in"
	if !self {
		summary = byName(r) + " unlinked the " + word + " sign-in of " + a.userName(r.Context(), userID)
	}
	a.accountEvent(r, userID, "sign_in_unlinked", summary, map[string]any{"provider": body.Kind, "issuer": body.Issuer, "sessions_ended": ended})
	w.WriteHeader(http.StatusNoContent)
}

func (a *Auth) unlinkMySSO(w http.ResponseWriter, r *http.Request) {
	who, ok := webserver.From(r.Context())
	if !ok || !RecentLocal(r) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "confirm your local password again before using this route", "code": "reauth"})
		return
	}
	var body unlinkBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, `send {"kind": "oidc" or "forward_auth", "issuer": "..."}`)
		return
	}
	a.unlinkSSO(w, r, who.UserID, true, body)
}

func (a *Auth) unlinkUserSSO(w http.ResponseWriter, r *http.Request) {
	who, ok := ssoOwner(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		fail(w, http.StatusBadRequest, "bad user id")
		return
	}
	var body unlinkBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, `send {"kind": "oidc" or "forward_auth", "issuer": "..."}`)
		return
	}
	a.unlinkSSO(w, r, id, id == who.UserID, body)
}

// dropTickets forgets pending two-factor steps that match.
func (a *Auth) dropTickets(match func(*ticket) bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for k, t := range a.tickets {
		if match(t) {
			delete(a.tickets, k)
		}
	}
}
