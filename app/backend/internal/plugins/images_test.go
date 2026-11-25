package plugins_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The page gets plugin logos and developer pictures from this server only;
// the server takes them from the plugin site and passes only real WebP files.
func TestPluginImages(t *testing.T) {
	webp := append([]byte("RIFF\x10\x00\x00\x00WEBPVP8 "), make([]byte, 8)...)
	ls, pub := newListServer(t)
	site := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/list/images/tailscale/icon-256.webp", "/list/images/avatars/CoderLuii-64.webp":
			w.Write(webp)
		case "/list/images/fake/icon-128.webp":
			w.Write([]byte("<svg onload=alert(1)>"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(site.Close)
	ls.srv = site
	e, _ := newEngine(t, ls, pub)
	m := mux(e)
	for path, want := range map[string]int{
		"/api/v1/plugin-images/images/tailscale/icon-256.webp":   200,
		"/api/v1/plugin-images/images/avatars/CoderLuii-64.webp": 200,
		"/api/v1/plugin-images/images/fake/icon-128.webp":        404, // not a WebP file
		"/api/v1/plugin-images/images/tailscale/icon-64.webp":    404, // not a size the robot makes
		"/api/v1/plugin-images/plugins.json":                     404, // only pictures
		"/api/v1/plugin-images/images/missing/icon-128.webp":     404,
	} {
		rec := httptest.NewRecorder()
		m.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != want {
			t.Fatalf("GET %s = %d, want %d", path, rec.Code, want)
		}
		if want == 200 && (rec.Header().Get("Content-Type") != "image/webp" || rec.Header().Get("X-Content-Type-Options") != "nosniff") {
			t.Fatalf("GET %s headers %v", path, rec.Header())
		}
	}
}
