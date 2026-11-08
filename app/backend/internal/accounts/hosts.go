package accounts

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"
)

// Host names and the cross-site check for state-changing browser requests.
//
// The Host header must be an IP address, localhost, a name from
// HOLYARRS_ALLOWED_HOSTS (or Options) or a name saved under "Allowed host
// names" in Settings. A rebinding page always carries its own name in Host,
// so it fails here. First-run setup may also be done on a home name
// (tower.local, nas, media.home.arpa, ...): with no proxy in between, and
// that name is then saved.

// keyAllowedHosts is the sign-in setting with the saved host names (JSON list).
const keyAllowedHosts = "allowed_hosts"

// maxAllowedHosts caps the saved list.
const maxAllowedHosts = 50

// Failed first-run owner attempts: about 10 a minute per client. There is no
// cap for all clients together: a shared cap would let anyone who reaches the
// port block the owner's setup.
const (
	setupTries  = 10
	setupWindow = time.Minute
)

// crossOrigin is Go's cross-origin protection: Sec-Fetch-Site must be
// same-origin or none, else the Origin host and port must equal Host. It
// does not guess the scheme, so a browser on https behind a proxy, tunnel
// or tailscale serve that talks plain HTTP to us is the same site. One
// instance, no trusted origins and no bypass.
var crossOrigin = http.NewCrossOriginProtection()

// normHost cleans a host name or a pasted address: lower case, no scheme,
// no port, no trailing slash or dot.
func normHost(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if _, rest, ok := strings.Cut(s, "://"); ok {
		s = rest
	}
	s = strings.TrimRight(s, "/")
	if h, _, err := net.SplitHostPort(s); err == nil {
		s = h
	}
	return strings.TrimSuffix(strings.Trim(s, "[]"), ".")
}

// hostEntryOK says whether a cleaned name may be saved: an IP address, or
// letters, digits, '.', '-' and '_' only, at most 253 characters.
func hostEntryOK(h string) bool {
	if h == "" || len(h) > 253 {
		return false
	}
	if net.ParseIP(h) != nil {
		return true
	}
	for _, c := range h {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '.' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

// homeName is a name that only resolves at home: no dot, or ending in
// .local, .home.arpa, .internal or .localhost. Never .lan, .ts.net or .onion.
func homeName(h string) bool {
	labels := strings.Split(h, ".")
	for _, l := range labels {
		if l == "" || len(l) > 63 || l[0] == '-' || l[len(l)-1] == '-' {
			return false
		}
		for _, c := range l {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	if len(labels) == 1 {
		return net.ParseIP(h) == nil
	}
	for _, suffix := range []string{".local", ".home.arpa", ".internal", ".localhost"} {
		if strings.HasSuffix(h, suffix) && len(h) > len(suffix) {
			return true
		}
	}
	return false
}

// forwarded says whether a proxy header is on the request.
func forwarded(r *http.Request) bool {
	for _, k := range []string{"X-Forwarded-For", "Forwarded", "X-Real-IP", "X-Forwarded-Host"} {
		if r.Header.Get(k) != "" {
			return true
		}
	}
	return false
}

// echoHost is the request's host name made safe to show: cleaned, cut at
// the first character a name never has, and capped.
func echoHost(host string) string {
	h := normHost(host)
	if i := strings.IndexFunc(h, func(c rune) bool {
		return !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '.' || c == '-' || c == '_' || c == ':')
	}); i >= 0 {
		h = h[:i]
	}
	if len(h) > 100 {
		h = h[:100] + "..."
	}
	if h == "" {
		h = "(empty)"
	}
	return h
}

func hostRefusedMsg(host string) string {
	return "unknown host name " + echoHost(host) + "; open HolyArrs by its IP address and add the name under Allowed host names in Settings, or add it to " + EnvAllowedHosts
}

// hostAllowed checks the Host header against IP addresses, localhost and
// the configured and saved names.
func (a *Auth) hostAllowed(r *http.Request) bool {
	h := normHost(r.Host)
	if h == "" {
		return false
	}
	if net.ParseIP(h) != nil || a.hosts[h] {
		return true
	}
	if saved := a.savedHosts.Load(); saved != nil {
		return slices.Contains(*saved, h)
	}
	return false
}

// refuse writes the 403 for a state-changing request from a host name that
// is not allowed (when checkHost) or from another site, and says which.
func (a *Auth) refuse(w http.ResponseWriter, r *http.Request, checkHost bool) bool {
	if checkHost && !a.hostAllowed(r) {
		a.logRefusedHost(r.Host)
		fail(w, http.StatusForbidden, hostRefusedMsg(r.Host))
		return true
	}
	if crossOrigin.Check(r) != nil {
		fail(w, http.StatusForbidden, "request from another site refused")
		return true
	}
	return false
}

// logRefusedHost logs each refused name once, for at most 32 names.
func (a *Auth) logRefusedHost(host string) {
	h := echoHost(host)
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.refusedHosts == nil {
		a.refusedHosts = map[string]bool{}
	}
	if a.refusedHosts[h] || len(a.refusedHosts) >= 32 {
		return
	}
	a.refusedHosts[h] = true
	log.Printf("auth: refused host name %s; add it under Allowed host names in Settings or to %s if it is yours", h, EnvAllowedHosts)
}

// cleanHostList cleans and checks names from Settings, keeping their order.
func cleanHostList(in []string) ([]string, error) {
	out := []string{}
	for _, e := range in {
		if strings.TrimSpace(e) == "" {
			continue
		}
		h := normHost(e)
		if !hostEntryOK(h) {
			return nil, errors.New("host names may have letters, digits, '.', '-' and '_' only, like tower.local or media.example.com")
		}
		if !slices.Contains(out, h) {
			out = append(out, h)
		}
	}
	if len(out) > maxAllowedHosts {
		return nil, errors.New("too many host names")
	}
	return out, nil
}

// loadSavedHosts reads the names saved in Settings.
func (a *Auth) loadSavedHosts(ctx context.Context) {
	list := []string{}
	if v := a.signInSetting(ctx, keyAllowedHosts); v != "" {
		var raw []string
		if err := json.Unmarshal([]byte(v), &raw); err != nil {
			log.Printf("auth: ignoring saved allowed host names: %v", err)
		} else if l, err := cleanHostList(raw); err == nil {
			list = l
		}
	}
	a.savedHosts.Store(&list)
}

func (a *Auth) savedHostList() []string {
	if p := a.savedHosts.Load(); p != nil {
		return *p
	}
	return []string{}
}

// ownerSetupLimited says whether a client has used up its failed first-run
// owner attempts for the current minute.
func (a *Auth) ownerSetupLimited(key string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	f := a.ownerSetupFails[key]
	return f != nil && a.now().Sub(f.last) < setupWindow && f.n >= setupTries
}

// noteOwnerSetupFail counts one failed first-run owner attempt for a client.
func (a *Auth) noteOwnerSetupFail(key string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.now()
	if a.ownerSetupFails == nil || len(a.ownerSetupFails) > 10000 {
		a.ownerSetupFails = map[string]*failure{}
	}
	f := a.ownerSetupFails[key]
	if f == nil || now.Sub(f.last) >= setupWindow {
		f = &failure{last: now}
		a.ownerSetupFails[key] = f
	}
	f.n++
}
