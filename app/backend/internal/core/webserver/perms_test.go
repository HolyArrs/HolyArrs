package webserver

import (
	"slices"
	"testing"
)

func TestEffectivePerms(t *testing.T) {
	if got := (Caller{Role: RoleOwner}).Effective(); !slices.Equal(got, AllPerms) {
		t.Fatalf("owner: %v", got)
	}
	m := Caller{Role: "member", Perms: []Perm{PermAsk, "bogus"}}
	if got := m.Effective(); !slices.Equal(got, []Perm{PermAsk}) {
		t.Fatalf("member: %v", got)
	}
	if got := (Caller{Role: "member"}).Effective(); got == nil || len(got) != 0 {
		t.Fatalf("no perms must be empty, not nil: %#v", got)
	}
}
