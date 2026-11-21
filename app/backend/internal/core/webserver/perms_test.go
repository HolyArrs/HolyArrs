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

func TestNewPermsAreOwnerOnlyByDefault(t *testing.T) {
	for _, role := range []string{RoleAdmin, RoleMember} {
		got := DefaultPerms(role)
		if slices.Contains(got, PermYouTube) || slices.Contains(got, PermSaveFiles) {
			t.Fatalf("%s starts with %v", role, got)
		}
	}
	owner := DefaultPerms(RoleOwner)
	if !slices.Contains(owner, PermYouTube) || !slices.Contains(owner, PermSaveFiles) {
		t.Fatalf("owner starts with %v", owner)
	}
	if !(Caller{Role: RoleOwner}).Has(PermYouTube) || !(Caller{Role: RoleOwner}).Has(PermSaveFiles) {
		t.Fatal("the owner lacks the youtube permissions")
	}
	if n := len(AllPerms); AllPerms[n-2] != PermYouTube || AllPerms[n-1] != PermSaveFiles {
		t.Fatalf("AllPerms order %v", AllPerms)
	}
}

func TestShownPermsFollowTheYouTubeFlag(t *testing.T) {
	t.Setenv("HOLYARRS_FLAGS", "")
	if got := ShownPerms(); slices.Contains(got, PermYouTube) || slices.Contains(got, PermSaveFiles) || len(got) != len(AllPerms)-2 {
		t.Fatalf("flag off: %v", got)
	}
	t.Setenv("HOLYARRS_FLAGS", "youtube")
	if got := ShownPerms(); !slices.Equal(got, AllPerms) {
		t.Fatalf("flag on: %v", got)
	}
}

func TestKidNeverHasYouTubeOrSaveFiles(t *testing.T) {
	for _, perms := range [][]Perm{{PermAsk}, {PermAsk, PermYouTube, PermSaveFiles}} {
		kid := Caller{Role: RoleMember, MaxRating: "PG", Perms: perms}
		if kid.Has(PermYouTube) || kid.Has(PermSaveFiles) {
			t.Fatalf("kid with %v has a YouTube permission", perms)
		}
		if eff := kid.Effective(); slices.Contains(eff, PermYouTube) || slices.Contains(eff, PermSaveFiles) || !slices.Contains(eff, PermAsk) {
			t.Fatalf("kid with %v: effective %v", perms, eff)
		}
	}
	adult := Caller{Role: RoleMember, Perms: []Perm{PermYouTube, PermSaveFiles}}
	if !adult.Has(PermYouTube) || !adult.Has(PermSaveFiles) {
		t.Fatal("a ticked member lost the YouTube permissions")
	}
	if !(Caller{Role: RoleOwner}).Has(PermYouTube) {
		t.Fatal("the owner lost youtube")
	}
}
