package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Prowlarr is a Prowlarr the person runs. HolyArrs adds itself there as an
// app on each side whose part is on (the Sonarr kind for TV, the Radarr kind
// for movies, pointing at HolyArrs's /sonarr and /radarr), then asks it to
// sync; Prowlarr then writes its sites into HolyArrs through the old-app API
// (#85). Calls and fields: research/upstream/prowlarr/manual/backend/api.md,
// "Connected apps and the sync".
type Prowlarr struct {
	URL  string
	Key  string
	http *http.Client
}

// NewProwlarr is a client for one Prowlarr.
func NewProwlarr(url, key string) *Prowlarr {
	return &Prowlarr{URL: strings.TrimRight(url, "/"), Key: key, http: client(30 * time.Second)}
}

func (p *Prowlarr) do(ctx context.Context, method, path string, body any, out any) error {
	status, b, err := call(ctx, p.http, method, p.URL+path, map[string]string{"X-Api-Key": p.Key}, body, 8<<20)
	if err != nil {
		return err
	}
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return fail("Prowlarr turned down the key. Copy it again from Prowlarr's Settings > General.")
	case status == http.StatusNotFound:
		return fail("That address answers, but not like Prowlarr. Check the address (and its URL base, if Prowlarr has one).")
	case status == http.StatusBadRequest:
		return fail("Prowlarr refused it: %s", validationWords(b))
	case status >= 300:
		return fail("Prowlarr answered with an error (%d).", status)
	}
	if out != nil && json.Unmarshal(b, out) != nil {
		return fail("That address answers, but not like Prowlarr.")
	}
	return nil
}

// validationWords reads Prowlarr's list of refusals (each a property and a message).
func validationWords(b []byte) string {
	var list []struct {
		PropertyName string `json:"propertyName"`
		ErrorMessage string `json:"errorMessage"`
	}
	if json.Unmarshal(b, &list) == nil && len(list) > 0 {
		var parts []string
		for _, v := range list[:min(len(list), 3)] {
			if m := strings.TrimSpace(v.ErrorMessage); m != "" {
				parts = append(parts, m)
			}
		}
		if len(parts) > 0 {
			return clip(strings.Join(parts, "; "), 300)
		}
	}
	return "it gave no reason"
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// Status checks the key and that it is Prowlarr; it answers the version.
func (p *Prowlarr) Status(ctx context.Context) (string, error) {
	var st struct {
		AppName string `json:"appName"`
		Version string `json:"version"`
	}
	if err := p.do(ctx, http.MethodGet, "/api/v1/system/status", nil, &st); err != nil {
		return "", err
	}
	if !strings.EqualFold(st.AppName, "Prowlarr") {
		if st.AppName != "" {
			return "", fail("That is %s, not Prowlarr.", clip(st.AppName, 40))
		}
		return "", fail("That address answers, but not like Prowlarr.")
	}
	return st.Version, nil
}

// app is one connected-app entry as Prowlarr sends and takes it.
type app map[string]any

func (a app) fields() []map[string]any {
	raw, _ := a["fields"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, f := range raw {
		if m, ok := f.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func (a app) field(name string) any {
	for _, f := range a.fields() {
		if n, _ := f["name"].(string); strings.EqualFold(n, name) {
			return f["value"]
		}
	}
	return nil
}

func (a app) set(name string, v any) {
	for _, f := range a.fields() {
		if n, _ := f["name"].(string); strings.EqualFold(n, name) {
			f["value"] = v
			return
		}
	}
	raw, _ := a["fields"].([]any)
	a["fields"] = append(raw, map[string]any{"name": name, "value": v})
}

func sameAddr(a, b string) bool {
	return strings.EqualFold(strings.TrimRight(strings.TrimSpace(a), "/"), strings.TrimRight(strings.TrimSpace(b), "/"))
}

// Side is one HolyArrs side Prowlarr connects to.
type Side struct {
	Kind string // Sonarr or Radarr (Prowlarr's implementation name)
	Name string // the entry's name in Prowlarr
	Base string // HolyArrs's address for it, as Prowlarr reaches HolyArrs
	Key  string // the old-app key; "" keeps the one Prowlarr has
}

// Connect adds or updates HolyArrs's entry for one side. An entry already
// pointing at the same address is changed, never doubled. The entry is
// built from Prowlarr's own template, so its other settings keep their
// defaults (the categories to sync among them).
func (p *Prowlarr) Connect(ctx context.Context, s Side) error {
	var have []app
	if err := p.do(ctx, http.MethodGet, "/api/v1/applications", nil, &have); err != nil {
		return err
	}
	var entry app
	for _, a := range have {
		if impl, _ := a["implementation"].(string); strings.EqualFold(impl, s.Kind) {
			if base, _ := a.field("baseUrl").(string); sameAddr(base, s.Base) {
				entry = a
				break
			}
		}
	}
	method, path := http.MethodPut, ""
	if entry == nil {
		var templates []app
		if err := p.do(ctx, http.MethodGet, "/api/v1/applications/schema", nil, &templates); err != nil {
			return err
		}
		for _, t := range templates {
			if impl, _ := t["implementation"].(string); strings.EqualFold(impl, s.Kind) {
				entry = t
				break
			}
		}
		if entry == nil {
			return fail("This Prowlarr can't connect to %s apps.", s.Kind)
		}
		delete(entry, "id")
		method, path = http.MethodPost, "/api/v1/applications"
		// Prowlarr wants unique names. An entry with ours that points
		// elsewhere (another HolyArrs) is left alone; this one gets its
		// address in the name.
		s.Name = uniqueName(have, s.Name, s.Base)
	} else {
		path = fmt.Sprintf("/api/v1/applications/%v", entry["id"])
	}
	entry["name"] = s.Name
	entry["syncLevel"] = "fullSync"
	entry["enable"] = true
	delete(entry, "presets")
	entry.set("prowlarrUrl", p.URL)
	entry.set("baseUrl", s.Base)
	if s.Key != "" {
		entry.set("apiKey", s.Key)
	}
	return p.do(ctx, method, path, entry, nil)
}

// Sync asks Prowlarr to write its sites into every connected app now (the
// "Sync App Indexers" button).
func (p *Prowlarr) Sync(ctx context.Context) error {
	return p.do(ctx, http.MethodPost, "/api/v1/command", map[string]any{"name": "ApplicationIndexerSync", "forceSync": true}, nil)
}

// Connected says which sides already have an entry pointing at HolyArrs.
func (p *Prowlarr) Connected(ctx context.Context, bases map[string]string) (map[string]bool, error) {
	var have []app
	if err := p.do(ctx, http.MethodGet, "/api/v1/applications", nil, &have); err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for kind, base := range bases {
		for _, a := range have {
			impl, _ := a["implementation"].(string)
			b, _ := a.field("baseUrl").(string)
			if strings.EqualFold(impl, kind) && sameAddr(b, base) {
				out[kind] = true
			}
		}
	}
	return out, nil
}

// Disconnect removes HolyArrs's entries from Prowlarr (the ones pointing at
// these addresses); Prowlarr's next sync then takes its Sources away.
func (p *Prowlarr) Disconnect(ctx context.Context, bases []string) error {
	var have []app
	if err := p.do(ctx, http.MethodGet, "/api/v1/applications", nil, &have); err != nil {
		return err
	}
	for _, a := range have {
		b, _ := a.field("baseUrl").(string)
		for _, base := range bases {
			if sameAddr(b, base) {
				if err := p.do(ctx, http.MethodDelete, fmt.Sprintf("/api/v1/applications/%v", a["id"]), nil, nil); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// uniqueName returns name, or name with the address's host added (and a
// number after that) when an app in Prowlarr already uses it.
func uniqueName(have []app, name, base string) string {
	taken := func(n string) bool {
		for _, a := range have {
			if got, _ := a["name"].(string); strings.EqualFold(strings.TrimSpace(got), n) {
				return true
			}
		}
		return false
	}
	if !taken(name) {
		return name
	}
	host := base
	if u, err := url.Parse(base); err == nil && u.Host != "" {
		host = u.Host
	}
	n := name + " " + host
	for i := 2; taken(n) && i < 100; i++ {
		n = fmt.Sprintf("%s %s %d", name, host, i)
	}
	return n
}
