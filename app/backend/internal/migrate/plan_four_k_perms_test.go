package migrate

import (
	"slices"
	"strings"
	"testing"
)

// Both 4K auto-approve halves together become the 4K permission, so they are
// not also listed as left behind; one half alone still is.
func TestFourKAutoApprovePairIsNotListedAsLeftBehind(t *testing.T) {
	s := &Snapshot{Kind: Seerr, Users: []User{
		{SourceID: 1, Name: "luis", Admin: true, Owner: true, Permissions: []string{"ADMIN"}},
		{SourceID: 2, Name: "Omar", Permissions: []string{"REQUEST", "AUTO_APPROVE_4K_MOVIE", "AUTO_APPROVE_4K_TV"}},
		{SourceID: 3, Name: "Kim", Permissions: []string{"REQUEST", "AUTO_APPROVE_4K_MOVIE"}},
	}}
	p := BuildPlan([]Input{{1, s}}, Choices{})
	if omar := item(t, p, GroupPeople, "person:seerr:2").Person; !slices.Contains(omar.Perms, "auto_approve_four_k") {
		t.Fatalf("Omar perms %v", omar.Perms)
	}
	if kim := item(t, p, GroupPeople, "person:seerr:3").Person; slices.Contains(kim.Perms, "auto_approve_four_k") {
		t.Fatalf("Kim perms %v", kim.Perms)
	}
	var omar, kim bool
	for _, n := range p.NotMoved {
		omar = omar || strings.HasPrefix(n.What, "Omar:")
		kim = kim || n.What == "Kim: AUTO_APPROVE_4K_MOVIE"
	}
	if omar || !kim {
		t.Fatalf("left behind: Omar listed %v, Kim listed %v: %+v", omar, kim, p.NotMoved)
	}
}
