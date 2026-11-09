package accounts

import (
	"context"
	"database/sql"
	"errors"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

func disableOwner(t *testing.T, s *database.Store) {
	t.Helper()
	if err := s.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE users SET state = 'disabled' WHERE role = 'owner'")
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func countRows(t *testing.T, s *database.Store, q string) int {
	t.Helper()
	var n int
	if err := s.Read().QueryRow(q).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestSetupRefusedWhenOwnerDisabled(t *testing.T) {
	f := setup(t, Options{})
	setupOwner(t, f, client(t))
	disableOwner(t, f.s)
	resp, body := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/setup", `{"name":"eve","password":"`+pw+`"}`, nil)
	if resp.StatusCode != 409 {
		t.Fatalf("setup with a disabled owner: %d %s", resp.StatusCode, body)
	}
	if n := countRows(t, f.s, "SELECT count(*) FROM users WHERE role = 'owner'"); n != 1 {
		t.Fatalf("owners %d", n)
	}
}

func TestMakeOwnerTxRefusesSecondOwner(t *testing.T) {
	f := setup(t, Options{})
	setupOwner(t, f, client(t))
	err := f.s.WriteCore(context.Background(), func(tx *sql.Tx) error {
		_, _, _, e := f.a.makeOwnerTx(tx, ProviderPlex, "x", "")
		return e
	})
	if !errors.Is(err, errOwnerExists) {
		t.Fatalf("got %v", err)
	}
	if n := countRows(t, f.s, "SELECT count(*) FROM users WHERE role = 'owner'"); n != 1 {
		t.Fatalf("owners %d", n)
	}
}

func TestOwnerIdentityActiveOnly(t *testing.T) {
	f := setup(t, Options{})
	ctx := context.Background()
	if _, _, err := ownerIdentity(ctx, f.s.Read()); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("empty: %v", err)
	}
	setupOwner(t, f, client(t))
	id, name, err := ownerIdentity(ctx, f.s.Read())
	if err != nil || id == 0 || name != "luis" {
		t.Fatalf("active: %d %q %v", id, name, err)
	}
	disableOwner(t, f.s)
	if _, _, err := ownerIdentity(ctx, f.s.Read()); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("disabled: %v", err)
	}
}

func TestMakeArrKeyReplacesAndDeleteReports(t *testing.T) {
	f := setup(t, Options{})
	setupOwner(t, f, client(t))
	ctx := context.Background()
	id, _, err := ownerIdentity(ctx, f.s.Read())
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/", nil)
	r = r.WithContext(webserver.ContextWith(r.Context(), webserver.Caller{UserID: id, Name: "luis"}))
	if _, err := f.a.MakeArrKey(r, "lidarr"); !errors.Is(err, ErrNoArrSide) {
		t.Fatalf("lidarr: %v", err)
	}
	k1, err := f.a.MakeArrKey(r, "sonarr")
	if err != nil {
		t.Fatal(err)
	}
	k2, err := f.a.MakeArrKey(r, "sonarr")
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(k2) || k1 == k2 {
		t.Fatalf("keys %q %q", k1, k2)
	}
	if _, ok := f.a.checkArrKey(ctx, "sonarr", k1); ok {
		t.Fatal("older key still works")
	}
	if _, ok := f.a.checkArrKey(ctx, "sonarr", k2); !ok {
		t.Fatal("new key refused")
	}
	if _, ok := f.a.checkArrKey(ctx, "radarr", k2); ok {
		t.Fatal("key works on the other side")
	}
	if ok, err := f.a.DeleteArrKey(r, "sonarr"); !ok || err != nil {
		t.Fatalf("delete: %v %v", ok, err)
	}
	if ok, err := f.a.DeleteArrKey(r, "sonarr"); ok || err != nil {
		t.Fatalf("second delete: %v %v", ok, err)
	}
	if _, ok := f.a.checkArrKey(ctx, "sonarr", k2); ok {
		t.Fatal("deleted key still works")
	}
}

func TestViewKeyCheckDefaultAndRegistered(t *testing.T) {
	was := viewKeyCheck.Load()
	t.Cleanup(func() { viewKeyCheck.Store(was) })
	viewKeyCheck.Store(nil)
	if !ViewKeyOK(ViewRow, "anything") {
		t.Fatal("no check should accept every key")
	}
	SetViewKeyCheck(func(k, v string) bool { return k == ViewLayer && v == "genre" })
	if !ViewKeyOK(ViewLayer, "genre") || ViewKeyOK(ViewLayer, "nope") || ViewKeyOK(ViewRow, "genre") {
		t.Fatal("registered check not used per kind")
	}
}

func TestResetOwnerKeepsOtherAdminsArrKeys(t *testing.T) {
	s, err := database.Open(t.TempDir(), database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if err := ResetOwner(ctx, s, "first", "first password"); err != nil {
		t.Fatal(err)
	}
	var admin int64
	if err := s.Write(ctx, func(tx *sql.Tx) error {
		var err error
		admin, err = insertUserTx(tx, &domain.User{Name: "ada", Role: "admin", State: "active", Perms: "[]"}, domain.System)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO compat_keys(side, created_at, created_by, owner_user_id, hash, hint) VALUES ('sonarr', '', '', 1, 'h1', 'abcd')`); err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO compat_keys(side, created_at, created_by, owner_user_id, hash, hint) VALUES ('radarr', '', '', ?, 'h2', 'efgh')`, admin)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := ResetOwner(ctx, s, "second", "second password"); err != nil {
		t.Fatal(err)
	}
	rows, err := s.Read().Query("SELECT side FROM compat_keys")
	if err != nil {
		t.Fatal(err)
	}
	var sides []string
	for rows.Next() {
		var x string
		_ = rows.Scan(&x)
		sides = append(sides, x)
	}
	rows.Close()
	if len(sides) != 1 || sides[0] != "radarr" {
		t.Fatalf("sides left %v", sides)
	}
	evs, _, err := s.Events(ctx, database.EventFilter{Kinds: []string{EventAccountChanged}, Limit: 1})
	if err != nil || len(evs) != 1 {
		t.Fatalf("events %v %v", evs, err)
	}
	if n, _ := evs[0].Data["old_app_keys_removed"].(float64); n != 1 {
		t.Fatalf("old_app_keys_removed %v", evs[0].Data["old_app_keys_removed"])
	}
}
