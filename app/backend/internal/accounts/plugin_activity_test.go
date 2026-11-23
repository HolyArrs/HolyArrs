package accounts

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

func TestPluginOpenCallCapAndActivity(t *testing.T) {
	IssuePluginToken("busy", []string{"library.read"})
	t.Cleanup(func() { RevokePluginToken("busy") })
	release := make(chan struct{})
	entered := make(chan struct{}, pluginInflightMax)
	m := http.NewServeMux()
	m.HandleFunc("GET /api/v1/titles", func(w http.ResponseWriter, r *http.Request) { entered <- struct{}{}; <-release })
	m.HandleFunc("GET /api/v1/asks", func(w http.ResponseWriter, r *http.Request) {})
	g := Gate(m)
	do := func(path string) int {
		r := httptest.NewRequest("GET", path, nil)
		r = r.WithContext(webserver.ContextWith(r.Context(), webserver.Caller{Via: "plugin", PluginID: "busy", Scopes: []string{"library.read"}}))
		w := httptest.NewRecorder()
		g.ServeHTTP(w, r)
		return w.Code
	}
	var wg sync.WaitGroup
	for range pluginInflightMax {
		wg.Add(1)
		go func() { defer wg.Done(); do("/api/v1/titles") }()
	}
	for range pluginInflightMax {
		<-entered
	}
	if c := do("/api/v1/titles"); c != http.StatusTooManyRequests {
		t.Errorf("call over the open-call cap: %d", c)
	}
	close(release)
	wg.Wait()
	if c := do("/api/v1/asks"); c != http.StatusForbidden {
		t.Errorf("refused call: %d", c)
	}
	calls, refused, limited := PluginActivity("busy")
	if len(calls) != pluginInflightMax+2 || refused != 1 || limited != 1 || calls[0].Outcome != "refused" {
		t.Errorf("activity %d calls, %d refused, %d limited, newest %+v", len(calls), refused, limited, calls[0])
	}
	for range activityKeep + 5 {
		recordPluginCall("busy", "GET", "/x", "allowed", calls[0].At)
	}
	if c, _, _ := PluginActivity("busy"); len(c) != activityKeep {
		t.Errorf("kept %d", len(c))
	}
}
