package accounts

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestSetupAccountEdit(t *testing.T) {
	f := gated(t, Options{})
	own := ownerOf(t, f)
	if err := f.s.WriteCore(context.Background(), func(tx *sql.Tx) error {
		_, err := insertUserTx(tx, &domain.User{Name: "ana", Role: "member", State: "active", Perms: "[]"}, domain.System)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	put := func(body string) (*http.Response, string) {
		return own.do(t, f, "PUT", "/api/v1/auth/setup/account", body)
	}
	if resp, out := put(`{"username":"has space","display_name":"L"}`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad username: %d %s", resp.StatusCode, out)
	}
	if resp, out := put(`{"username":"ANA","display_name":"L"}`); resp.StatusCode != http.StatusConflict {
		t.Fatalf("taken username: %d %s", resp.StatusCode, out)
	}
	if resp, out := put(`{"username":"luis","display_name":"L","password":"x"}`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("short password: %d %s", resp.StatusCode, out)
	}
	if resp, out := put(`{"username":"luisb","display_name":" Luis B "}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("edit: %d %s", resp.StatusCode, out)
	}
	if _, prof := own.do(t, f, "GET", "/api/v1/my/profile", ""); !strings.Contains(prof, `"display_name":"Luis B"`) || !strings.Contains(prof, `"username":"luisb"`) {
		t.Fatalf("profile: %s", prof)
	}
	// No password sent keeps the old one.
	if resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/login", `{"name":"luisb","password":"`+pw+`"}`, nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("old password after rename: %d", resp.StatusCode)
	}
	const pw2 = "another long passphrase"
	if resp, out := put(`{"username":"luisb","display_name":"Luis B","password":"` + pw2 + `"}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("password change: %d %s", resp.StatusCode, out)
	}
	if resp, _ := call(t, client(t), "POST", f.srv.URL+"/api/v1/auth/login", `{"name":"luisb","password":"`+pw2+`"}`, nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("new password: %d", resp.StatusCode)
	}
	if resp, out := own.do(t, f, http.MethodPost, "/api/v1/auth/setup/done", ``); resp.StatusCode != http.StatusOK {
		t.Fatalf("setup done: %d %s", resp.StatusCode, out)
	}
	if resp, out := put(`{"username":"luisc","display_name":"L"}`); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("after setup: %d %s", resp.StatusCode, out)
	}
}
