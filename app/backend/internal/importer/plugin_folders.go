package importer

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// pluginMay lets a person through, and a plugin only for a path inside a
// folder the owner granted it (after links are followed). It answers 403
// itself when not.
func (h *Handlers) pluginMay(w http.ResponseWriter, r *http.Request, p string) bool {
	who, ok := webserver.From(r.Context())
	if !ok || who.Via != "plugin" {
		return true
	}
	var roots []string
	if h.PluginFolders != nil {
		roots = h.PluginFolders(who.PluginID)
	}
	if p != "" && insideFolders(p, roots) {
		return true
	}
	webserver.Fail(w, http.StatusForbidden, "plugins may only use folders the owner gave them")
	return false
}

// insideFolders says whether p, links followed, is one of roots or inside one.
func insideFolders(p string, roots []string) bool {
	if !filepath.IsAbs(p) {
		return false
	}
	if real, err := filepath.EvalSymlinks(p); err == nil {
		p = real
	}
	p = filepath.Clean(p)
	for _, r := range roots {
		if real, err := filepath.EvalSymlinks(r); err == nil {
			r = real
		}
		rel, err := filepath.Rel(filepath.Clean(r), p)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
			return true
		}
	}
	return false
}
