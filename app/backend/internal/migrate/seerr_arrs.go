package migrate

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/migrate/secretsfile"
)

// SeerrArrs lists the Sonarr and Radarr servers an Overseerr or Seerr is set
// up with, each as a Source carrying the key Overseerr keeps, under src's
// local-network leave and limits. Any other app has none. The keys stay on
// this server.
func SeerrArrs(ctx context.Context, src Source) ([]Source, error) {
	if (src.Kind != Seerr && src.Kind != Overseerr) || src.Via() != ViaAPI {
		return nil, nil
	}
	c, err := seerrReader{kind: src.Kind}.client(src)
	if err != nil {
		return nil, err
	}
	var out []Source
	for _, k := range []Kind{Sonarr, Radarr} {
		var list []map[string]any
		if err := c.getJSON(ctx, "/api/v1/settings/"+string(k), nil, &list); err != nil {
			return nil, err
		}
		for _, m := range list {
			ssl, _ := m["useSsl"].(bool)
			a, key := seerrArrURL(str(m["hostname"]), num(m["port"]), ssl, str(m["baseUrl"])), str(m["apiKey"])
			if a == "" || key == "" {
				continue
			}
			out = append(out, Source{Kind: k, URL: a, Key: key, AllowLocal: src.AllowLocal, Limits: src.Limits})
		}
	}
	return out, nil
}

// seerrArrURL builds an app address from the parts Overseerr keeps.
func seerrArrURL(host string, port int64, ssl bool, base string) string {
	host = strings.TrimSpace(host)
	if host == "" || port <= 0 || port > 65535 {
		return ""
	}
	u := url.URL{Scheme: "http", Host: net.JoinHostPort(host, strconv.FormatInt(port, 10))}
	if ssl {
		u.Scheme = "https"
	}
	if base = strings.Trim(strings.TrimSpace(base), "/"); base != "" {
		u.Path = "/" + base
	}
	return InstanceAddress(u.String())
}

// LinkedFrom names the Overseerr or Seerr read before and the address it
// keeps for the Sonarr or Radarr being read at a new address.
type LinkedFrom struct {
	Kind    Kind   `json:"kind"`
	URL     string `json:"url"`
	Address string `json:"address"`
}

// linkedKey fills in.Key with the key the Overseerr or Seerr in
// in.LinkedFrom keeps for that app, using that Seerr's saved key. It answers
// and returns false when there is none.
func (h *Handlers) linkedKey(w http.ResponseWriter, r *http.Request, in *MigrateSourceInput) bool {
	if who, signed := webserver.From(r.Context()); !signed || who.Role != webserver.RoleOwner {
		webserver.Fail(w, http.StatusForbidden, "only the owner may use a saved key")
		return false
	} else if who.Via != "session" {
		// The saved key goes to in.URL, which may differ from the address Seerr
		// keeps (the owner fixes a Docker name like radarr:7878 to an IP on the
		// "needs a look" step). Only a signed-in browser may send it to a
		// different address, so an API key may never use a linked key at all.
		webserver.Fail(w, http.StatusForbidden, "sign in with your password to use a saved key")
		return false
	}
	from := in.LinkedFrom
	none := func() bool {
		webserver.Fail(w, http.StatusBadRequest, "there is no saved key for this app at this address; type its key")
		return false
	}
	if (from.Kind != Seerr && from.Kind != Overseerr) || (in.Kind != Sonarr && in.Kind != Radarr) {
		return none()
	}
	c, _, has, err := secretsfile.ReadMoveCredentialAt(r.Context(), h.Store, h.Secrets, string(from.Kind), from.URL)
	if err != nil {
		webserver.Internal(w, "migrate", err)
		return false
	}
	if !has {
		return none()
	}
	ctx, cancel := context.WithTimeout(r.Context(), readTimeout)
	defer cancel()
	arrs, err := SeerrArrs(ctx, Source{Kind: from.Kind, URL: c.Address, Key: c.APIKey, AllowLocal: c.AllowLocalNetwork, Limits: h.Limits})
	if err != nil {
		fail(w, err)
		return false
	}
	for _, a := range arrs {
		if a.Kind == in.Kind && InstanceAddress(a.URL) == InstanceAddress(from.Address) {
			in.Key = a.Key
			return true
		}
	}
	return none()
}
