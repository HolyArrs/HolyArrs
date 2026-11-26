package plugins_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/plugins"
	"github.com/HolyArrs/holyarrs/app/backend/internal/plugins/pluginstest"
)

// A failed install shows its short reason on the row until the next try,
// and a good install leaves no step or error behind.
func TestInstallErrorShownUntilRetry(t *testing.T) {
	ls, pub := newListServer(t)
	e, run := newEngine(t, ls, pub)
	run.Err = context.DeadlineExceeded
	m := mux(e)
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusInternalServerError)
	p := row(t, call(t, m, "GET", "/api/v1/plugins", http.StatusOK), "tailscale")
	if p["install_error"] == nil || p["install_step"] != nil {
		t.Fatalf("after a failed install: %v", p)
	}
	run.Err = nil
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)
	p = row(t, call(t, m, "GET", "/api/v1/plugins", http.StatusOK), "tailscale")
	if p["install_error"] != nil || p["install_step"] != nil || p["running"] != true {
		t.Fatalf("after the retry: %v", p)
	}
}

// A plugin that needs a newer app is listed with needs_app and refused.
func TestNeedsNewerApp(t *testing.T) {
	ls, pub := newListServer(t)
	e, _ := newEngine(t, ls, pub)
	e.AppVersion = "1.2.0"
	ls.publishWith("1.0.0", pluginFile, 1, func(m map[string]any) { m["min_app_version"] = "9.0.0" })
	m := mux(e)
	p := row(t, call(t, m, "GET", "/api/v1/plugins", http.StatusOK), "tailscale")
	if p["needs_app"] != "9.0.0" {
		t.Fatalf("needs_app: %v", p)
	}
	out := call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusUnprocessableEntity)
	if !strings.Contains(out["error"].(string), "9.0.0") {
		t.Fatalf("refusal: %v", out)
	}
	// A dev build skips the check.
	e2 := &plugins.Engine{Dir: t.TempDir(), ListURL: e.ListURL, Keys: e.Keys, Runner: &pluginstest.FakeRunner{}, HTTP: e.HTTP, AppVersion: "dev"}
	defer e2.StopAll()
	if _, err := e2.Install(context.Background(), "tailscale"); err != nil {
		t.Fatalf("dev build refused: %v", err)
	}
	// An unstamped lab build (0.0.0-skeleton) lists no needs_app; a real 1.2.1 release compares (1.2.0 needed: fine).
	for _, c := range []struct{ app, min, want string }{{"0.0.0-skeleton", "9.0.0", ""}, {"1.2.1", "1.2.0", ""}, {"1.2.1", "1.3.0", "1.3.0"}} {
		e3 := &plugins.Engine{Dir: t.TempDir(), ListURL: e.ListURL, Keys: e.Keys, Runner: &pluginstest.FakeRunner{}, HTTP: e.HTTP, AppVersion: c.app}
		ls.publishWith("1.0.0", pluginFile, 1, func(m map[string]any) { m["min_app_version"] = c.min })
		got, _ := row(t, call(t, mux(e3), "GET", "/api/v1/plugins", http.StatusOK), "tailscale")["needs_app"].(string)
		if got != c.want {
			t.Errorf("app %s min %s: needs_app %q, want %q", c.app, c.min, got, c.want)
		}
		e3.StopAll()
	}
}

// With a newer version listed, the row carries the installed version's
// access, so the page can show what the update adds.
func TestUpdateShowsInstalledAccess(t *testing.T) {
	ls, pub := newListServer(t)
	e, _ := newEngine(t, ls, pub)
	if _, err := e.Install(context.Background(), "tailscale"); err != nil {
		t.Fatal(err)
	}
	m := mux(e)
	if p := row(t, call(t, m, "GET", "/api/v1/plugins", http.StatusOK), "tailscale"); p["installed_access"] != nil {
		t.Fatalf("no update yet: %v", p)
	}
	ls.publishWith("1.1.0", []byte("version two"), 2, func(m map[string]any) {
		m["access"] = map[string]any{"hosts": []string{"controlplane.tailscale.com", "log.tailscale.io"}, "folders": []string{}, "actions": []string{}}
	})
	if err := e.CheckBlocklist(context.Background()); err != nil {
		t.Fatal(err)
	}
	p := row(t, call(t, m, "GET", "/api/v1/plugins", http.StatusOK), "tailscale")
	ia, _ := p["installed_access"].(map[string]any)
	if p["version"] != "1.1.0" || ia == nil || len(ia["hosts"].([]any)) != 1 || len(p["access"].(map[string]any)["hosts"].([]any)) != 2 {
		t.Fatalf("update row: %v", p)
	}
}
