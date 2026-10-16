package auth

import (
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

// SSOOptions is operator-owned startup configuration, never writable by a provider.
type SSOOptions struct {
	OIDC    *OIDCOptions
	Forward *ForwardAuthOptions
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

func (a *Auth) initSSO(opt SSOOptions) {
	state := &ssoState{}
	a.sso = state
	var err error
	if opt.OIDC != nil {
		state.oidc, err = newOIDCRuntime(*opt.OIDC)
		if err != nil {
			log.Print("auth: invalid SSO configuration; SSO disabled")
			return
		}
	}
	if opt.Forward != nil {
		state.forward, err = newForwardRuntime(*opt.Forward)
		if err != nil {
			if state.oidc != nil {
				state.oidc.close()
			}
			a.sso = &ssoState{}
			log.Print("auth: invalid SSO configuration; SSO disabled")
			return
		}
	}
}

// Close stops auth-owned provider work and is safe to call more than once.
func (a *Auth) Close() {
	if a.sso != nil && a.sso.oidc != nil {
		a.sso.oidc.close()
	}
}

func (a *Auth) ssoRoutes(m *http.ServeMux) {
	a.oidcRoutes(m)
	m.HandleFunc("POST /api/v1/auth/forward/login", a.forwardLogin)
	m.HandleFunc("POST /api/v1/auth/forward/link", a.forwardLink)
}

func (a *Auth) requireSSOLink(w http.ResponseWriter, r *http.Request) (Who, bool) {
	who, ok := From(r.Context())
	if !ok || who.Via != "session" || !who.LocalRecent || who.MustTwoFactor {
		fail(w, http.StatusForbidden, "confirm your local password in My account before linking")
		return Who{}, false
	}
	return who, true
}
