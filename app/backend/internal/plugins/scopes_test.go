package plugins_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/plugins"
)

func TestManifestScopes(t *testing.T) {
	base := `{"id":"x","name":"X","version":"1.0.0","author":"a","min_app_version":"1.3.0","sha256":"` + strings.Repeat("a", 64) + `","access":{"hosts":[],"folders":[],"actions":[%A]}}`
	for _, c := range []struct {
		actions string
		ok      bool
	}{
		{``, true},
		{`"library.read"`, true},
		{`"library.read","library.music.read","network.forward"`, true},
		{`"files.read"`, true},
		{`"settings.read"`, false},
		{`"plugins.change"`, false},
		{`"auth.change"`, false},
		{`"library.read","library.read"`, false},
		{`"made.up.read"`, false},
		{`"Library.read"`, false},
	} {
		_, err := plugins.ParseManifest([]byte(strings.Replace(base, "%A", c.actions, 1)))
		if (err == nil) != c.ok {
			t.Errorf("actions [%s]: err %v, want ok %v", c.actions, err, c.ok)
		}
	}
}

// An update that asks for a new scope waits for the owner; the approved
// scopes are what a start gets.
func TestUpdateWithNewScopeIsHeld(t *testing.T) {
	ls, pub := newListServer(t)
	e, _ := newEngine(t, ls, pub)
	m := mux(e)
	ls.publishWith("1.0.0", []byte("v100s"), 2, func(man map[string]any) {
		man["min_app_version"] = "1.3.0"
		man["access"] = map[string]any{"hosts": []string{"controlplane.tailscale.com"}, "folders": []string{}, "actions": []string{"library.read"}}
	})
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)
	if got := e.GrantedScopes("tailscale"); !slices.Equal(got, []string{"library.read"}) {
		t.Fatalf("granted %v", got)
	}
	ls.publishWith("1.0.1", []byte("v101s"), 3, func(man map[string]any) {
		man["min_app_version"] = "1.3.0"
		man["access"] = map[string]any{"hosts": []string{"controlplane.tailscale.com"}, "folders": []string{}, "actions": []string{"library.read", "library.change"}}
	})
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusUnprocessableEntity)
	if n := needs(t, m); len(n) != 1 || n[0] != "scope:library.change" {
		t.Fatalf("needs = %v", n)
	}
	if got := e.GrantedScopes("tailscale"); !slices.Equal(got, []string{"library.read"}) {
		t.Fatalf("while the update waits, the running version keeps %v", got)
	}
	if ov := call(t, m, "GET", "/api/v1/plugins", http.StatusOK); row(t, ov, "tailscale")["installed_version"] != "1.0.0" {
		t.Fatalf("the old version should stay installed: %v", row(t, ov, "tailscale")["installed_version"])
	}
	callBody(t, m, "POST", "/api/v1/plugins/tailscale/approve", `{"folders":{}}`, http.StatusOK)
	if got := e.GrantedScopes("tailscale"); !slices.Equal(got, []string{"library.change", "library.read"}) {
		t.Fatalf("after approval %v", got)
	}
	if ov := call(t, m, "GET", "/api/v1/plugins", http.StatusOK); row(t, ov, "tailscale")["installed_version"] != "1.0.1" {
		t.Fatalf("approval should install the waiting update: %v", row(t, ov, "tailscale")["installed_version"])
	}
}

// The asked caps reach the start once approved, and the owner's own caps
// win on the next start.
func TestResourcesAppliedAndOwnerWins(t *testing.T) {
	ls, pub := newListServer(t)
	e, run := newEngine(t, ls, pub)
	m := mux(e)
	ls.publishWith("1.0.0", []byte("v100r"), 2, func(man map[string]any) {
		man["min_app_version"] = "1.3.0"
		man["resources"] = map[string]any{"processes": 512}
	})
	// The install sheet showed the caps, so a first install approves them.
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)
	if got := run.Starts[len(run.Starts)-1].Limits; got.Processes != 512 || got.CPUs != 1 {
		t.Fatalf("approved start limits %+v", got)
	}
	callBody(t, m, "POST", "/api/v1/plugins/tailscale/approve", `{"folders":{},"limits":{"processes":64}}`, http.StatusOK)
	call(t, m, "POST", "/api/v1/plugins/tailscale/stop", http.StatusOK)
	call(t, m, "POST", "/api/v1/plugins/tailscale/start", http.StatusOK)
	if got := run.Starts[len(run.Starts)-1].Limits; got.Processes != 64 {
		t.Fatalf("owner limits not applied on restart: %+v", got)
	}
	callBody(t, m, "POST", "/api/v1/plugins/tailscale/approve", `{"folders":{},"limits":{"memory_mib":65536,"cpus":64}}`, http.StatusUnprocessableEntity)
	// An update that asks for more waits for the owner again.
	ls.publishWith("1.0.1", []byte("v101r"), 3, func(man map[string]any) {
		man["min_app_version"] = "1.3.0"
		man["resources"] = map[string]any{"processes": 1024}
	})
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusUnprocessableEntity)
	if n := needs(t, m); len(n) != 1 || n[0] != "resources" {
		t.Fatalf("needs = %v", n)
	}
}
