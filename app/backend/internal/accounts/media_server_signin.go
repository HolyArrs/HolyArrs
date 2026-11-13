package accounts

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
)

// mediaMe is the Jellyfin or Emby account a name and password belong to.
type mediaMe struct {
	ID, Name string
	// Limited is set when the server limits what this person may watch.
	Limited bool
}

func (a *Auth) mediaServerLogin(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	v, ok := a.mediaWay(w, r, kind)
	if !ok {
		return
	}
	got, pl, ok := a.checkMediaServer(w, r, kind, v)
	if !ok {
		return
	}
	id, name, err := a.linkedUser(r.Context(), kind, got.ID, got.Name, allowed(v, got.Name), got.Limited, nil)
	a.outsideDone(w, r, kind, id, name, err, pl)
}

// mediaWay is the Jellyfin or Emby sign-in way, or false after answering
// 404 when there is no such way or it is off.
func (a *Auth) mediaWay(w http.ResponseWriter, r *http.Request, kind string) (WayView, bool) {
	if kind != ProviderJellyfin && kind != ProviderEmby {
		fail(w, http.StatusNotFound, "no such sign-in way")
		return WayView{}, false
	}
	v := a.way(r.Context(), kind)
	if !v.On || v.URL == "" {
		fail(w, http.StatusNotFound, "sign-in with "+kind+" is off")
		return v, false
	}
	return v, true
}

// checkMediaServer checks the name and password in the request against the
// owner's Jellyfin or Emby server, with the sign-in limits. It returns the
// account and the place to record, or false after answering.
func (a *Auth) checkMediaServer(w http.ResponseWriter, r *http.Request, kind string, v WayView) (mediaMe, place, bool) {
	ctx := r.Context()
	c, ok := readCreds(w, r)
	if !ok {
		return mediaMe{}, place{}, false
	}
	if c.Name == "" || len(c.Name) > 128 || len(c.Password) > MaxPassword {
		fail(w, http.StatusBadRequest, "give your "+kind+" name and password")
		return mediaMe{}, place{}, false
	}
	ipKey, altKey, sure := a.signInKeys(r)
	userKey := "name:" + kind + ":" + strings.ToLower(c.Name)
	var marks []failMark
	a.mu.Lock()
	wait := max(a.signInWait(ipKey, userKey, sure), a.altWait(altKey))
	if wait == 0 {
		marks = a.markFails(ipKey, altKey, userKey)
		a.noteFail(ipKey, altKey, userKey)
	}
	a.mu.Unlock()
	// Only an attempt the server could not check is taken back: no answer
	// at all, or a proxy saying the server behind it is down (502, 503,
	// 504). Any other answer counts, so a server that turns down wrong
	// passwords with an odd status is still limited. A client that hung up
	// mid-check keeps its count, so hanging up skips no wait.
	undo := func(code int) {
		down := code == 0 || code == http.StatusBadGateway || code == http.StatusServiceUnavailable || code == http.StatusGatewayTimeout
		if down && ctx.Err() == nil {
			a.undoFail(marks)
		}
	}
	if wait > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		fail(w, http.StatusTooManyRequests, "too many failed sign-ins; wait a little and try again")
		return mediaMe{}, place{}, false
	}
	authz := fmt.Sprintf(`MediaBrowser Client="%s", Device="%s", DeviceId="%s", Version="1"`, product, product, a.clientID(ctx))
	hdr := map[string]string{"Authorization": authz, "X-Emby-Authorization": authz}
	var got struct {
		User struct {
			ID     string `json:"Id"`
			Name   string `json:"Name"`
			Policy struct {
				IsDisabled bool `json:"IsDisabled"`
				// Set when the server limits what this person may watch.
				MaxParentalRating *int `json:"MaxParentalRating"`
			} `json:"Policy"`
		} `json:"User"`
		AccessToken string `json:"AccessToken"`
	}
	code, err := a.serverFetch(ctx, http.MethodPost, v.URL+"/Users/AuthenticateByName", hdr, map[string]string{"Username": c.Name, "Pw": c.Password}, &got)
	if err != nil {
		undo(code)
		log.Printf("auth: %s sign-in: %v", kind, err)
		fail(w, http.StatusBadGateway, "the "+kind+" server did not answer; ask the owner")
		return mediaMe{}, place{}, false
	}
	if code == http.StatusUnauthorized || code == http.StatusForbidden || code/100 == 4 {
		log.Printf("auth: failed %s sign-in from %s", kind, r.RemoteAddr)
		a.failedEvent(r, a.linkedName(ctx, kind, "", c.Name), kind)
		fail(w, http.StatusUnauthorized, "wrong name or password")
		return mediaMe{}, place{}, false
	}
	if code/100 != 2 || got.User.ID == "" {
		undo(code)
		log.Printf("auth: %s sign-in: status %d", kind, code)
		fail(w, http.StatusBadGateway, "the "+kind+" server gave an odd answer; ask the owner")
		return mediaMe{}, place{}, false
	}
	if got.AccessToken != "" {
		// HolyArrs does not keep the server's token: end that session now.
		lh := map[string]string{"Authorization": fmt.Sprintf("%s, Token=%q", authz, got.AccessToken)}
		lh["X-Emby-Token"] = got.AccessToken
		_, _ = a.serverFetch(context.WithoutCancel(ctx), http.MethodPost, v.URL+"/Sessions/Logout", lh, nil, nil)
	}
	if got.User.Policy.IsDisabled {
		a.failedEvent(r, a.linkedName(ctx, kind, got.User.ID, got.User.Name), kind+" turned off")
		fail(w, http.StatusForbidden, "this account is turned off on the "+kind+" server")
		return mediaMe{}, place{}, false
	}
	a.mu.Lock()
	a.undoFailLocked(marks[:2]) // this attempt's address counts only (ipKey, altKey)
	delete(a.fails, userKey)
	a.mu.Unlock()
	return mediaMe{ID: got.User.ID, Name: got.User.Name, Limited: got.User.Policy.MaxParentalRating != nil}, place{ipKey, userKey, sure}, true
}
