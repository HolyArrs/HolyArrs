package plugins_test

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/plugins"
)

func hasEnv(env []string, prefix string) bool {
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			return true
		}
	}
	return false
}

func needs(t *testing.T, m http.Handler) []any {
	t.Helper()
	p := row(t, call(t, m, "GET", "/api/v1/plugins", http.StatusOK), "tailscale")
	n, _ := p["needs_approval"].([]any)
	return n
}

// A first install approves the hosts the owner saw and starts with the proxy;
// an update that adds a host is held until approved; one that asks for less
// applies at once.
func TestUpdateThatWidensAccessIsHeld(t *testing.T) {
	ls, pub := newListServer(t)
	e, run := newEngine(t, ls, pub)
	m := mux(e)
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)
	first := run.Starts[len(run.Starts)-1]
	if first.Proxy == nil || !hasEnv(first.Env, "HTTPS_PROXY=http://p:") || !hasEnv(first.Env, "HOLYARRS_PLUGIN_NO_UDP=1") {
		t.Fatalf("first start: proxy %v env %v", first.Proxy, first.Env)
	}
	if n := needs(t, m); len(n) != 0 {
		t.Fatalf("fresh install needs approval: %v", n)
	}

	ls.publishWith("1.0.1", []byte("v101"), 2, func(man map[string]any) {
		man["access"] = map[string]any{"hosts": []string{"controlplane.tailscale.com", "extra.example.com"}, "folders": []string{}, "actions": []string{}}
	})
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusUnprocessableEntity)
	p := row(t, call(t, m, "GET", "/api/v1/plugins", http.StatusOK), "tailscale")
	// The widening update waits; the approved 1.0.0 keeps running.
	if p["running"] != true || p["installed_version"] != "1.0.0" {
		t.Fatalf("widening update replaced the running version: %v", p)
	}
	if n := needs(t, m); len(n) != 1 || n[0] != "host:extra.example.com" {
		t.Fatalf("needs = %v", n)
	}
	if last := run.Starts[len(run.Starts)-1]; strings.Contains(strings.Join(last.Env, " "), "extra.example.com") {
		t.Fatal("the waiting update's access reached a start")
	}
	callBody(t, m, "POST", "/api/v1/plugins/tailscale/approve", `{"folders":{}}`, http.StatusOK)
	if p = row(t, call(t, m, "GET", "/api/v1/plugins", http.StatusOK), "tailscale"); p["running"] != true {
		t.Fatalf("after approval: %v", p)
	}

	ls.publishWith("1.0.2", []byte("v102"), 3, func(man map[string]any) {
		man["access"] = map[string]any{"hosts": []string{"controlplane.tailscale.com"}, "folders": []string{}, "actions": []string{}}
	})
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)
	if p = row(t, call(t, m, "GET", "/api/v1/plugins", http.StatusOK), "tailscale"); p["running"] != true {
		t.Fatalf("narrowing update held: %v", p)
	}
}

func TestFolderApproval(t *testing.T) {
	ls, pub := newListServer(t)
	e, run := newEngine(t, ls, pub)
	m := mux(e)
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)
	ls.publishWith("1.0.1", []byte("v101"), 2, func(man map[string]any) {
		man["access"] = map[string]any{"hosts": []string{"controlplane.tailscale.com"}, "actions": []string{},
			"folders": []any{map[string]any{"name": "music", "mode": "read", "why": "to read tags", "required": true}, "covers"}}
	})
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusUnprocessableEntity)
	if n := needs(t, m); len(n) != 2 || n[0] != "folder:music" || n[1] != "folder:covers" {
		t.Fatalf("needs = %v", n)
	}
	// A required folder must be chosen; system and HolyArrs folders are refused.
	callBody(t, m, "POST", "/api/v1/plugins/tailscale/approve", `{"folders":{}}`, http.StatusUnprocessableEntity)
	cfg := filepath.Dir(e.Dir)
	callBody(t, m, "POST", "/api/v1/plugins/tailscale/approve", `{"folders":{"music":`+jsonStr(cfg)+`}}`, http.StatusUnprocessableEntity)
	callBody(t, m, "POST", "/api/v1/plugins/tailscale/approve", `{"folders":{"other":"/tmp"}}`, http.StatusUnprocessableEntity)
	music := filepath.Join(t.TempDir(), "music")
	os.Mkdir(music, 0o700)
	if runtime.GOOS != "linux" {
		return // the folder is opened only on Linux, where plugins run
	}
	callBody(t, m, "POST", "/api/v1/plugins/tailscale/approve", `{"folders":{"music":`+jsonStr(music)+`}}`, http.StatusOK)
	last := run.Starts[len(run.Starts)-1]
	if len(last.Folders) != 1 || last.Folders[0].Mode != plugins.FolderRead || !hasEnv(last.Env, "HOLYARRS_PLUGIN_FOLDER_MUSIC=") {
		t.Fatalf("start folders %+v env %v", last.Folders, last.Env)
	}
	// Read to read-write is held again.
	ls.publishWith("1.0.2", []byte("v102"), 3, func(man map[string]any) {
		man["access"] = map[string]any{"hosts": []string{"controlplane.tailscale.com"}, "actions": []string{},
			"folders": []any{map[string]any{"name": "music", "mode": "read_write", "required": true}}}
	})
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusUnprocessableEntity)
	if n := needs(t, m); len(n) != 1 || n[0] != "folder:music" {
		t.Fatalf("needs after read_write = %v", n)
	}
	// The folder replaced by another one is refused at start.
	callBody(t, m, "POST", "/api/v1/plugins/tailscale/approve", `{}`, http.StatusOK)
	os.Rename(music, music+".old")
	os.Mkdir(music, 0o700)
	call(t, m, "POST", "/api/v1/plugins/tailscale/stop", http.StatusOK)
	call(t, m, "POST", "/api/v1/plugins/tailscale/start", http.StatusUnprocessableEntity)
}

func TestOwnerHostsRoute(t *testing.T) {
	ls, pub := newListServer(t)
	e, run := newEngine(t, ls, pub)
	m := mux(e)
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)
	callBody(t, m, "PUT", "/api/v1/plugins/tailscale/owner-hosts", `{"hosts":["192.168.1.5:8080"]}`, http.StatusUnprocessableEntity)
	callBody(t, m, "PUT", "/api/v1/plugins/tailscale/owner-hosts", `{"hosts":[`+strings.TrimSuffix(strings.Repeat(`"a.x.com","b.x.com",`, 51), ",")+`]}`, http.StatusUnprocessableEntity)
	callBody(t, m, "PUT", "/api/v1/plugins/tailscale/owner-hosts", `{"hosts":["https://hs.home.arpa:8080/"]}`, http.StatusOK)
	p := row(t, call(t, m, "GET", "/api/v1/plugins", http.StatusOK), "tailscale")
	if oh, _ := p["owner_hosts"].([]any); len(oh) != 1 || oh[0] != "hs.home.arpa:8080" || p["running"] != true {
		t.Fatalf("owner hosts: %v", p)
	}
	if n := len(run.Starts); n < 2 {
		t.Fatalf("saving owner hosts did not restart: %d starts", n)
	}
}

func TestNetLockOptIn(t *testing.T) {
	ls, pub := newListServer(t)
	e, run := newEngine(t, ls, pub)
	e.NoNetLock, e.NetLockReason = true, plugins.FileLockBlocked
	m := mux(e)
	run.Err = plugins.ErrNoNetLock
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusUnprocessableEntity)
	ov := call(t, m, "GET", "/api/v1/plugins", http.StatusOK)
	if ov["no_net_lock"] != true || ov["net_lock_reason"] != "blocked" || ov["no_net_lock_asked"] != false {
		t.Fatalf("overview: %v", ov)
	}
	run.Err = nil
	callBody(t, m, "PUT", "/api/v1/plugins/net-lock", `{"allow_without": true}`, http.StatusOK)
	if last := run.Starts[len(run.Starts)-1]; !last.AllowNoNetLock {
		t.Fatalf("start after opt-in did not carry it: %+v", last)
	}
	if p := row(t, call(t, m, "GET", "/api/v1/plugins", http.StatusOK), "tailscale"); p["running"] != true {
		t.Fatalf("after opt-in: %v", p)
	}
	callBody(t, m, "PUT", "/api/v1/plugins/net-lock", `{"allow_without": false}`, http.StatusOK)
	if p := row(t, call(t, m, "GET", "/api/v1/plugins", http.StatusOK), "tailscale"); p["running"] != false {
		t.Fatalf("after opt-out: %v", p)
	}
	callBody(t, m, "PUT", "/api/v1/plugins/net-lock", `{}`, http.StatusBadRequest)
}

func jsonStr(s string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
}

func TestManifestFolderForms(t *testing.T) {
	base := `{"id":"x","name":"X","version":"1.0.0","author":"a","min_app_version":"1.0.0","sha256":"` + strings.Repeat("a", 64) + `","access":{"hosts":[%H],"folders":[%F],"actions":[]}}`
	mk := func(h, f string) []byte {
		return []byte(strings.NewReplacer("%H", h, "%F", f).Replace(base))
	}
	man, err := plugins.ParseManifest(mk(`"*.tailscale.com"`, `"music",{"name":"downloads","mode":"read_write","why":"save files","required":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if f := man.Access.Folders; len(f) != 2 || f[0].Mode != plugins.FolderRead || f[1].Mode != plugins.FolderReadWrite || !f[1].Required {
		t.Fatalf("folders %+v", f)
	}
	for _, c := range []struct{ h, f string }{
		{`"*.com"`, ``},
		{`"1.2.3.4"`, ``},
		{`"x.com:443"`, ``},
		{`"X.com"`, ``},
		{``, `{"name":"m","mode":"write"}`},
		{``, `{"name":"a/b"}`},
		{``, `"/abs"`},
		{``, `"c:"`},
		{``, `{"name":"m","Mode":"read"}`},
		{``, `"m","M"`},
		{``, `{"name":"m","why":"<b>x</b>"}`},
	} {
		if _, err := plugins.ParseManifest(mk(c.h, c.f)); err == nil {
			t.Errorf("hosts [%s] folders [%s] accepted", c.h, c.f)
		}
	}
	many := strings.TrimSuffix(strings.Repeat(`"a.example.com",`, 101), ",")
	if _, err := plugins.ParseManifest(mk(many, "")); err == nil {
		t.Error("101 hosts accepted")
	}
}
