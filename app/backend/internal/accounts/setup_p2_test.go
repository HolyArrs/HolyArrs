package accounts

import (
	"context"
	"database/sql"
	"encoding/json"
	"slices"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// setup.Service reads the first-run settings row (this_app/first_run_done) the
// way wizardOpen does here; accounts must write it so they agree. setup imports
// accounts, so the read is copied here instead of importing it.
func TestSetupServiceAgreesWithAccounts(t *testing.T) {
	f := gated(t, Options{})
	ctx := context.Background()
	s := wizardRow{f.s}
	if open, err := s.Open(ctx); err != nil || !open {
		t.Fatalf("fresh install: open=%v err=%v", open, err)
	}
	own := ownerOf(t, f)
	if open, _ := s.Open(ctx); !open {
		t.Fatal("wizard closed once the owner exists but before done")
	}
	if resp, out := own.do(t, f, "POST", "/api/v1/auth/setup/done", ``); resp.StatusCode != 200 {
		t.Fatalf("setup done: %d %s", resp.StatusCode, out)
	}
	if open, err := s.Open(ctx); err != nil || open {
		t.Fatalf("after done: open=%v err=%v", open, err)
	}
}

func TestMakeOwnerTxUniqueName(t *testing.T) {
	f := setup(t, Options{})
	ctx := context.Background()
	var name, role string
	err := f.s.WriteCore(ctx, func(tx *sql.Tx) error {
		for _, n := range []string{"luis", "luis-2"} {
			if _, err := insertUserTx(tx, &domain.User{Name: n, Role: "member", State: "active", Perms: "[]"}, domain.System); err != nil {
				return err
			}
		}
		id, got, _, err := f.a.makeOwnerTx(tx, ProviderPlex, "luis", "")
		if err != nil {
			return err
		}
		name = got
		return tx.QueryRow("SELECT role FROM users WHERE id = ?", id).Scan(&role)
	})
	if err != nil {
		t.Fatal(err)
	}
	if name != "luis-3" || role != "owner" {
		t.Fatalf("name %q role %q", name, role)
	}
}

func TestSetupHomeHostPersistsAfterRestart(t *testing.T) {
	f := gated(t, Options{})
	ctx := context.Background()
	// An earlier saved name stays when setup adds its own.
	if err := f.s.Write(ctx, func(tx *sql.Tx) error {
		return database.PutSetting(tx, settingsGroup, signInPrefix+keyAllowedHosts, `["old.local"]`, domain.System, f.clock.now())
	}); err != nil {
		t.Fatal(err)
	}
	f = gatedOn(t, f.s, Options{})
	if resp, body := setupOn(t, f, client(t), "tower.local", map[string]string{"Origin": "http://tower.local"}); resp.StatusCode != 201 {
		t.Fatalf("setup: %d %s", resp.StatusCode, body)
	}
	var v string
	if err := f.s.Read().QueryRow("SELECT value FROM settings WHERE grp = ? AND key = ?", settingsGroup, signInPrefix+keyAllowedHosts).Scan(&v); err != nil {
		t.Fatal(err)
	}
	var list []string
	if err := json.Unmarshal([]byte(v), &list); err != nil {
		t.Fatalf("stored value %q: %v", v, err)
	}
	if !slices.Contains(list, "tower.local") || !slices.Contains(list, "old.local") {
		t.Fatalf("saved hosts %v", list)
	}
	again := gatedOn(t, f.s, Options{})
	if resp, out := loginOn(t, again, client(t), "tower.local", nil); resp.StatusCode == 403 {
		t.Fatalf("sign-in on the saved name after a restart: %d %s", resp.StatusCode, out)
	}
}

func TestStateListsSetupWaysOnlyBeforeOwner(t *testing.T) {
	f := setup(t, Options{})
	c := client(t)
	state := func() map[string]any {
		t.Helper()
		_, body := call(t, c, "GET", f.srv.URL+"/api/v1/auth/state", "", nil)
		var out map[string]any
		jsonOf(t, body, &out)
		return out
	}
	out := state()
	ways, _ := out["setup_ways"].([]any)
	want := []any{"password", "plex", "jellyfin", "emby"}
	if !slices.Equal(ways, want) {
		t.Fatalf("setup_ways before owner: %v", out["setup_ways"])
	}
	setupOwner(t, f, c)
	if _, ok := state()["setup_ways"]; ok {
		t.Fatal("setup_ways still listed after the owner exists")
	}
}

// wizardRow mirrors setup.Service.Open.
type wizardRow struct{ st *database.Store }

func (w wizardRow) Open(ctx context.Context) (bool, error) {
	v, err := database.Setting(ctx, w.st.Read(), "this_app", "first_run_done")
	return v != "1", err
}
