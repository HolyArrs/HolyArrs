package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// BridgeProwlarr is a client for a Prowlarr the person runs. HolyArrs adds itself
// there as an app for each side that is on (the Sonarr kind for TV, the
// Radarr kind for movies), then asks it to sync. Prowlarr then writes its
// sites into HolyArrs through the arr API.
type BridgeProwlarr struct {
	URL  string
	Key  string
	http *http.Client
}

// BridgeSide is one HolyArrs side that Prowlarr connects to.
type BridgeSide struct {
	Kind string // Sonarr or Radarr, the app type name in Prowlarr
	Name string // the entry's name in Prowlarr
	Base string // the address Prowlarr uses to reach this side of HolyArrs
	Key  string // the arr key; empty keeps the one Prowlarr has
}

// app is one connected-app entry, kept as loose JSON so fields HolyArrs does
// not know are sent back unchanged.
type app map[string]any

// NewBridgeProwlarr returns a client for one Prowlarr.
func NewBridgeProwlarr(url, key string) *BridgeProwlarr {
	return &BridgeProwlarr{URL: strings.TrimRight(url, "/"), Key: key, http: client(30 * time.Second)}
}

// Status checks the key and that the address is a Prowlarr, and returns its
// version.
func (p *BridgeProwlarr) Status(ctx context.Context) (string, error) {
	var st struct {
		AppName string `json:"appName"`
		Version string `json:"version"`
	}
	if err := p.do(ctx, http.MethodGet, "/api/v1/system/status", nil, &st); err != nil {
		return "", err
	}
	if !strings.EqualFold(st.AppName, "Prowlarr") {
		return "", &BridgeError{Words: "That address answers, but not like Prowlarr.", Detail: "app name " + clip(st.AppName, 40)}
	}
	return st.Version, nil
}

// Connect adds or updates HolyArrs's entry for one side. An entry that
// already points at the same address is changed, never doubled. A new entry
// starts from Prowlarr's own template so its other settings keep their
// defaults, the categories to sync among them.
func (p *BridgeProwlarr) Connect(ctx context.Context, s BridgeSide) error {
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
		// Prowlarr wants unique names. An entry with our name that points
		// elsewhere (another HolyArrs) is left alone, and this one gets its
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

// Sync asks Prowlarr to write its sites into every connected app now, the
// same as its "Sync App Indexers" button.
func (p *BridgeProwlarr) Sync(ctx context.Context) error {
	return p.do(ctx, http.MethodPost, "/api/v1/command", map[string]any{"name": "ApplicationIndexerSync", "forceSync": true}, nil)
}

// Connected says which sides already have an entry pointing at HolyArrs.
// bases maps a side's kind to the address Prowlarr uses for it.
func (p *BridgeProwlarr) Connected(ctx context.Context, bases map[string]string) (map[string]bool, error) {
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

// Disconnect removes the entries that point at these addresses. Prowlarr's
// next sync then takes its Sources away.
func (p *BridgeProwlarr) Disconnect(ctx context.Context, bases []string) error {
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

// do sends one request to Prowlarr, turns its failures into plain words and
// decodes a JSON answer into out when out is set.
func (p *BridgeProwlarr) do(ctx context.Context, method, path string, body any, out any) error {
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
		return fail("Prowlarr answered with status %d.", status)
	}
	if out != nil && json.Unmarshal(b, out) != nil {
		return fail("That address answers, but not like Prowlarr.")
	}
	return nil
}

// validationWords reads Prowlarr's list of refusals (each a property and a
// message) and keeps the first three messages.
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

// clip cuts s to n bytes and marks the cut.
func clip(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// fields returns the entry's settings list.
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

// field returns the value of the named setting, or nil.
func (a app) field(name string) any {
	for _, f := range a.fields() {
		if n, _ := f["name"].(string); strings.EqualFold(n, name) {
			return f["value"]
		}
	}
	return nil
}

// set changes the named setting, or adds it when missing.
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

// sameAddr compares two addresses ignoring case and a trailing slash.
func sameAddr(a, b string) bool {
	return strings.EqualFold(strings.TrimRight(strings.TrimSpace(a), "/"), strings.TrimRight(strings.TrimSpace(b), "/"))
}

// uniqueName returns name, or name with the address host added (and then a
// number) when an app in Prowlarr already uses it. The count stops at 99 so
// the loop always ends.
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
