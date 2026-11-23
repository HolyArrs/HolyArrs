package accounts

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// plexServerChoice is one address of one of the owner's Plex servers, as the
// first-run Connect server step lists it: the secure plex.direct address when
// Plex has it, and a plain http address only for a local (home network) one.
type plexServerChoice struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Address   string `json:"address"`
	Port      int    `json:"port"`
	Local     bool   `json:"local"`
	URL       string `json:"url"`
	SecureURL string `json:"secure_url,omitempty"`
	// Relay is a Plex relay address, offered only when a server has no other way in.
	Relay bool `json:"relay,omitempty"`
}

// plexServers lists the Plex servers the owner's linked Plex account owns,
// one row per address, local ones first. The token never leaves the server.
func (a *Auth) plexServers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// The list is the owner's own servers: from a browser session only, never a key.
	if who, ok := webserver.From(ctx); !ok || who.Via != "session" {
		fail(w, http.StatusForbidden, "sign in in the browser first")
		return
	}
	tok := ""
	if a.vault != nil {
		tok, _ = a.vault.Setting(ctx, settingsGroup, signInPrefix+keyPlexOwner)
	}
	if tok == "" {
		fail(w, http.StatusConflict, "no Plex account is linked")
		return
	}
	hdr := a.plexHeaders(ctx)
	hdr["X-Plex-Token"] = tok
	var res []struct {
		Name             string `json:"name"`
		ClientIdentifier string `json:"clientIdentifier"`
		Provides         string `json:"provides"`
		Owned            bool   `json:"owned"`
		Connections      []struct {
			Protocol string `json:"protocol"`
			Address  string `json:"address"`
			Port     int    `json:"port"`
			URI      string `json:"uri"`
			Local    bool   `json:"local"`
			Relay    bool   `json:"relay"`
			IPv6     bool   `json:"IPv6"`
		} `json:"connections"`
	}
	code, err := a.fetch(ctx, http.MethodGet, a.plexURL+"/api/v2/resources?includeHttps=1", hdr, nil, &res)
	if err != nil || code/100 != 2 {
		fail(w, http.StatusBadGateway, "Plex did not list your servers; try again")
		return
	}
	out := []*plexServerChoice{}
	byKey := map[string]*plexServerChoice{}
	plain := map[*plexServerChoice]string{}
	// A secure relay per server, kept for a server with no direct address.
	relays := map[string]*plexServerChoice{}
	direct := map[string]bool{}
	for _, x := range res {
		if !x.Owned || !strings.Contains(x.Provides, "server") || x.ClientIdentifier == "" {
			continue
		}
		for _, c := range x.Connections {
			// IPv6 addresses rarely work from a container.
			if c.IPv6 || c.Address == "" || c.Port <= 0 {
				continue
			}
			// Relays are slow and capped: only a secure one, and only as a last resort.
			if c.Relay {
				if c.Protocol == "https" && hostOf(c.URI) != "" && relays[x.ClientIdentifier] == nil {
					u := truncate(c.URI, 500)
					relays[x.ClientIdentifier] = &plexServerChoice{ID: x.ClientIdentifier, Name: truncate(x.Name, 200), Address: c.Address, Port: c.Port, URL: u, SecureURL: u, Relay: true}
				}
				continue
			}
			k := x.ClientIdentifier + "|" + strings.ToLower(c.Address) + "|" + strconv.Itoa(c.Port)
			row := byKey[k]
			if row == nil {
				row = &plexServerChoice{ID: x.ClientIdentifier, Name: truncate(x.Name, 200), Address: c.Address, Port: c.Port, Local: c.Local}
				byKey[k] = row
				out = append(out, row)
			}
			row.Local = row.Local || c.Local
			// The secure way in is the plex.direct name that carries the certificate.
			if c.Protocol == "https" && row.SecureURL == "" && hostOf(c.URI) != "" {
				row.SecureURL = truncate(c.URI, 500)
			}
			if c.Protocol == "http" && plain[row] == "" && hostOf(c.URI) != "" {
				plain[row] = truncate(c.URI, 500)
			}
		}
	}
	// Plain http only on the home network. A remote row offers only what plex.tv
	// lists for it, secure first; an http address is never made up for it.
	kept := out[:0]
	for _, row := range out {
		switch {
		case row.Local:
			row.URL = truncate("http://"+row.Address+":"+strconv.Itoa(row.Port), 500)
		case row.SecureURL != "":
			row.URL = row.SecureURL
		default:
			row.URL = plain[row]
		}
		if row.URL != "" {
			kept = append(kept, row)
			direct[row.ID] = true
		}
	}
	out = kept
	for _, x := range res {
		if row := relays[x.ClientIdentifier]; row != nil && !direct[x.ClientIdentifier] {
			direct[x.ClientIdentifier] = true
			out = append(out, row)
		}
	}
	rank := func(c *plexServerChoice) int {
		switch {
		case c.Local:
			return 0
		case c.Relay:
			return 2
		}
		return 1
	}
	sort.SliceStable(out, func(i, j int) bool { return rank(out[i]) < rank(out[j]) })
	writeJSON(w, http.StatusOK, map[string]any{"servers": out})
}

// hostOf is the host name of an address like https://1-2-3-4.abc.plex.direct:32400.
func hostOf(u string) string {
	u = strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://")
	if i := strings.IndexAny(u, ":/"); i >= 0 {
		u = u[:i]
	}
	return u
}
