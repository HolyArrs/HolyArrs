package accounts

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
)

// TestEmbyHeldNotOfferedAtFirstRun: while Emby sign-in is held back, the
// first-run screen does not offer Emby and the Emby setup call refuses, so
// nobody becomes an owner who could never sign in again. Jellyfin still works.
func TestEmbyHeldNotOfferedAtFirstRun(t *testing.T) {
	t.Setenv("HOLYARRS_FLAGS", strings.Join(slices.DeleteFunc(startup.HeldFeatures(), func(n string) bool { return n == startup.EmbySignIn }), ","))
	ms := fakeSetupServer(t)
	f := gated(t, Options{})
	_, st := call(t, client(t), "GET", f.srv.URL+"/api/v1/auth/state", "", nil)
	if !strings.Contains(st, `"setup_ways":["password","plex","jellyfin"]`) {
		t.Fatalf("state while emby is held: %s", st)
	}
	b, _ := json.Marshal(map[string]string{"url": ms.URL, "name": "boss", "password": "right"})
	if resp, out := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/setup/media-server/emby", string(b), nil); resp.StatusCode != 404 {
		t.Fatalf("emby setup while held: %d %s", resp.StatusCode, out)
	}
	if _, _, ok := f.a.owner(context.Background()); ok {
		t.Fatal("emby setup made the owner while held")
	}
	if resp, out := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/setup/media-server/jellyfin", string(b), nil); resp.StatusCode != 201 {
		t.Fatalf("jellyfin setup while emby is held: %d %s", resp.StatusCode, out)
	}
}
