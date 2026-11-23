//go:build linux

package plugins

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// tailscaleHosts is the Tailscale plugin's manifest host list.
func tailscaleHosts(t *testing.T) []string {
	t.Helper()
	file := "../../../plugins/tailscale/holyarrs-plugin.json"
	if f := os.Getenv("HOLYARRS_TS_MANIFEST"); f != "" {
		file = f
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Access struct {
			Hosts []string `json:"hosts"`
		} `json:"access"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m.Access.Hosts
}

// TestTailscaleHostsThroughLock reaches the real Tailscale and Let's Encrypt
// hosts from inside the lock, the way the plugin does, and checks a host off
// its list is refused. It needs the internet, so it runs only with
// HOLYARRS_NET_E2E=1.
func TestTailscaleHostsThroughLock(t *testing.T) {
	if os.Getenv("HOLYARRS_NET_E2E") != "1" {
		t.Skip("set HOLYARRS_NET_E2E=1 to reach the real Tailscale hosts")
	}
	requireNetLock(t)
	_, data, probe := probeDirs(t)
	proxy := &Proxy{}
	h, err := proxy.Register("tailscale", tailscaleHosts(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	run := func(args ...string) error {
		p, err := (ExecRunner{Sandbox: LinuxSandbox{NetLock: true}}).Start(context.Background(),
			Spec{ID: "tailscale", Program: probe, Args: args, DataDir: data, Proxy: h, Env: h.Env("")})
		if err != nil {
			t.Fatal(err)
		}
		return p.Wait()
	}
	for _, u := range []string{
		"https://controlplane.tailscale.com/key?v=71",
		"https://login.tailscale.com/",
		"https://derp1.tailscale.com/derp/probe",
		"https://acme-v02.api.letsencrypt.org/directory",
	} {
		if err := run("httpget", u); err != nil {
			t.Errorf("%s through the lock: %v", u, err)
		}
	}
	for _, c := range [][]string{{"httpget", "https://example.com/"}, {"tcp", "1.1.1.1:443"}, {"udp"}} {
		if err := run(c...); err == nil {
			t.Errorf("%v worked; want refused", c)
		}
	}
}

// TestTailscalePluginStartsLocked builds the real Tailscale plugin, runs it
// in the lock relay-only and waits until Tailscale's control server hands
// it a sign-in link, which proves the control connection through the proxy.
func TestTailscalePluginStartsLocked(t *testing.T) {
	if os.Getenv("HOLYARRS_NET_E2E") != "1" {
		t.Skip("set HOLYARRS_NET_E2E=1 to run the real Tailscale plugin")
	}
	requireNetLock(t)
	root := t.TempDir()
	data := filepath.Join(root, "data")
	ver := filepath.Join(root, "ver")
	os.MkdirAll(data, 0o700)
	os.MkdirAll(ver, 0o700)
	bin := filepath.Join(ver, "run-tailscale")
	if pre := os.Getenv("HOLYARRS_TS_BIN"); pre != "" { // a prebuilt plugin, for images without Go
		b, err := os.ReadFile(pre)
		if err != nil || os.WriteFile(bin, b, 0o700) != nil {
			t.Fatalf("copy prebuilt plugin: %v", err)
		}
	} else {
		build := exec.Command("go", "build", "-o", bin, ".")
		build.Dir = "../../../plugins/tailscale"
		build.Env = append(os.Environ(), "CGO_ENABLED=0")
		if out, err := build.CombinedOutput(); err != nil {
			t.Fatalf("build tailscale plugin: %v\n%s", err, out)
		}
	}
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("app")) }))
	defer app.Close()
	self := strings.TrimPrefix(app.URL, "http://")
	proxy := &Proxy{}
	h, err := proxy.Register("tailscale", tailscaleHosts(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	env := append([]string{"HOLYARRS_PLUGIN_TARGET=" + app.URL, "HOLYARRS_PLUGIN_MARK=0123456789abcdef0123", "HOLYARRS_PLUGIN_NO_UDP=1",
		"HOLYARRS_PLUGIN_DATA=" + data}, h.Env(self)...)
	p, err := (ExecRunner{Sandbox: LinuxSandbox{NetLock: true}}).Start(context.Background(),
		Spec{ID: "tailscale", Program: bin, DataDir: data, Proxy: h, SelfAddr: self, Env: env})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop()
	deadline := time.Now().Add(90 * time.Second)
	var st map[string]any
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		raw, err := os.ReadFile(filepath.Join(data, "status.json"))
		if err != nil {
			continue
		}
		st = nil
		_ = json.Unmarshal(raw, &st)
		if u, _ := st["login_url"].(string); strings.HasPrefix(u, "https://login.tailscale.com/") {
			n, recent := proxy.Blocked("tailscale")
			t.Logf("sign-in link reached through the lock; state %v; %d blocked connections: %+v", st["state"], n, recent)
			return
		}
	}
	n, recent := proxy.Blocked("tailscale")
	t.Fatalf("no sign-in link within 90 s; status %v; blocked %d %+v", st, n, recent)
}
