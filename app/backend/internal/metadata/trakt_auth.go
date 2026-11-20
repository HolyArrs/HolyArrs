package metadata

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

// Trakt sign-in with the owner's own Trakt app, by Trakt's device sign-in:
// HolyArrs asks Trakt for a code, the person types it at trakt.tv/activate,
// and HolyArrs asks Trakt until it is approved. A new Trakt app has only a
// client id, and that is enough. An older app's client secret is sent when
// one is saved, never otherwise. The tokens are kept encrypted beside the
// client id; a sign-in lets Outside lists read private lists and
// watchlists, and watch history read what the person watched on Trakt.
const (
	traktSecretName  = "trakt_client_secret"
	traktAccessName  = "trakt_oauth_access"
	traktRefreshName = "trakt_oauth_refresh"
	traktExpiresName = "trakt_oauth_expires" // RFC 3339
	traktUserName    = "trakt_oauth_user"    // the Trakt username
	traktPersonName  = "trakt_oauth_person"  // the id of the person here who signed in
	traktProblemName = "trakt_oauth_problem" // why the sign-in ended, until the next one
)

// traktRedirect is the redirect address Trakt wants on a token refresh.
const traktRedirect = "urn:ietf:wg:oauth:2.0:oob"

// Errors of the Trakt sign-in.
var (
	ErrTraktNoApp     = errors.New("add your Trakt app's client id first")
	ErrTraktNoSignIn  = errors.New("start the Trakt sign-in first")
	ErrTraktSignedOut = errors.New("Trakt ended the sign-in; sign in with Trakt again under Settings > Outside lists")
)

// The answers of a sign-in check.
const (
	TraktWaiting  = "waiting"
	TraktSignedIn = "signed_in"
	TraktDenied   = "denied"
	TraktExpired  = "expired"
)

// TraktAuth keeps the Trakt sign-in. One refresh runs at a time.
type TraktAuth struct {
	Secrets *secrets.Vault
	Base    string // default https://api.trakt.tv
	HTTP    *http.Client
	Now     func() time.Time

	mu      sync.Mutex
	pending *traktDevice
}

// traktDevice is a device sign-in waiting for the person to approve it.
type traktDevice struct {
	code     string
	until    time.Time
	interval time.Duration
	asked    time.Time // the last time Trakt was asked
	person   int64
	by       database.Actor
}

// TraktDeviceCode is what the person types at Trakt to approve the sign-in.
type TraktDeviceCode struct {
	UserCode        string `json:"user_code"`
	VerificationURL string `json:"verification_url"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// TraktStatus is the sign-in as the settings page shows it. It never holds
// a secret.
type TraktStatus struct {
	ClientID     bool       `json:"client_id"`
	ClientSecret bool       `json:"client_secret"`
	SignedIn     bool       `json:"signed_in"`
	Username     string     `json:"username,omitempty"`
	PersonID     int64      `json:"person_id,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	Waiting      bool       `json:"waiting"`
	Problem      string     `json:"problem,omitempty"`
}

func (a *TraktAuth) now() time.Time {
	if a.Now != nil {
		return a.Now().UTC()
	}
	return time.Now().UTC()
}

func (a *TraktAuth) base() string {
	if b := strings.TrimRight(a.Base, "/"); b != "" {
		return b
	}
	return "https://api.trakt.tv"
}

func (a *TraktAuth) setting(ctx context.Context, name string) string {
	if a.Secrets == nil {
		return ""
	}
	v, err := a.Secrets.Setting(ctx, TraktKeyGroup, name)
	if err != nil {
		log.Printf("trakt: reading a setting: %v", err)
		return ""
	}
	return v
}

func (a *TraktAuth) put(ctx context.Context, by database.Actor, kv ...string) error {
	for i := 0; i+1 < len(kv); i += 2 {
		if err := a.Secrets.SetSetting(ctx, TraktKeyGroup, kv[i], kv[i+1], by); err != nil {
			return err
		}
	}
	return nil
}

// app reads the client id and client secret.
func (a *TraktAuth) app(ctx context.Context) (string, string) {
	return a.setting(ctx, TraktKeyName), a.setting(ctx, traktSecretName)
}

// SetSecret saves the Trakt app's client secret.
func (a *TraktAuth) SetSecret(ctx context.Context, secret string, by database.Actor) error {
	return a.put(ctx, by, traktSecretName, secret)
}

// Status says whether the app is set up and who is signed in.
func (a *TraktAuth) Status(ctx context.Context) TraktStatus {
	id, secret := a.app(ctx)
	st := TraktStatus{ClientID: id != "", ClientSecret: secret != "", Problem: a.setting(ctx, traktProblemName)}
	if a.setting(ctx, traktRefreshName) != "" || a.setting(ctx, traktAccessName) != "" {
		st.SignedIn, st.Username = true, a.setting(ctx, traktUserName)
		st.PersonID, _ = strconv.ParseInt(a.setting(ctx, traktPersonName), 10, 64)
		if t, err := time.Parse(time.RFC3339, a.setting(ctx, traktExpiresName)); err == nil {
			st.ExpiresAt = &t
		}
	}
	a.mu.Lock()
	st.Waiting = a.pending != nil && a.now().Before(a.pending.until)
	a.mu.Unlock()
	return st
}

// ClientID is the Trakt key (the app's client id); "" when none is saved.
func (a *TraktAuth) ClientID(ctx context.Context) string {
	if a == nil || a.Secrets == nil {
		return ""
	}
	return a.setting(ctx, TraktKeyName)
}

// Person is the id of the person here who signed in to Trakt; 0 when
// nobody is signed in.
func (a *TraktAuth) Person(ctx context.Context) int64 {
	if a == nil || a.setting(ctx, traktRefreshName) == "" && a.setting(ctx, traktAccessName) == "" {
		return 0
	}
	id, _ := strconv.ParseInt(a.setting(ctx, traktPersonName), 10, 64)
	return id
}

// StartSignIn asks Trakt for a device code. person is who signs in here.
func (a *TraktAuth) StartSignIn(ctx context.Context, person int64, by database.Actor) (TraktDeviceCode, error) {
	id, _ := a.app(ctx)
	if id == "" {
		return TraktDeviceCode{}, ErrTraktNoApp
	}
	var out struct {
		DeviceCode string `json:"device_code"`
		TraktDeviceCode
	}
	status, err := a.post(ctx, "/oauth/device/code", map[string]string{"client_id": id}, &out)
	if err != nil {
		return TraktDeviceCode{}, err
	}
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return TraktDeviceCode{}, errors.New("Trakt refused the client id; check it in Settings")
	case status != http.StatusOK || out.DeviceCode == "" || out.UserCode == "":
		return TraktDeviceCode{}, fmt.Errorf("Trakt answered %d", status)
	}
	if out.Interval < 1 {
		out.Interval = 5
	}
	if out.ExpiresIn < 1 {
		out.ExpiresIn = 600
	}
	a.mu.Lock()
	a.pending = &traktDevice{code: out.DeviceCode, until: a.now().Add(time.Duration(out.ExpiresIn) * time.Second),
		interval: time.Duration(out.Interval) * time.Second, person: person, by: by}
	a.mu.Unlock()
	return out.TraktDeviceCode, nil
}

// CheckSignIn asks Trakt once whether the person approved the device code.
// It asks no more often than Trakt's interval; in between it says waiting.
func (a *TraktAuth) CheckSignIn(ctx context.Context) (string, error) {
	a.mu.Lock()
	d := a.pending
	now := a.now()
	switch {
	case d == nil:
		a.mu.Unlock()
		return "", ErrTraktNoSignIn
	case !now.Before(d.until):
		a.pending = nil
		a.mu.Unlock()
		return TraktExpired, nil
	case now.Sub(d.asked) < d.interval:
		a.mu.Unlock()
		return TraktWaiting, nil
	}
	d.asked = now
	a.mu.Unlock()
	id, secret := a.app(ctx)
	if id == "" {
		return "", ErrTraktNoApp
	}
	var tok traktTokens
	status, err := a.post(ctx, "/oauth/device/token", withSecret(map[string]string{"code": d.code, "client_id": id}, secret), &tok)
	if err != nil {
		return "", err
	}
	end := func(state string) (string, error) {
		a.mu.Lock()
		if a.pending == d {
			a.pending = nil
		}
		a.mu.Unlock()
		return state, nil
	}
	switch status {
	case http.StatusOK:
	case http.StatusBadRequest: // not approved yet
		return TraktWaiting, nil
	case http.StatusTooManyRequests: // slow down
		a.mu.Lock()
		d.interval += time.Second
		a.mu.Unlock()
		return TraktWaiting, nil
	case http.StatusTeapot:
		return end(TraktDenied)
	case http.StatusNotFound, http.StatusConflict, http.StatusGone:
		return end(TraktExpired)
	case http.StatusUnauthorized, http.StatusForbidden:
		end("")
		return "", errors.New("Trakt refused the client id or client secret; check them in Settings")
	default:
		return "", fmt.Errorf("Trakt answered %d", status)
	}
	// The code is used up whatever happens next.
	end("")
	if tok.AccessToken == "" || tok.RefreshToken == "" {
		return "", errors.New("Trakt sent a sign-in without tokens")
	}
	if err := a.signedIn(ctx, id, tok, d.person, d.by); err != nil {
		return "", err
	}
	return TraktSignedIn, nil
}

// signedIn keeps a new sign-in's tokens, its Trakt username and who here
// signed in.
func (a *TraktAuth) signedIn(ctx context.Context, id string, tok traktTokens, person int64, by database.Actor) error {
	if err := a.save(ctx, tok, by); err != nil {
		return err
	}
	name := a.username(ctx, id, tok.AccessToken)
	return a.put(ctx, by, traktUserName, name, traktPersonName, strconv.FormatInt(person, 10), traktProblemName, "")
}

// withSecret adds the app's client secret to a token request when one is
// saved: older Trakt apps need it, new apps have none.
func withSecret(body map[string]string, secret string) map[string]string {
	if secret != "" {
		body["client_secret"] = secret
	}
	return body
}

// traktTokens is Trakt's token answer.
type traktTokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	CreatedAt    int64  `json:"created_at"`
}

func (a *TraktAuth) save(ctx context.Context, tok traktTokens, by database.Actor) error {
	start := a.now()
	if tok.CreatedAt > 0 {
		start = time.Unix(tok.CreatedAt, 0).UTC()
	}
	exp := tok.ExpiresIn
	if exp <= 0 {
		exp = 24 * 3600
	}
	return a.put(ctx, by, traktAccessName, tok.AccessToken, traktRefreshName, tok.RefreshToken,
		traktExpiresName, start.Add(time.Duration(exp)*time.Second).Format(time.RFC3339))
}

// username reads the signed-in Trakt username; "" when Trakt does not say.
func (a *TraktAuth) username(ctx context.Context, id, token string) string {
	var out struct {
		User struct {
			Username string `json:"username"`
		} `json:"user"`
	}
	k := &Trakt{Base: a.Base, ClientID: id, Token: token, HTTP: a.HTTP}
	if _, err := k.get(ctx, "/users/settings", &out); err != nil {
		return ""
	}
	return out.User.Username
}

// SignOut ends the sign-in here and asks Trakt to forget the token.
func (a *TraktAuth) SignOut(ctx context.Context, by database.Actor) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pending = nil
	id, secret := a.app(ctx)
	if tok := a.setting(ctx, traktAccessName); tok != "" && id != "" {
		if _, err := a.post(ctx, "/oauth/revoke", withSecret(map[string]string{"token": tok, "client_id": id}, secret), nil); err != nil {
			log.Printf("trakt: sign-out: %v", err) // the tokens go here all the same
		}
	}
	return a.clear(ctx, by, "")
}

func (a *TraktAuth) clear(ctx context.Context, by database.Actor, problem string) error {
	return a.put(ctx, by, traktAccessName, "", traktRefreshName, "", traktExpiresName, "", traktUserName, "", traktPersonName, "", traktProblemName, problem)
}

// Access returns the client id and a fresh access token, refreshing it
// first when it ends within the hour. The token is "" when nobody is
// signed in. A refresh Trakt refuses ends the sign-in (ErrTraktSignedOut);
// one Trakt does not answer keeps it for the next try.
func (a *TraktAuth) Access(ctx context.Context) (string, string, error) {
	if a == nil || a.Secrets == nil {
		return "", "", nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	id, secret := a.app(ctx)
	access, refresh := a.setting(ctx, traktAccessName), a.setting(ctx, traktRefreshName)
	if refresh == "" && access == "" {
		return id, "", nil
	}
	exp, err := time.Parse(time.RFC3339, a.setting(ctx, traktExpiresName))
	if access != "" && err == nil && a.now().Add(time.Hour).Before(exp) {
		return id, access, nil
	}
	if refresh == "" || id == "" {
		return id, access, nil // nothing to refresh with; Trakt says if it is still good
	}
	// Refresh tokens are good once: the answer's pair replaces both.
	var tok traktTokens
	status, err := a.post(ctx, "/oauth/token", withSecret(map[string]string{"refresh_token": refresh, "client_id": id,
		"redirect_uri": traktRedirect, "grant_type": "refresh_token"}, secret), &tok)
	switch {
	case err != nil:
		return id, "", err
	case status == http.StatusBadRequest || status == http.StatusUnauthorized || status == http.StatusForbidden:
		if err := a.clear(ctx, database.System, ErrTraktSignedOut.Error()); err != nil {
			return id, "", err
		}
		return id, "", ErrTraktSignedOut
	case status != http.StatusOK || tok.AccessToken == "":
		return id, "", fmt.Errorf("Trakt answered %d", status)
	}
	if tok.RefreshToken == "" {
		tok.RefreshToken = refresh
	}
	if err := a.save(ctx, tok, database.System); err != nil {
		return id, "", err
	}
	return id, tok.AccessToken, nil
}

// Refused ends the sign-in when Trakt refused token, the access token in
// use, so the next read does not try it again.
func (a *TraktAuth) Refused(ctx context.Context, token string) {
	if a == nil || token == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.setting(ctx, traktAccessName) != token {
		return // a newer token came in meanwhile
	}
	if err := a.clear(ctx, database.System, ErrTraktSignedOut.Error()); err != nil {
		log.Printf("trakt: ending a refused sign-in: %v", err)
	}
}

// post sends one form to Trakt and decodes a 200 answer into into. It
// returns Trakt's status; err is only for no answer.
func (a *TraktAuth) post(ctx context.Context, path string, body map[string]string, into any) (int, error) {
	op, err := outbound.Begin(ctx, outbound.Target{Kind: "trakt_auth", Identity: []string{a.base(), path}})
	if err != nil {
		return 0, err
	}
	outcome := outbound.Canceled
	defer func() { op.Finish(outcome) }()
	actx, cancel := op.AttemptContext(ctx, 15*time.Second)
	defer cancel()
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(actx, http.MethodPost, a.base()+path, bytes.NewReader(raw))
	if err != nil {
		outcome = outbound.TerminalFailure
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("trakt-api-version", "2")
	if id := body["client_id"]; id != "" {
		req.Header.Set("trakt-api-key", id)
	}
	cl := a.HTTP
	if cl == nil {
		cl = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := cl.Do(req)
	if err != nil {
		outcome = outbound.HTTPOutcome(0, err, ctx)
		return 0, errors.New("Trakt did not answer")
	}
	defer resp.Body.Close()
	// A refusal is an answer, not an outage.
	outcome = outbound.Success
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		outcome = outbound.TransientFailure
	}
	if resp.StatusCode == http.StatusOK && into != nil {
		if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(into); err != nil {
			return 0, errors.New("Trakt sent an answer that could not be read")
		}
	}
	return resp.StatusCode, nil
}

// Read GETs path from Trakt as the signed-in person. Nobody signed in, or
// a token Trakt refuses (the sign-in then ends), is ErrTraktSignedOut.
func (a *TraktAuth) Read(ctx context.Context, path string, into any) error {
	id, token, err := a.Access(ctx)
	if err != nil {
		return err
	}
	if token == "" {
		return ErrTraktSignedOut
	}
	k := &Trakt{Base: a.Base, ClientID: id, Token: token, HTTP: a.HTTP}
	if _, err := k.get(ctx, path, into); errors.Is(err, ErrTraktLocked) {
		a.Refused(ctx, token)
		return ErrTraktSignedOut
	} else if err != nil {
		return err
	}
	return nil
}
