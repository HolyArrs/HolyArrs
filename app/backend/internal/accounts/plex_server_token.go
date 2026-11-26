package accounts

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// ErrNotOwnedPlex means the address is not one of the owner's own Plex
// servers, so no Plex token is handed out for it.
var ErrNotOwnedPlex = errors.New("that address is not one of your Plex servers")

// ErrPlainRemotePlex means a remote Plex address was given as plain http; the
// token is only sent there over https.
var ErrPlainRemotePlex = errors.New("a remote Plex server needs its https address")

// PlexServerToken is the access token of the owner's own Plex server that
// rawURL points at, as plex.tv lists it. The account token itself is never
// returned, and an address that matches none of the owner's servers gets
// nothing, so a token can't be sent to some other host.
func (a *Auth) PlexServerToken(ctx context.Context, rawURL string) (string, error) {
	host, port, ok := hostPort(rawURL)
	plainHTTP := strings.HasPrefix(strings.ToLower(strings.TrimSpace(rawURL)), "http:")
	if !ok {
		return "", ErrNotOwnedPlex
	}
	tok := ""
	if a.vault != nil {
		tok, _ = a.vault.Setting(ctx, settingsGroup, signInPrefix+keyPlexOwner)
	}
	if tok == "" {
		return "", errors.New("no Plex account is linked")
	}
	hdr := a.plexHeaders(ctx)
	hdr["X-Plex-Token"] = tok
	var res []struct {
		Provides    string `json:"provides"`
		Owned       bool   `json:"owned"`
		AccessToken string `json:"accessToken"`
		Connections []struct {
			Address  string `json:"address"`
			Port     int    `json:"port"`
			URI      string `json:"uri"`
			Protocol string `json:"protocol"`
			Local    bool   `json:"local"`
		} `json:"connections"`
	}
	code, err := a.fetch(ctx, http.MethodGet, a.plexURL+"/api/v2/resources?includeHttps=1", hdr, nil, &res)
	if err != nil || code/100 != 2 {
		return "", errors.New("Plex did not list your servers; try again")
	}
	// Plain http is only for a local address, or one plex.tv itself lists as http,
	// so the token never crosses the internet unencrypted.
	refused := false
	for _, x := range res {
		if !x.Owned || !strings.Contains(x.Provides, "server") || x.AccessToken == "" {
			continue
		}
		for _, c := range x.Connections {
			h, p, _ := hostPort(c.URI)
			if !(c.Address != "" && strings.EqualFold(c.Address, host) && c.Port == port) && !(strings.EqualFold(h, host) && p == port) {
				continue
			}
			if plainHTTP && !c.Local && c.Protocol != "http" {
				refused = true
				continue
			}
			return x.AccessToken, nil
		}
	}
	if refused {
		return "", ErrPlainRemotePlex
	}
	return "", ErrNotOwnedPlex
}

// hostPort is the host and port of an http(s) address, with the scheme's
// default port when none is written.
func hostPort(raw string) (string, int, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return "", 0, false
	}
	p := u.Port()
	if p == "" {
		p = map[string]string{"http": "80", "https": "443"}[u.Scheme]
	}
	n, err := strconv.Atoi(p)
	if err != nil {
		return "", 0, false
	}
	return u.Hostname(), n, true
}
