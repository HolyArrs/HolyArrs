package accounts

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

func TestGroupRoleFromClaims(t *testing.T) {
	roles := map[string]string{"admins": webserver.RoleAdmin, "family": webserver.RoleMember}
	for _, tc := range []struct {
		name  string
		claim string
		roles map[string]string
		raw   map[string]any
		want  string
	}{
		{"no claim name set", "", roles, map[string]any{"groups": []any{"admins"}}, ""},
		{"no map", "groups", nil, map[string]any{"groups": []any{"admins"}}, ""},
		{"claim missing", "groups", roles, map[string]any{"name": "x"}, ""},
		{"claim of the wrong type", "groups", roles, map[string]any{"groups": 7.0}, ""},
		{"string claim", "groups", roles, map[string]any{"groups": "admins"}, webserver.RoleAdmin},
		{"string under roles", "roles", roles, map[string]any{"roles": "admins"}, webserver.RoleAdmin},
		{"string under another name", "team", roles, map[string]any{"team": "admins"}, ""},
		{"array under another name", "team", roles, map[string]any{"team": []any{"admins"}}, webserver.RoleAdmin},
		{"array claim", "groups", roles, map[string]any{"groups": []any{"family"}}, webserver.RoleMember},
		{"unknown group only", "groups", roles, map[string]any{"groups": []any{"strangers"}}, webserver.RoleMember},
		{"empty array", "groups", roles, map[string]any{"groups": []any{}}, webserver.RoleMember},
		{"several groups, highest wins", "groups", roles, map[string]any{"groups": []any{"family", "strangers", "admins"}}, webserver.RoleAdmin},
		{"other claim name", "roles", roles, map[string]any{"roles": []any{"admins"}, "groups": []any{"family"}}, webserver.RoleAdmin},
		{"case kept exact", "groups", roles, map[string]any{"groups": []any{"Admins"}}, webserver.RoleMember},
		{"non-text entries skipped", "groups", roles, map[string]any{"groups": []any{3.0, "admins"}}, webserver.RoleAdmin},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := groupRole(tc.claim, tc.roles, tc.raw); got != tc.want {
				t.Fatalf("groupRole = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseGroupRoles(t *testing.T) {
	for _, tc := range []struct {
		in   string
		ok   bool
		want map[string]string
	}{
		{"", true, map[string]string{}},
		{"admins=admin, family=member", true, map[string]string{"admins": "admin", "family": "member"}},
		{"bosses=owner", false, nil},
		{"admins", false, nil},
		{"=admin", false, nil},
		{"admins=superuser", false, nil},
	} {
		got, err := parseGroupRoles(tc.in)
		if (err == nil) != tc.ok || tc.ok && len(got) != len(tc.want) {
			t.Fatalf("parseGroupRoles(%q) = %v, %v", tc.in, got, err)
		}
		for k, v := range tc.want {
			if got[k] != v {
				t.Fatalf("parseGroupRoles(%q)[%q] = %q", tc.in, k, got[k])
			}
		}
	}
}

func TestGroupRoleAppliedOnSignIn(t *testing.T) {
	const iss = "https://id.example.test"
	f := setup(t, Options{})
	ctx := context.Background()
	setupOwner(t, f, client(t))
	roleOf := func(id int64) string {
		var role, perms string
		if err := f.s.Read().QueryRow("SELECT role, perms FROM users WHERE id = ?", id).Scan(&role, &perms); err != nil {
			t.Fatal(err)
		}
		return role + " " + perms
	}
	// A new person joins with the group's role.
	id, _, err := f.a.ssoIdentityRole(ctx, "oidc", iss, "sub-a", "alex", true, webserver.RoleAdmin)
	if err != nil || roleOf(id) != `admin ["ask","auto_approve","approve","manage_library"]` {
		t.Fatalf("join as admin: %v %s", err, roleOf(id))
	}
	// No claim leaves the role alone.
	if _, _, err := f.a.ssoIdentityRole(ctx, "oidc", iss, "sub-a", "alex", false, ""); err != nil || roleOf(id) != `admin ["ask","auto_approve","approve","manage_library"]` {
		t.Fatalf("missing claim changed the role: %s", roleOf(id))
	}
	// Taken out of the group: back to member, and other sessions end.
	if err := f.s.WriteCore(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO sessions (hash, user_id, csrf, created_at, rotated_at, last_seen_at, expires_at) VALUES ('other-device', ?, 'c', '', '', '', '')", id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.a.ssoIdentityRole(ctx, "oidc", iss, "sub-a", "alex", false, webserver.RoleMember); err != nil || roleOf(id) != `member ["ask"]` {
		t.Fatalf("removed group kept the role: %s", roleOf(id))
	}
	var left int
	_ = f.s.Read().QueryRow("SELECT count(*) FROM sessions WHERE user_id = ?", id).Scan(&left)
	if left != 0 {
		t.Fatalf("demoted person kept %d sessions", left)
	}
	// The owner is never changed by a group.
	var ownerID int64
	if err := f.s.Read().QueryRow("SELECT id FROM users WHERE role = 'owner'").Scan(&ownerID); err != nil {
		t.Fatal(err)
	}
	if err := f.s.WriteCore(ctx, func(tx *sql.Tx) error {
		return insertSSOLink(ctx, tx, "oidc", iss, "sub-owner", ownerID, "owner", "2026-01-01T00:00:00Z")
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.a.ssoIdentityRole(ctx, "oidc", iss, "sub-owner", "owner", false, webserver.RoleMember); err != nil {
		t.Fatal(err)
	}
	if r := roleOf(ownerID); r[:5] != "owner" {
		t.Fatalf("owner demoted: %s", r)
	}
}

// The groups claim is read from the ID token, or from userinfo when the
// token has none, and the groups scope is asked for only when it is used.
func TestOIDCGroupsFromTokenOrUserInfo(t *testing.T) {
	for _, tc := range []struct {
		name               string
		claim              string
		token, info        any
		want, wantScopeHas string
	}{
		{"off", "", []string{"admins"}, nil, "", ""},
		{"array in token", "groups", []string{"family", "admins"}, nil, webserver.RoleAdmin, "groups"},
		{"string in token", "groups", "admins", nil, webserver.RoleAdmin, "groups"},
		{"only in userinfo", "groups", nil, []string{"admins"}, webserver.RoleAdmin, "groups"},
		{"removed from admins", "groups", []string{"family"}, nil, webserver.RoleMember, "groups"},
		{"missing everywhere", "groups", nil, nil, "", "groups"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeOIDC(t)
			f.groups, f.infoGroups, f.infoSub = tc.token, tc.info, "subject-1"
			opt := f.options()
			opt.GroupsClaim, opt.GroupRoles = tc.claim, map[string]string{"admins": "admin", "family": "member"}
			r, err := newOIDCRuntime(opt)
			if err != nil {
				t.Fatal(err)
			}
			defer r.close()
			hasGroups := false
			for _, s := range oidcScopes(opt) {
				hasGroups = hasGroups || s == "groups"
			}
			if hasGroups != (tc.wantScopeHas != "") {
				t.Fatalf("scopes %v", oidcScopes(opt))
			}
			claims, err := b4Exchange(t, f, r, "c1")
			if err != nil || claims.role != tc.want {
				t.Fatalf("role %q, want %q (%v)", claims.role, tc.want, err)
			}
		})
	}
}

// route: PUT /api/v1/auth/sso
// The groups claim and the group roles are saved, shown and checked.
func TestSSOSettingsSaveGroupRoles(t *testing.T) {
	f, own, idp := ssoFixture(t)
	save := map[string]any{"on": true, "issuer": idp.srv.URL, "client_id": "holyarrs", "public_client": true,
		"groups_claim": "groups", "group_roles": "family=member, admins=admin"}
	if resp, out := own.do(t, f, "PUT", "/api/v1/auth/sso", ssoBody(t, save)); resp.StatusCode != 200 {
		t.Fatalf("save: %d %s", resp.StatusCode, out)
	}
	if o := f.a.ssoNow().oidc.opt; o.GroupsClaim != "groups" || o.GroupRoles["admins"] != "admin" {
		t.Fatalf("running setup: %+v", o)
	}
	if _, out := own.do(t, f, "GET", "/api/v1/auth/sso", ""); !strings.Contains(out, `"group_roles":"admins=admin, family=member"`) || !strings.Contains(out, `"groups_claim":"groups"`) {
		t.Fatalf("view: %s", out)
	}
	for _, bad := range []map[string]any{{"group_roles": "bosses=owner"}, {"group_roles": "admins"}, {"groups_claim": "two words"}, {"groups_claim": "preferred_username"}, {"groups_claim": "Email"}} {
		body := map[string]any{"on": true, "issuer": idp.srv.URL, "client_id": "holyarrs", "public_client": true}
		for k, v := range bad {
			body[k] = v
		}
		if resp, out := own.do(t, f, "PUT", "/api/v1/auth/sso", ssoBody(t, body)); resp.StatusCode != 400 || !strings.Contains(out, "group_roles") {
			t.Fatalf("%v accepted: %d %s", bad, resp.StatusCode, out)
		}
	}
	if o := f.a.ssoNow().oidc.opt; o.GroupRoles["admins"] != "admin" {
		t.Fatal("a refused save changed the running setup")
	}
}
