package accounts

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// Unlinking a server during first-run setup undoes the link: the sign-in way
// goes off with no address, the saved media server and the kept key go.
func TestSetupUnlinkClearsWay(t *testing.T) {
	f := gated(t, Options{})
	forgot := []string{}
	f.a.SetMediaLinks(MediaLinks{Forget: func(_ context.Context, kind string) error { forgot = append(forgot, kind); return nil }})
	own := ownerOf(t, f)
	ms := fakeSetupServer(t)
	b, _ := json.Marshal(map[string]string{"url": ms.URL, "name": "boss", "password": "right"})
	if resp, out := own.do(t, f, "POST", "/api/v1/auth/setup/link/jellyfin", string(b)); resp.StatusCode != http.StatusOK {
		t.Fatalf("link: %d %s", resp.StatusCode, out)
	}
	if _, out := own.do(t, f, "GET", "/api/v1/auth/state", ""); !strings.Contains(out, `"jellyfin"`) {
		t.Fatalf("state after link: %s", out)
	}
	h := f.a.setupKeyHold()
	h.mu.Lock()
	h.kind, h.origin, h.key, h.until = ProviderJellyfin, keyOrigin(ms.URL), "k", f.a.now().Add(setupKeyLife)
	h.mu.Unlock()
	if resp, out := own.do(t, f, "DELETE", "/api/v1/auth/media-link/jellyfin", ""); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("unlink: %d %s", resp.StatusCode, out)
	}
	if _, out := own.do(t, f, "GET", "/api/v1/auth/state", ""); strings.Contains(out, `"jellyfin"`) {
		t.Fatalf("state after unlink: %s", out)
	}
	ctx := context.Background()
	if on, url := database.SettingOr(ctx, f.s.Read(), settingsGroup, signInPrefix+"jellyfin_on", ""), database.SettingOr(ctx, f.s.Read(), settingsGroup, signInPrefix+"jellyfin_url", "x"); on != "false" || url != "" {
		t.Fatalf("settings on=%q url=%q", on, url)
	}
	if len(forgot) != 1 || forgot[0] != ProviderJellyfin {
		t.Fatalf("forget: %v", forgot)
	}
	if k, err := f.a.MediaSignInKey(ctx, ProviderJellyfin, ms.URL); err == nil {
		t.Fatalf("key kept: %q", k)
	}
}

// The same for Plex: unlinking during setup turns Plex sign-in off and
// forgets the Plex server saved for it.
func TestSetupPlexUnlinkClearsWay(t *testing.T) {
	f := gated(t, Options{})
	forgot := []string{}
	f.a.SetMediaLinks(MediaLinks{Forget: func(_ context.Context, kind string) error { forgot = append(forgot, kind); return nil }})
	own := ownerOf(t, f)
	ctx := context.Background()
	if err := f.s.WriteCore(ctx, func(tx *sql.Tx) error {
		if err := database.PutSetting(tx, settingsGroup, signInPrefix+"plex_on", "true", domain.System, f.a.now()); err != nil {
			return err
		}
		return database.PutSetting(tx, settingsGroup, signInPrefix+"plex_server_id", "abc", domain.System, f.a.now())
	}); err != nil {
		t.Fatal(err)
	}
	if resp, out := own.do(t, f, "DELETE", "/api/v1/auth/plex/link", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("unlink: %d %s", resp.StatusCode, out)
	}
	if _, out := own.do(t, f, "GET", "/api/v1/auth/state", ""); strings.Contains(out, `"plex"`) {
		t.Fatalf("state after unlink: %s", out)
	}
	if on, id := database.SettingOr(ctx, f.s.Read(), settingsGroup, signInPrefix+"plex_on", ""), database.SettingOr(ctx, f.s.Read(), settingsGroup, signInPrefix+"plex_server_id", "x"); on != "false" || id != "" {
		t.Fatalf("settings on=%q server=%q", on, id)
	}
	if len(forgot) != 1 || forgot[0] != ProviderPlex {
		t.Fatalf("forget: %v", forgot)
	}
}
