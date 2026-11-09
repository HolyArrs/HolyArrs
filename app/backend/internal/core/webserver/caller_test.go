package webserver

import "testing"

func TestMaySeeAudit(t *testing.T) {
	cases := []struct {
		c    Caller
		want bool
	}{
		{Caller{Role: RoleOwner}, true},
		{Caller{Role: RoleAdmin}, true},
		{Caller{Role: "member", Perms: []Perm{PermSettings}}, true},
		{Caller{Role: "member", Perms: []Perm{PermManageLibrary}}, false},
		{Caller{Role: RoleOwner, Via: "api_key", Scopes: []string{ScopeRead}}, false},
		{Caller{Role: RoleOwner, Via: "api_key", Scopes: []string{ScopeAdmin}}, true},
	}
	for i, tc := range cases {
		if got := tc.c.MaySeeAudit(); got != tc.want {
			t.Errorf("case %d %+v: got %v want %v", i, tc.c, got, tc.want)
		}
	}
}
