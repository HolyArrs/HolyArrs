package accounts

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

const (
	pinLife    = 10 * time.Minute
	maxPins    = 1000
	pinsPerIP  = 5
	outsideMax = 1 << 20 // bytes read from an outside answer
	product    = "HolyArrs"
)

// errPlexServers says Plex did not list a person's servers.
var errPlexServers = errors.New("plex did not list the servers")

const (
	pinOwner = "owner"
	pinMine  = "mine"
)

// pinCheckGap is the shortest time between two checks of one pin.
const pinCheckGap = time.Second

// pin is a Plex sign-in in progress.
type pin struct {
	ip string
	// alt is the other header's key when the proxy headers disagreed, so
	// rotating a forged header still counts toward the same caller.
	alt string
	// secret is also in a cookie on the browser that started the pin, so
	// another visitor behind the same address cannot finish it.
	secret  string
	expires time.Time
	// checked is the last time Plex was asked about this pin; checks
	// closer than pinCheckGap answer 429 (the page polls every 2 s).
	checked time.Time
	// purpose is "" for a sign-in, pinOwner for the owner linking in Settings,
	// pinMine for anyone linking their own Plex account from My account.
	purpose string
	userID  int64 // who started a pinMine pin; only they may finish it
}

// plexMe is the Plex account a confirmed PIN belongs to.
type plexMe struct {
	ID       int64  `json:"id"`
	UUID     string `json:"uuid"`
	Username string `json:"username"`
	Email    string `json:"email"`
	// A managed Plex Home user with limits on what they may watch.
	Restricted bool `json:"restricted"`
}

func (a *Auth) plexHeaders(ctx context.Context) map[string]string {
	return map[string]string{"X-Plex-Product": product, "X-Plex-Client-Identifier": a.clientID(ctx)}
}

func (a *Auth) plexStart(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !a.way(ctx, ProviderPlex).On {
		fail(w, http.StatusNotFound, "sign-in with Plex is off")
		return
	}
	a.startPin(w, r, "")
}

// startPin asks Plex for a PIN and answers with the page to confirm it on.
func (a *Auth) startPin(w http.ResponseWriter, r *http.Request, purpose string) {
	ctx := r.Context()
	ip, alt, _ := a.clientKeys(r)
	now := a.now()
	a.mu.Lock()
	mine := 0
	for id, p := range a.pins {
		if now.After(p.expires) {
			delete(a.pins, id)
		} else if p.ip == ip || (alt != "" && p.alt == alt) {
			mine++
		}
	}
	busy := mine >= pinsPerIP || len(a.pins) >= maxPins
	a.mu.Unlock()
	if busy {
		fail(w, http.StatusTooManyRequests, "too many Plex sign-ins started; wait a few minutes")
		return
	}
	var got struct {
		ID   int64  `json:"id"`
		Code string `json:"code"`
	}
	code, err := a.fetch(ctx, http.MethodPost, a.plexURL+"/api/v2/pins?strong=true", a.plexHeaders(ctx), nil, &got)
	if err != nil || code/100 != 2 || got.ID == 0 || got.Code == "" {
		log.Printf("auth: plex pin: status %d: %v", code, err)
		fail(w, http.StatusBadGateway, "Plex did not answer; try again in a moment")
		return
	}
	secret := token(24)
	a.mu.Lock()
	p := &pin{ip: ip, alt: alt, secret: secret, expires: now.Add(pinLife), purpose: purpose}
	if purpose == pinMine {
		p.userID = webserver.Actor(ctx).ID
	}
	a.pins[got.ID] = p
	a.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: pinCookie(got.ID), Value: secret, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: a.behindHTTPS(r), MaxAge: int(pinLife / time.Second)})
	q := url.Values{"clientID": {a.clientID(ctx)}, "code": {got.Code}, "context[device][product]": {product}}
	writeJSON(w, http.StatusOK, map[string]any{"id": got.ID, "auth_url": a.plexApp + "/auth#?" + q.Encode()})
}

func (a *Auth) plexFinish(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	v := a.way(ctx, ProviderPlex)
	if !v.On {
		fail(w, http.StatusNotFound, "sign-in with Plex is off")
		return
	}
	tok, ok := a.checkPin(w, r, "")
	if !ok {
		return
	}
	user, hdr, ok := a.plexUser(ctx, w, tok)
	if !ok {
		return
	}
	extID := strconv.FormatInt(user.ID, 10)
	mayJoin := allowed(v, user.Username, user.Email)
	// shares says whether the person shares the owner's server.
	shares := func() (bool, error) {
		var res []struct {
			ClientIdentifier string `json:"clientIdentifier"`
			Provides         string `json:"provides"`
			Owned            bool   `json:"owned"`
		}
		code, err := a.fetch(ctx, http.MethodGet, a.plexURL+"/api/v2/resources?includeHttps=1", hdr, nil, &res)
		if err != nil || code/100 != 2 {
			log.Printf("auth: plex resources: status %d: %v", code, err)
			return false, errPlexServers
		}
		return slices.ContainsFunc(res, func(x struct {
			ClientIdentifier string `json:"clientIdentifier"`
			Provides         string `json:"provides"`
			Owned            bool   `json:"owned"`
		}) bool {
			// A server the joiner owns is their own, not a share: anyone
			// can copy the owner's server id onto a server they run.
			return x.ClientIdentifier == v.ServerID && !x.Owned
		}), nil
	}
	if mayJoin && v.ServerID != "" {
		// Only people who share the owner's server may join.
		var err error
		if mayJoin, err = shares(); err != nil {
			fail(w, http.StatusBadGateway, "Plex did not list your servers; try again")
			return
		}
	}
	// An imported person's first sign-in passes the same server check; with
	// no server picked, the join rule decides as for anyone new.
	onServer := func() (bool, error) { return mayJoin, nil }
	if v.ServerID != "" {
		onServer = shares
	}
	id, name, err := a.linkedUser(ctx, ProviderPlex, extID, user.Username, mayJoin, user.Restricted, onServer)
	if errors.Is(err, errPlexServers) {
		fail(w, http.StatusBadGateway, "Plex did not list your servers; try again")
		return
	}
	if err == nil {
		a.keepPlexToken(ctx, id, extID, tok) // for their Plex watchlist
	}
	a.outsideDone(w, r, ProviderPlex, id, name, err, place{})
}

// plexUser asks Plex who a confirmed PIN's token belongs to. It returns the
// headers with the token for further calls, or has answered 502.
func (a *Auth) plexUser(ctx context.Context, w http.ResponseWriter, tok string) (plexMe, map[string]string, bool) {
	hdr := a.plexHeaders(ctx)
	hdr["X-Plex-Token"] = tok
	var user plexMe
	code, err := a.fetch(ctx, http.MethodGet, a.plexURL+"/api/v2/user", hdr, nil, &user)
	if err != nil || code/100 != 2 || user.ID == 0 {
		log.Printf("auth: plex user: status %d: %v", code, err)
		fail(w, http.StatusBadGateway, "Plex did not say who you are; try again")
		return user, hdr, false
	}
	return user, hdr, true
}

// checkPin asks Plex whether the person confirmed the PIN. It returns the
// token once they did; otherwise it has answered (waiting, ended, too soon).
// A PIN started to link an account cannot sign anyone in, and the other way
// round; one started from My account is only finished by the same person.
func (a *Auth) checkPin(w http.ResponseWriter, r *http.Request, purpose string) (string, bool) {
	ctx := r.Context()
	var body struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, `send {"id": ...}`)
		return "", false
	}
	a.mu.Lock()
	p := a.pins[body.ID]
	ok := p != nil && p.purpose == purpose && (purpose != pinMine || p.userID == webserver.Actor(ctx).ID) && p.ip == a.clientKey(r) && a.now().Before(p.expires) && pinSecretMatches(r, body.ID, p.secret)
	soon := ok && !p.checked.IsZero() && a.now().Sub(p.checked) < pinCheckGap
	if ok && !soon {
		p.checked = a.now()
	}
	a.mu.Unlock()
	if !ok {
		fail(w, http.StatusNotFound, "this Plex sign-in has ended; start again")
		return "", false
	}
	if soon {
		w.Header().Set("Retry-After", "1")
		fail(w, http.StatusTooManyRequests, "checking too often; wait a second")
		return "", false
	}
	var pinState struct {
		AuthToken string `json:"authToken"`
	}
	code, err := a.fetch(ctx, http.MethodGet, a.plexURL+"/api/v2/pins/"+strconv.FormatInt(body.ID, 10), a.plexHeaders(ctx), nil, &pinState)
	if err != nil || code/100 != 2 {
		log.Printf("auth: plex pin check: status %d: %v", code, err)
		fail(w, http.StatusBadGateway, "Plex did not answer; try again in a moment")
		return "", false
	}
	if pinState.AuthToken == "" {
		writeJSON(w, http.StatusAccepted, map[string]bool{"waiting": true})
		return "", false
	}
	a.mu.Lock()
	delete(a.pins, body.ID) // one pin, one use
	a.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: pinCookie(body.ID), Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: a.behindHTTPS(r), MaxAge: -1})
	return pinState.AuthToken, true
}

// pinCookie names the cookie that holds one pin's secret.
func pinCookie(id int64) string { return "holyarrs_plex_" + strconv.FormatInt(id, 10) }

// pinSecretMatches reports whether the request carries the pin's cookie.
func pinSecretMatches(r *http.Request, id int64, secret string) bool {
	c, err := r.Cookie(pinCookie(id))
	return err == nil && secret != "" && subtle.ConstantTimeCompare([]byte(c.Value), []byte(secret)) == 1
}
