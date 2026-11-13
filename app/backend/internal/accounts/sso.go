package accounts

import (
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// SSOOptions is operator-owned startup configuration, never writable by a provider.
type SSOOptions struct {
	OIDC    *OIDCOptions
	Forward *ForwardAuthOptions
	// OIDCFromEnv is set when HOLYARRS_OIDC_ENABLED is present at all, even
	// as false: the environment then owns single sign-on and Settings only
	// shows it. Otherwise the owner sets it up in Settings (sso_settings.go).
	OIDCFromEnv bool
	// SavedLimits are the address rules for single sign-on set up in
	// Settings. They come only from the environment, never from a browser.
	SavedLimits OIDCLimits
}

// OIDCLimits say which provider addresses single sign-on may reach.
type OIDCLimits struct {
	AllowLocal        bool // providers on the local network (cloud metadata stays blocked)
	AllowLoopbackHTTP bool // plain HTTP on this machine, for testing only
}

type ssoState struct {
	oidc    *oidcRuntime
	forward *forwardRuntime
}

// SSOFromEnv validates enabled provider configuration without contacting it.
func SSOFromEnv(urlBase string) (SSOOptions, error) {
	var out SSOOptions
	flag := func(name string) (bool, error) {
		v := os.Getenv(name)
		switch v {
		case "", "false":
			return false, nil
		case "true":
			return true, nil
		}
		return false, errors.New("SSO boolean settings must be true or false")
	}
	const prefix = "HOLYARRS_"
	_, out.OIDCFromEnv = os.LookupEnv(prefix + "OIDC_ENABLED")
	// The address rules also apply to single sign-on set up in Settings.
	var err error
	if out.SavedLimits.AllowLocal, err = flag(prefix + "OIDC_ALLOW_LOCAL_NETWORK"); err != nil {
		return out, err
	}
	if out.SavedLimits.AllowLoopbackHTTP, err = flag(prefix + "OIDC_ALLOW_LOOPBACK_HTTP"); err != nil {
		return out, err
	}
	on, err := flag(prefix + "OIDC_ENABLED")
	if err != nil {
		return out, err
	}
	if on {
		v := &OIDCOptions{Issuer: os.Getenv(prefix + "OIDC_ISSUER"), ClientID: os.Getenv(prefix + "OIDC_CLIENT_ID"), RedirectURL: os.Getenv(prefix + "OIDC_REDIRECT_URL")}
		for name, dst := range map[string]*bool{"ALLOW_JOIN": &v.AllowJoin, "ALLOW_LOCAL_NETWORK": &v.AllowLocal, "ALLOW_LOOPBACK_HTTP": &v.AllowLoopbackHTTP} {
			if *dst, err = flag(prefix + "OIDC_" + name); err != nil {
				return out, err
			}
		}
		if path := os.Getenv(prefix + "OIDC_CLIENT_SECRET_FILE"); path != "" {
			f, err := os.Open(path)
			if err != nil {
				return out, errors.New("OIDC client secret file cannot be read")
			}
			b, readErr := io.ReadAll(io.LimitReader(f, 4097))
			_ = f.Close()
			if readErr != nil || len(b) > 4096 || strings.TrimSpace(string(b)) == "" {
				return out, errors.New("OIDC client secret file must contain a bounded nonempty value")
			}
			v.ClientSecret = strings.TrimSpace(string(b))
		}
		runtime, err := newOIDCRuntime(*v)
		if err != nil {
			return out, errors.New("OIDC configuration is invalid")
		}
		runtime.close()
		if runtime.redirect.Path != urlBase+"/api/v1/auth/oidc/callback" {
			return out, errors.New("OIDC callback path does not match the configured URL base")
		}
		out.OIDC = v
	}
	on, err = flag(prefix + "FORWARD_AUTH_ENABLED")
	if err != nil {
		return out, err
	}
	if on {
		v := &ForwardAuthOptions{Issuer: os.Getenv(prefix + "FORWARD_AUTH_ISSUER"), TrustedPeers: strings.Split(os.Getenv(prefix+"FORWARD_AUTH_TRUSTED_PEERS"), ",")}
		if v.AllowJoin, err = flag(prefix + "FORWARD_AUTH_ALLOW_JOIN"); err != nil {
			return out, err
		}
		if _, err := newForwardRuntime(*v); err != nil {
			return out, errors.New("forward-auth configuration is invalid")
		}
		out.Forward = v
	}
	return out, nil
}

// Close stops auth-owned provider work and is safe to call more than once.
func (a *Auth) Close() {
	if s := a.sso.Load(); s != nil && s.oidc != nil {
		s.oidc.close()
	}
}

// initSSO builds each way on its own: a broken single sign-on setup turns
// off only single sign-on, never trusted proxy sign-in, and the other way round.
func (a *Auth) initSSO(opt SSOOptions) {
	state := &ssoState{}
	if opt.OIDC != nil {
		o, err := newOIDCRuntime(*opt.OIDC)
		if err != nil {
			log.Print("auth: invalid single sign-on configuration; single sign-on is off")
		} else {
			state.oidc = o
		}
	}
	if opt.Forward != nil {
		f, err := newForwardRuntime(*opt.Forward)
		if err != nil {
			log.Print("auth: invalid trusted proxy configuration; trusted proxy sign-in is off")
		} else {
			state.forward = f
		}
	}
	a.sso.Store(state)
}

// ssoNow is the current single sign-on state, never nil. A handler reads it
// once and keeps what it got, as a settings save swaps the whole state.
func (a *Auth) ssoNow() *ssoState {
	if s := a.sso.Load(); s != nil {
		return s
	}
	return &ssoState{}
}

// swapOIDC puts o (nil turns single sign-on off) in place of the current
// runtime and closes the old one; flows in flight on it fail safely and the
// person starts again.
func (a *Auth) swapOIDC(o *oidcRuntime) {
	for {
		cur := a.sso.Load()
		next := &ssoState{oidc: o}
		if cur != nil {
			next.forward = cur.forward
		}
		if a.sso.CompareAndSwap(cur, next) {
			if cur != nil && cur.oidc != nil && cur.oidc != o {
				cur.oidc.close()
			}
			return
		}
	}
}

func (a *Auth) ssoRoutes(m *http.ServeMux) {
	a.oidcRoutes(m)
	a.ssoSettingsRoutes(m)
	m.HandleFunc("POST /api/v1/auth/forward/login", a.forwardLogin)
	m.HandleFunc("POST /api/v1/auth/forward/link", a.forwardLink)
}

func (a *Auth) requireSSOLink(w http.ResponseWriter, r *http.Request) (webserver.Caller, bool) {
	who, ok := webserver.From(r.Context())
	if !ok || who.Via != "session" || !(who.LocalRecent || outsideCounts(who)) || who.MustTwoFactor {
		fail(w, http.StatusForbidden, "confirm your local password in My account before linking")
		return webserver.Caller{}, false
	}
	return who, true
}
