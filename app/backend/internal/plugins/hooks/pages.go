package hooks

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// PageCSP is sent with every plugin page file. The sandbox gives the page an
// opaque origin: it can't read HolyArrs' cookies or storage, can't call the
// API (connect-src 'none') and talks to HolyArrs only by postMessage, which
// the bridge checks against the plugin's scopes.
const PageCSP = "sandbox allow-scripts allow-forms; default-src 'none'; script-src 'self' 'unsafe-inline'; " +
	"style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; media-src 'self' blob:; font-src 'self' data:; " +
	"connect-src 'none'; form-action 'none'; base-uri 'none'; frame-ancestors 'self'"

func (h *Handlers) pages(w http.ResponseWriter, r *http.Request) {
	ps := h.Reg.Pages()
	if ps == nil {
		ps = []PluginPage{}
	}
	webserver.WriteJSONNoStore(w, http.StatusOK, map[string]any{"pages": ps})
}

// openPage gives a short-lived address for one plugin page; the sandboxed
// frame loads its files from there.
func (h *Handlers) openPage(w http.ResponseWriter, r *http.Request) {
	pl, pg := r.PathValue("plugin"), r.PathValue("page")
	p, ok := h.Reg.page(pl, pg)
	if !ok || !h.Reg.granted(pl, "pages.add") {
		webserver.Fail(w, http.StatusNotFound, "no such plugin page")
		return
	}
	tok, until := h.pageToks.issue(pl, pg)
	webserver.WriteJSONNoStore(w, http.StatusOK, map[string]any{"url": "/api/v1/plugin-page-files/" + tok + "/" + p.File, "expires": until})
}

// pageFile hands one file of a plugin page, fetched from the plugin.
func (h *Handlers) pageFile(w http.ResponseWriter, r *http.Request) {
	pl, ok := h.pageToks.owner(r.PathValue("token"))
	if !ok || !h.Reg.granted(pl, "pages.add") {
		webserver.Fail(w, http.StatusNotFound, "this page has ended; open it again")
		return
	}
	f := r.PathValue("path")
	if !safeFile(f) {
		webserver.Fail(w, http.StatusNotFound, "no such file")
		return
	}
	raw, err := h.Reg.Broker.Call(r.Context(), pl, Pages, "file", map[string]string{"path": f}, 10*time.Second)
	var a FeedAnswer
	if err != nil || json.Unmarshal(raw, &a) != nil || a.Status != http.StatusOK || len(a.Body) > feedMax {
		webserver.Fail(w, http.StatusNotFound, "no such file")
		return
	}
	w.Header().Set("Content-Security-Policy", PageCSP)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", pageType(f))
	_, _ = w.Write(a.Body)
}

// pageType picks the type by name only, never from the plugin's word.
func pageType(f string) string {
	switch path.Ext(f) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".js", ".mjs":
		return "text/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".woff2":
		return "font/woff2"
	case ".json":
		return "application/json"
	}
	return "application/octet-stream"
}

// bridge runs one API call a plugin page asked for by postMessage, as the
// plugin itself: the Gate holds it to the plugin's scopes, whoever is
// signed in.
func (h *Handlers) bridge(w http.ResponseWriter, r *http.Request) {
	pl := r.PathValue("plugin")
	who, ok := webserver.From(r.Context())
	if !ok || who.Via == "plugin" || who.KeyID != 0 {
		webserver.Fail(w, http.StatusForbidden, "plugin pages are used from a signed-in session")
		return
	}
	if !h.hasPages(pl) || h.AsPlugin == nil || h.BridgeAllowed == nil {
		webserver.Fail(w, http.StatusNotFound, "no such plugin page")
		return
	}
	var in struct {
		Method string          `json:"method"`
		Path   string          `json:"path"`
		Body   json.RawMessage `json:"body,omitempty"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&in); err != nil ||
		!strings.HasPrefix(in.Path, "/api/v1/") || strings.Contains(in.Path, "..") || strings.Contains(in.Path, "//") {
		webserver.Fail(w, http.StatusBadRequest, `send {"method": "GET", "path": "/api/v1/..."}`)
		return
	}
	// A page never gets a stream link or a way to make one: the app's own
	// player asks for it (play) and shows the video itself.
	if strings.HasPrefix(in.Path, "/api/v1/plugin-hooks/stream") || strings.HasPrefix(in.Path, "/api/v1/plugin-stream/") || strings.HasPrefix(in.Path, "/api/v1/plugin-pages/") {
		webserver.Fail(w, http.StatusNotFound, "no such plugin page")
		return
	}
	switch in.Method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete:
	default:
		webserver.Fail(w, http.StatusBadRequest, "method must be GET, POST, PUT or DELETE")
		return
	}
	req := httptest.NewRequestWithContext(r.Context(), in.Method, in.Path, bytes.NewReader(in.Body))
	req.RemoteAddr = r.RemoteAddr // the person's own address, never loopback
	if len(in.Body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	// Both must hold: the plugin its scope and the person the route's own
	// permission, so a page can't lift a low-rights person (A5 overlap).
	if err := h.BridgeAllowed(pl, who, req); err != nil {
		webserver.Fail(w, http.StatusForbidden, err.Error())
		return
	}
	resp, err := h.AsPlugin(pl, req)
	if err != nil {
		webserver.Fail(w, http.StatusForbidden, err.Error())
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, feedMax))
	var v any
	if json.Unmarshal(body, &v) != nil {
		v = nil
	}
	webserver.WriteJSONNoStore(w, http.StatusOK, map[string]any{"status": resp.StatusCode, "body": v})
}

// hasPages says whether pl declared pages and still holds pages.add.
func (h *Handlers) hasPages(pl string) bool {
	for _, p := range h.Reg.Pages() {
		if p.Plugin == pl {
			return true
		}
	}
	return false
}

// play makes the stream link for the app's own player, which the app's page
// (not the plugin page) asks for after a plugin page called play({file_id}).
// The link goes back to the app's page only and plays with the person's own
// session (see streamFile). The plugin must hold media.stream and pages.add, the
// file be inside a folder it was given, and the person be one who may use
// the route and has no age limit.
func (h *Handlers) play(w http.ResponseWriter, r *http.Request) {
	pl := r.PathValue("plugin")
	who, ok := webserver.From(r.Context())
	if !ok || who.Via == "plugin" || who.KeyID != 0 || who.UserID == 0 {
		webserver.Fail(w, http.StatusForbidden, "plugin pages are used from a signed-in session")
		return
	}
	if !h.hasPages(pl) || !h.Reg.granted(pl, "pages.add") || h.BridgeAllowed == nil {
		webserver.Fail(w, http.StatusNotFound, "no such plugin page")
		return
	}
	if scope := "media.stream"; !h.Reg.granted(pl, scope) {
		webserver.Fail(w, http.StatusForbidden, "this plugin was not given the "+scope+" permission")
		return
	}
	probe := httptest.NewRequestWithContext(r.Context(), http.MethodPost, "/api/v1/plugin-hooks/stream", nil)
	if err := h.BridgeAllowed(pl, who, probe); err != nil {
		webserver.Fail(w, http.StatusForbidden, err.Error())
		return
	}
	if who.MaxRating != "" {
		webserver.Fail(w, http.StatusForbidden, "your account may not do this; ask the owner")
		return
	}
	var in struct {
		FileID int64 `json:"file_id"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&in); err != nil || in.FileID <= 0 {
		webserver.Fail(w, http.StatusBadRequest, `send {"file_id": 123}`)
		return
	}
	if h.StreamFile == nil {
		webserver.Fail(w, http.StatusServiceUnavailable, "not available")
		return
	}
	if _, err := h.StreamFile(r.Context(), pl, in.FileID); err != nil {
		webserver.Fail(w, http.StatusForbidden, err.Error())
		return
	}
	tok, until := h.Reg.streams.issue(pl, strconv.FormatInt(in.FileID, 10))
	webserver.WriteJSONNoStore(w, http.StatusOK, map[string]any{"url": "/api/v1/plugin-stream/" + tok, "expires": until})
}
