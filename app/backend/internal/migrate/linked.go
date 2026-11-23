package migrate

import (
	"context"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Linked is an app another old app is set up with: Overseerr and Seerr keep
// their Sonarr and Radarr servers with keys, Prowlarr its synced apps
// without. The key stays on the server: it is never answered.
type Linked struct {
	Kind    Kind   `json:"kind"`
	Name    string `json:"name,omitempty"`
	Address string `json:"address"`
	FourK   bool   `json:"four_k,omitempty"`
	// HasKey says the key came along, so the app can be added in one go.
	HasKey bool `json:"has_key"`
	// Reachable says HolyArrs reached the app with that key (only with a key).
	Reachable bool   `json:"reachable"`
	Version   string `json:"version,omitempty"`
	key       string
}

// linkedProbeWait bounds the check of each linked app.
const linkedProbeWait = 5 * time.Second

// ReadLinked lists the apps src is set up with and checks the ones with a
// key, from this server, so an address only the old app resolves shows as
// not reached.
func ReadLinked(ctx context.Context, src Source) ([]Linked, error) {
	if err := check(src); err != nil {
		return nil, err
	}
	if src.Via() != ViaAPI {
		return nil, ErrBadSource
	}
	var out []Linked
	var err error
	switch src.Kind {
	case Seerr, Overseerr:
		out, err = seerrLinked(ctx, src)
	case Prowlarr:
		out, err = prowlarrLinked(ctx, src)
	default:
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var wg sync.WaitGroup
	for i := range out {
		if out[i].key == "" {
			continue
		}
		wg.Add(1)
		go func(l *Linked) {
			defer wg.Done()
			pctx, cancel := context.WithTimeout(ctx, linkedProbeWait)
			defer cancel()
			if info, err := Probe(pctx, l.source(src)); err == nil {
				l.Reachable, l.Version = true, info.Version
			}
		}(&out[i])
	}
	wg.Wait()
	return out, nil
}

// source is the Source to read l with, under parent's local-network leave
// and limits.
func (l Linked) source(parent Source) Source {
	return Source{Kind: l.Kind, URL: l.Address, Key: l.key, AllowLocal: parent.AllowLocal, Limits: parent.Limits}
}

// linkedAddress builds an app address from the parts Overseerr keeps.
func linkedAddress(host string, port int64, ssl bool, base string) string {
	host = strings.TrimSpace(host)
	if host == "" || port <= 0 || port > 65535 {
		return ""
	}
	scheme := "http"
	if ssl {
		scheme = "https"
	}
	base = strings.Trim(strings.TrimSpace(base), "/")
	u := url.URL{Scheme: scheme, Host: net.JoinHostPort(host, strconv.FormatInt(port, 10))}
	if base != "" {
		u.Path = "/" + base
	}
	return InstanceAddress(u.String())
}

func seerrLinked(ctx context.Context, src Source) ([]Linked, error) {
	c, err := seerrReader{kind: src.Kind}.client(src)
	if err != nil {
		return nil, err
	}
	var out []Linked
	for _, k := range []Kind{Sonarr, Radarr} {
		var list []map[string]any
		if err := c.getJSON(ctx, "/api/v1/settings/"+string(k), nil, &list); err != nil {
			return nil, err
		}
		for _, m := range list {
			ssl, _ := m["useSsl"].(bool)
			four, _ := m["is4k"].(bool)
			a := linkedAddress(str(m["hostname"]), num(m["port"]), ssl, str(m["baseUrl"]))
			if a == "" {
				continue
			}
			key := str(m["apiKey"])
			out = append(out, Linked{Kind: k, Name: str(m["name"]), Address: a, FourK: four, HasKey: key != "", key: key})
		}
	}
	return out, nil
}

// prowlarrLinked lists Prowlarr's synced apps. Prowlarr hides their keys,
// so these give the address only.
func prowlarrLinked(ctx context.Context, src Source) ([]Linked, error) {
	c, err := arrReader{kind: Prowlarr}.client(src)
	if err != nil {
		return nil, err
	}
	var list []map[string]any
	if err := c.getJSON(ctx, "/api/v1/applications", nil, &list); err != nil {
		return nil, err
	}
	var out []Linked
	for _, m := range list {
		k := Kind(strings.ToLower(str(m["implementation"])))
		if !ValidKind(k) {
			continue
		}
		fields, _ := m["fields"].([]any)
		for _, f := range fields {
			fm, _ := f.(map[string]any)
			if str(fm["name"]) != "baseUrl" {
				continue
			}
			if a := InstanceAddress(str(fm["value"])); a != "" {
				out = append(out, Linked{Kind: k, Name: str(m["name"]), Address: a})
			}
		}
	}
	return out, nil
}
