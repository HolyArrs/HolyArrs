package plugins_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/plugins"
)

func TestRefreshPicksUpNewListAndIsRateLimited(t *testing.T) {
	ls, pub := newListServer(t)
	e, _ := newEngine(t, ls, pub)
	h := &plugins.Handlers{Engine: e}
	m := http.NewServeMux()
	h.Routes(m)

	// An API key, even an admin one, may not press Refresh.
	req := httptest.NewRequest("POST", "/api/v1/plugins/refresh", nil)
	req = req.WithContext(webserver.ContextWith(req.Context(), webserver.Caller{KeyID: 7, Scopes: []string{webserver.ScopeAdmin}}))
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("refresh with an admin key = %d %s", rec.Code, rec.Body)
	}

	// First press fetches the list (counter 1 is new to this install).
	out := call(t, m, "POST", "/api/v1/plugins/refresh", http.StatusOK)
	if out["changed"] != true {
		t.Fatalf("first refresh: %v", out)
	}
	row(t, out["plugins"].(map[string]any), "tailscale")

	// A new plugin is published: inside 30 s nothing is fetched.
	zero := uint64(0)
	ls.mu.Lock()
	ls.list = ls.sign(plugins.List{Kind: plugins.KindPlugins, Counter: 2, MinBlocklistCounter: &zero, Entries: []plugins.Entry{
		{ID: "tailscale", Version: "1.0.0", SHA256: hashOf(pluginFile), ManifestSHA256: hashOf(ls.manifest)},
		{ID: "newone", Version: "0.1.0", SHA256: hashOf([]byte("x")), ManifestSHA256: hashOf([]byte("y"))},
	}})
	ls.mu.Unlock()
	out = call(t, m, "POST", "/api/v1/plugins/refresh", http.StatusOK)
	if out["changed"] != false {
		t.Fatalf("refresh inside the limit fetched: %v", out)
	}

	// After the limit the new list is picked up and listed.
	plugins.SetRefreshAt(h, time.Now().Add(-plugins.RefreshEvery-time.Second))
	out = call(t, m, "POST", "/api/v1/plugins/refresh", http.StatusOK)
	if out["changed"] != true {
		t.Fatalf("refresh after the limit: %v", out)
	}
	row(t, out["plugins"].(map[string]any), "newone")

	// The same list again is already up to date.
	plugins.SetRefreshAt(h, time.Time{})
	if out = call(t, m, "POST", "/api/v1/plugins/refresh", http.StatusOK); out["changed"] != false {
		t.Fatalf("refresh with no new list: %v", out)
	}

	// The plugin site down answers 502.
	plugins.SetRefreshAt(h, time.Time{})
	ls.srv.Close()
	call(t, m, "POST", "/api/v1/plugins/refresh", http.StatusBadGateway)
}
