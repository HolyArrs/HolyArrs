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

// Saving files to a device is off for everyone but the owner until the owner
// gives it, and a Kid never holds it, whatever the stored list says.
func TestSaveFilesOffByDefaultAndNeverForAKid(t *testing.T) {
	for _, role := range []string{RoleAdmin, RoleMember} {
		if slices.Contains(DefaultPerms(role), PermSaveFiles) {
			t.Fatalf("%s starts with save_files", role)
		}
	}
	if !(Caller{Role: RoleOwner}).Has(PermSaveFiles) {
		t.Fatal("the owner may save files")
	}
	adult := Caller{Role: RoleMember, Perms: []Perm{PermAsk, PermSaveFiles}}
	if !adult.Has(PermSaveFiles) {
		t.Fatal("a member given save_files may save files")
	}
	kid := Caller{Role: RoleMember, MaxRating: "PG", Perms: []Perm{PermAsk, PermSaveFiles}}
	if kid.Has(PermSaveFiles) || slices.Contains(kid.Effective(), PermSaveFiles) || !kid.Has(PermAsk) {
		t.Fatalf("kid: %v", kid.Effective())
	}
}
