package plugins_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/plugins"
)

// A start the owner allowed without the file lock is handed its API token:
// without it the plugin runs with approved scopes it cannot use at all.
func TestStartGetsTokenWithoutFileLock(t *testing.T) {
	ls, pub := newListServer(t)
	e, run := newEngine(t, ls, pub)
	e.NoFileLock, e.FileLockReason = true, plugins.FileLockOldKernel
	var issued []string
	e.Token = func(id string, scopes []string) []string {
		issued = append([]string{id}, scopes...)
		return []string{"HOLYARRS_PLUGIN_TARGET=http://127.0.0.1:6262", "HOLYARRS_PLUGIN_TOKEN=hap_test"}
	}
	m := mux(e)
	callBody(t, m, "PUT", "/api/v1/plugins/file-lock", `{"allow_without": true}`, http.StatusOK)
	call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)

	last := run.Starts[len(run.Starts)-1]
	if !last.AllowNoFileLock {
		t.Fatalf("the start did not carry the owner's allowance: %+v", last)
	}
	if !slices.Contains(last.Env, "HOLYARRS_PLUGIN_TOKEN=hap_test") {
		t.Fatalf("no API token without the file lock: %v", last.Env)
	}
	if len(issued) == 0 || issued[0] != "tailscale" || !slices.Contains(issued, "network.forward") {
		t.Fatalf("token issued for %v", issued)
	}
}

// Both lock cards can be answered before the first install: the answers
// are what the starts that come later need, so they have to be kept.
func TestLockAnswersBeforeFirstInstall(t *testing.T) {
	ls, pub := newListServer(t)
	e, _ := newEngine(t, ls, pub)
	m := mux(e)
	callBody(t, m, "PUT", "/api/v1/plugins/file-lock", `{"allow_without": true}`, http.StatusOK)
	callBody(t, m, "PUT", "/api/v1/plugins/net-lock", `{"allow_without": true}`, http.StatusOK)
	ov := call(t, m, "GET", "/api/v1/plugins", http.StatusOK)
	if ov["allow_no_file_lock"] != true || ov["no_file_lock_asked"] != true ||
		ov["allow_no_net_lock"] != true || ov["no_net_lock_asked"] != true {
		t.Fatalf("answers lost before the first install: %v", ov)
	}
}

// Without the owner's allowance a start is refused outright, so nothing
// running ever ends up without a token in silence.
func TestStartStillRefusedWithoutTheAllowance(t *testing.T) {
	ls, pub := newListServer(t)
	e, run := newEngine(t, ls, pub)
	e.NoFileLock, e.FileLockReason = true, plugins.FileLockOldKernel
	var calls int
	e.Token = func(id string, scopes []string) []string {
		calls++
		return []string{"HOLYARRS_PLUGIN_TOKEN=hap_test"}
	}
	run.Err = plugins.ErrNoLandlock // what the real sandbox answers without the opt-in
	m := mux(e)
	body := call(t, m, "POST", "/api/v1/plugins/tailscale/install", http.StatusUnprocessableEntity)
	if body == nil || calls != 0 {
		t.Fatalf("a refused start got a token: %v", body)
	}
	ov := call(t, m, "GET", "/api/v1/plugins", http.StatusOK)
	if ov["allow_no_file_lock"] != false {
		t.Fatalf("the allowance was on without the owner: %v", ov)
	}
	if p := row(t, ov, "tailscale"); !strings.Contains(p["disabled_reason"].(string), "file lock") {
		t.Fatalf("nothing says why the plugin is off: %v", p)
	}
}
