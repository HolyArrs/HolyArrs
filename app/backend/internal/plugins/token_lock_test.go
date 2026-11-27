package plugins_test

import (
	"net/http"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/plugins"
)

// a start gets its API token when every lock this system lacks is
// one the owner allowed going without; never when one is missing unasked.
func TestTokenFollowsOwnerLockChoice(t *testing.T) {
	ls, pub := newListServer(t)
	e, _ := newEngine(t, ls, pub)
	call(t, mux(e), "POST", "/api/v1/plugins/tailscale/install", http.StatusOK)
	if !plugins.MayTokenForTest(e) {
		t.Fatal("fully locked: no token")
	}
	e.NoFileLock = true
	if plugins.MayTokenForTest(e) {
		t.Fatal("no file lock, not allowed: token given")
	}
	if err := e.SetAllowNoFileLock(true); err != nil {
		t.Fatal(err)
	}
	if !plugins.MayTokenForTest(e) {
		t.Fatal("no file lock, owner allowed: no token")
	}
	e.NoNetLock = true
	if plugins.MayTokenForTest(e) {
		t.Fatal("no net lock, not allowed: token given")
	}
	if err := e.SetAllowNoNetLock(true); err != nil {
		t.Fatal(err)
	}
	if !plugins.MayTokenForTest(e) {
		t.Fatal("both allowed: no token")
	}
	e.Runner = nil
	if plugins.MayTokenForTest(e) {
		t.Fatal("no runner: token given")
	}
}
