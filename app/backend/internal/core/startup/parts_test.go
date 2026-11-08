package startup

import (
	"slices"
	"testing"
)

func TestPartsKeepRegistrationOrderAndLabels(t *testing.T) {
	partsMu.Lock()
	saved, savedOwners := partDef, owners
	partDef, owners = nil, map[OwnerKind]map[string]Owner{}
	partsMu.Unlock()
	defer func() { partsMu.Lock(); partDef, owners = saved, savedOwners; partsMu.Unlock() }()

	RegisterPart("b", "Bee")
	RegisterPart("a", "Ay")
	RegisterPart("b", "again")
	if got := Parts(); !slices.Equal(got, []Part{"b", "a"}) {
		t.Fatalf("Parts() = %v", got)
	}
	if Part("b").Label() != "Bee" || Part("zz").Label() != "zz" {
		t.Fatalf("labels %q %q", Part("b").Label(), Part("zz").Label())
	}
	if !PartKnown("a") || PartKnown("zz") {
		t.Fatal("PartKnown is wrong")
	}
}

func TestOwnersAreRegisteredPerKind(t *testing.T) {
	partsMu.Lock()
	savedOwners := owners
	owners = map[OwnerKind]map[string]Owner{}
	partsMu.Unlock()
	defer func() { partsMu.Lock(); owners = savedOwners; partsMu.Unlock() }()

	RegisterOwners(LoopOwners, map[string]Owner{"x": Core})
	RegisterOwners(LoopOwners, map[string]Owner{"y": Media})
	if o, ok := OwnerOf(LoopOwners, "y"); !ok || o != Media {
		t.Fatalf("OwnerOf y = %v %v", o, ok)
	}
	if _, ok := OwnerOf(JobOwners, "x"); ok {
		t.Fatal("kinds must not mix")
	}
	m := OwnersOf(LoopOwners)
	m["z"] = Core
	if _, ok := OwnerOf(LoopOwners, "z"); ok {
		t.Fatal("OwnersOf must return a copy")
	}
}
