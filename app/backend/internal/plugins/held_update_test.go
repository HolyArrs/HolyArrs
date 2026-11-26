package plugins_test

import (
	"crypto/ed25519"
	"net/http"
	"slices"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/plugins"
)

// An update that asks for more waits; the running version keeps running
// with what it had until the owner approves.
func TestHeldUpdateKeepsOldRunning(t *testing.T) {
	ls, pub := newListServer(t)
	e, _ := newEngine(t, ls, pub)
	m := mux(e)
	ls.publishWith("1.0.0", []byte("held100"), 2, func(man map[string]any) {
		man["min_app_version"] = "1.3.0"
		man["access"] = map[string]any{"hosts": []string{"controlplane.tailscale.com"}, "folders": []string{}, "actions": []string{"library.read"}}
	})
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)
	ls.publishWith("1.0.1", []byte("held101"), 3, func(man map[string]any) {
		man["min_app_version"] = "1.3.0"
		man["access"] = map[string]any{"hosts": []string{"controlplane.tailscale.com"}, "folders": []string{}, "actions": []string{"library.read", "files.delete"}}
	})
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusUnprocessableEntity)
	if p := row(t, call(t, m, "GET", "/api/v1/plugins", http.StatusOK), "tailscale"); p["install_error"] == nil {
		t.Fatalf("the held update shows no message: %v", p)
	}
	p := row(t, call(t, m, "GET", "/api/v1/plugins", http.StatusOK), "tailscale")
	if p["running"] != true || p["enabled"] != true || p["installed_version"] != "1.0.0" {
		t.Fatalf("the approved version stopped while the update waits: %v", p)
	}
	if got := e.GrantedScopes("tailscale"); !slices.Equal(got, []string{"library.read"}) {
		t.Fatalf("granted %v", got)
	}
	// Installing the same waiting update again changes nothing.
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusUnprocessableEntity)
	callBody(t, m, "POST", "/api/v1/plugins/tailscale/approve", `{"folders":{}}`, http.StatusOK)
	p = row(t, call(t, m, "GET", "/api/v1/plugins", http.StatusOK), "tailscale")
	if p["install_error"] != nil {
		t.Fatalf("the message stays after approval: %v", p["install_error"])
	}
	if p["installed_version"] != "1.0.1" || !slices.Contains(e.GrantedScopes("tailscale"), "files.delete") {
		t.Fatalf("after approval: %v %v", p["installed_version"], e.GrantedScopes("tailscale"))
	}
}

// An update signed by another trusted key than the installed version is
// refused.
func TestUpdateFromAnotherKeyRefused(t *testing.T) {
	ls, pub := newListServer(t)
	e, _ := newEngine(t, ls, pub)
	pub2, priv2, _ := ed25519.GenerateKey(nil)
	e.Keys = append(e.Keys, pub2)
	m := mux(e)
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)
	ls.mu.Lock()
	ls.priv = priv2
	ls.mu.Unlock()
	ls.publishWith("1.0.1", []byte("otherkey"), 3, nil)
	ls.mu.Lock()
	ls.block = ls.sign(plugins.List{Kind: plugins.KindBlocklist, Counter: 2, Entries: []plugins.Entry{}})
	ls.mu.Unlock()
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusConflict)
}

// An install from before keys were kept learns its key from the signed list
// it was installed from, so a later update by the same key goes through.
func TestOldInstallLearnsItsKey(t *testing.T) {
	ls, pub := newListServer(t)
	e, _ := newEngine(t, ls, pub)
	m := mux(e)
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)
	if err := plugins.ClearKeyForTest(e, "tailscale"); err != nil {
		t.Fatal(err)
	}
	ls.publish("1.0.1", []byte("samekey"), 2)
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)
	if k := plugins.KeyForTest(e, "tailscale"); k == "" {
		t.Fatal("key not learned")
	}
}

// Approving with every limit box empty sends limits {}; that means no
// limits of the owner's own, not an error.
func TestApproveWithEmptyLimits(t *testing.T) {
	ls, pub := newListServer(t)
	e, _ := newEngine(t, ls, pub)
	m := mux(e)
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)
	for _, body := range []string{`{"folders":{},"limits":{}}`, `{"folders":{},"limits":null}`, `{"folders":{}}`} {
		callBody(t, m, "POST", "/api/v1/plugins/tailscale/approve", body, http.StatusOK)
	}
	callBody(t, m, "POST", "/api/v1/plugins/tailscale/approve", `{"folders":{},"limits":{"cpus":0}}`, http.StatusBadRequest)
}

// A Tailscale installed before 1.3.0 (manifest with no actions) keeps
// forwarding after the upgrade; another plugin gets nothing it didn't ask for.
func TestUpgradeKeepsTailscaleForwarding(t *testing.T) {
	ls, pub := newListServer(t)
	e, run := newEngine(t, ls, pub)
	m := mux(e)
	ls.publishWith("1.0.0", []byte("old-ts"), 2, func(man map[string]any) {
		man["access"] = map[string]any{"hosts": []string{"controlplane.tailscale.com"}, "folders": []string{}, "actions": []string{}}
	})
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)
	if got := e.GrantedScopes("tailscale"); len(got) != 0 {
		t.Fatalf("before the upgrade step: %v", got)
	}
	plugins.KeepForwardingForTest(e)
	plugins.KeepForwardingForTest(e) // once is enough
	if got := e.GrantedScopes("tailscale"); !slices.Equal(got, []string{"network.forward"}) {
		t.Fatalf("after the upgrade step: %v", got)
	}
	// It still starts.
	call(t, m, "POST", "/api/v1/plugins/tailscale/stop", http.StatusOK)
	n := len(run.Starts)
	call(t, m, "POST", "/api/v1/plugins/tailscale/start", http.StatusOK)
	if len(run.Starts) != n+1 {
		t.Fatal("the upgraded plugin did not start")
	}
	call(t, m, "GET", "/api/v1/plugins/tailscale/remote/status", http.StatusOK)
}

// "Pass visits on to HolyArrs" shows only for a forwarding plugin, not for
// every plugin with scopes.
func TestReachesAppOnlyForForwarders(t *testing.T) {
	for _, c := range []struct {
		actions []string
		want    bool
	}{{[]string{"library.read"}, false}, {[]string{"network.forward"}, true}} {
		ls, pub := newListServer(t)
		e, _ := newEngine(t, ls, pub)
		e.Forwards = func(id string) bool { return slices.Contains(e.GrantedScopes(id), "network.forward") }
		m := mux(e)
		ls.publishWith("1.0.0", []byte("reach"+c.actions[0]), 2, func(man map[string]any) {
			man["min_app_version"] = "1.3.0"
			man["access"] = map[string]any{"hosts": []string{"controlplane.tailscale.com"}, "folders": []string{}, "actions": c.actions}
		})
		call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)
		if got := row(t, call(t, m, "GET", "/api/v1/plugins", http.StatusOK), "tailscale")["reaches_app"]; got != c.want {
			t.Errorf("%v: reaches_app %v", c.actions, got)
		}
	}
}
