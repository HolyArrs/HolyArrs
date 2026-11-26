package importer

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

func TestPluginImportOnlyInGrantedFoldersTable(t *testing.T) {
	dir := t.TempDir()
	granted := filepath.Join(dir, "downloads")
	other := filepath.Join(dir, "config")
	for _, d := range []string{granted, other} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(granted, "escape")
	_ = os.Symlink(other, link) // may need rights on Windows; the check below holds either way
	h := &Handlers{Deps: Deps{PluginFolders: func(id string) []string {
		if id == "imp" {
			return []string{granted}
		}
		return nil
	}}}
	for _, c := range []struct {
		name   string
		who    webserver.Caller
		path   string
		passes bool
	}{
		{"person anywhere", webserver.Caller{Via: "session", UserID: 1}, other, true},
		{"plugin inside", webserver.Caller{Via: "plugin", PluginID: "imp"}, filepath.Join(granted, "a.mkv"), true},
		{"plugin the folder itself", webserver.Caller{Via: "plugin", PluginID: "imp"}, granted, true},
		{"plugin outside", webserver.Caller{Via: "plugin", PluginID: "imp"}, other, false},
		{"plugin climbing", webserver.Caller{Via: "plugin", PluginID: "imp"}, granted + string(filepath.Separator) + ".." + string(filepath.Separator) + "config", false},
		{"plugin import places list", webserver.Caller{Via: "plugin", PluginID: "imp"}, "", false},
		{"plugin relative", webserver.Caller{Via: "plugin", PluginID: "imp"}, "downloads", false},
		{"other plugin", webserver.Caller{Via: "plugin", PluginID: "x"}, filepath.Join(granted, "a.mkv"), false},
	} {
		r := httptest.NewRequest("POST", "/", strings.NewReader(""))
		r = r.WithContext(webserver.ContextWith(r.Context(), c.who))
		w := httptest.NewRecorder()
		if got := h.pluginMay(w, r, c.path); got != c.passes || (!got && w.Code != http.StatusForbidden) {
			t.Errorf("%s: %v %d", c.name, got, w.Code)
		}
	}
	if fi, err := os.Lstat(link); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		r := httptest.NewRequest("POST", "/", nil)
		r = r.WithContext(webserver.ContextWith(r.Context(), webserver.Caller{Via: "plugin", PluginID: "imp"}))
		if h.pluginMay(httptest.NewRecorder(), r, link) {
			t.Error("a link out of the granted folder counted as inside")
		}
	}
}
