package accounts

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ForwardAuthOptions trusts only direct proxy peers, independently of forwarded-IP settings.
type ForwardAuthOptions struct {
	Issuer       string
	TrustedPeers []string
	AllowJoin    bool
}

type forwardRuntime struct {
	opt   ForwardAuthOptions
	peers []*net.IPNet
}

func newForwardRuntime(opt ForwardAuthOptions) (*forwardRuntime, error) {
	u, err := url.Parse(opt.Issuer)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || len(opt.Issuer) > 1000 {
		return nil, errors.New("invalid forward-auth issuer")
	}
	f := &forwardRuntime{opt: opt}
	if len(opt.TrustedPeers) > 64 {
		return nil, errors.New("too many forward-auth peers")
	}
	for _, peer := range opt.TrustedPeers {
		peer = strings.TrimSpace(peer)
		if !strings.Contains(peer, "/") {
			ip := net.ParseIP(peer)
			if ip == nil {
				return nil, errors.New("forward-auth peers must be IP addresses or CIDRs")
			}
			if ip.To4() != nil {
				peer += "/32"
			} else {
				peer += "/128"
			}
		}
		_, n, err := net.ParseCIDR(peer)
		if err != nil {
			return nil, errors.New("invalid forward-auth peer")
		}
		bits, _ := n.Mask.Size()
		if bits == 0 {
			return nil, errors.New("forward-auth peers cannot trust every address")
		}
		f.peers = append(f.peers, n)
	}
	if len(f.peers) == 0 {
		return nil, errors.New("forward-auth requires explicit trusted peers")
	}
	return f, nil
}

func (a *Auth) forwardIdentity(r *http.Request, fwd *forwardRuntime) (string, string, bool) {
	if fwd == nil {
		return "", "", false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return "", "", false
	}
	ip := net.ParseIP(host)
	trusted := false
	for _, peer := range fwd.peers {
		trusted = trusted || peer.Contains(ip)
	}
	if !trusted {
		return "", "", false
	}
	values, names := r.Header.Values("Remote-User"), r.Header.Values("Remote-Name")
	if len(values) != 1 || len(names) > 1 || !ssoText(values[0], 255) || strings.Contains(values[0], ",") {
		return "", "", false
	}
	name := values[0]
	if len(names) == 1 {
		if !ssoText(names[0], 200) || strings.Contains(names[0], ",") {
			return "", "", false
		}
		name = names[0]
	}
	return values[0], name, true
}

func (a *Auth) ssoAttempt(w http.ResponseWriter, r *http.Request) bool {
	return a.ssoGate(w, r, true)
}

// ssoContinue gates the later steps of a sign-in already counted at its
// start (callback, finish): they wait out a running wait but add no count,
// so one refused or abandoned sign-in cannot fail the next person's last step.
// Guessing a state or result cookie (32 random bytes) gains nothing from extra tries.
func (a *Auth) ssoContinue(w http.ResponseWriter, r *http.Request) bool {
	return a.ssoGate(w, r, false)
}

func (a *Auth) ssoGate(w http.ResponseWriter, r *http.Request, count bool) bool {
	keys := a.ssoKeys(r)
	a.mu.Lock()
	var wait time.Duration
	for _, k := range keys {
		wait = max(wait, a.waitFor(k, freeTries))
	}
	if wait == 0 && count {
		a.noteFail(keys...)
	}
	a.mu.Unlock()
	if wait > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		fail(w, http.StatusTooManyRequests, "too many sign-in attempts; wait a little and try again")
		return false
	}
	return true
}

// ssoKeys are the single sign-on limit keys: the caller's, plus the other
// header's address when the proxy headers disagree, so rotating a forged
// header buys no fresh budget.
func (a *Auth) ssoKeys(r *http.Request) []string {
	ipKey, altKey, _ := a.clientKeys(r)
	if altKey == "" {
		return []string{"sso:" + ipKey}
	}
	return []string{"sso:" + ipKey, "sso:" + altKey}
}

func (a *Auth) ssoPassed(r *http.Request) {
	a.mu.Lock()
	for _, k := range a.ssoKeys(r) {
		delete(a.fails, k)
	}
	a.mu.Unlock()
}

func (a *Auth) forwardLogin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	a.ssoMu.RLock() // until the session exists: see ssoMu
	defer a.ssoMu.RUnlock()
	fwd := a.ssoNow().forward // read once: a settings save may swap it
	if fwd == nil {
		fail(w, http.StatusNotFound, "this sign-in method is off")
		return
	}
	if !a.ssoAttempt(w, r) {
		return
	}
	subject, display, ok := a.forwardIdentity(r, fwd)
	if !ok {
		fail(w, http.StatusForbidden, "trusted proxy identity required")
		return
	}
	id, name, err := a.ssoIdentity(r.Context(), "forward_auth", fwd.opt.Issuer, subject, display, fwd.opt.AllowJoin, 0)
	if err != nil {
		a.failedEvent(r, outsideName(display), ssoRefusal("forward_auth", err))
		fail(w, http.StatusForbidden, "this identity is not allowed to sign in")
		return
	}
	a.ssoPassed(r)
	a.passedAs(w, r, id, name, "forward_auth")
}

func (a *Auth) forwardLink(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	a.ssoMu.RLock()
	defer a.ssoMu.RUnlock()
	fwd := a.ssoNow().forward // read once: a settings save may swap it
	if fwd == nil {
		fail(w, http.StatusNotFound, "this sign-in method is off")
		return
	}
	who, ok := a.requireSSOLink(w, r)
	if !ok {
		return
	}
	subject, display, ok := a.forwardIdentity(r, fwd)
	if !ok {
		fail(w, http.StatusForbidden, "trusted proxy identity required")
		return
	}
	if _, _, err := a.ssoLinkIdentity(r, "forward_auth", fwd.opt.Issuer, subject, display, who.UserID, a.currentFamily(r)); err != nil {
		fail(w, http.StatusForbidden, "this identity cannot be linked")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"linked": true})
}
