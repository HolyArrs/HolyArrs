package startup

import (
	"maps"
	"sync"
)

// Part is one switchable part of the app. Modules register their parts here;
// the registry knows no part by name.
type Part string

// Owner says who a route, screen, loop or job kind belongs to: one part, the
// core, the media parts together, or the title the request is about.
type Owner string

const (
	// Core is always on.
	Core Owner = "core"
	// Media is on while any media part is on.
	Media Owner = "media"
	// ByTitle is decided by the title the request or job is about.
	ByTitle Owner = "title"
)

// OwnerKind names what an owner table is about.
type OwnerKind int

// The things that have an owner.
const (
	RouteOwners OwnerKind = iota
	ScreenOwners
	LoopOwners
	JobOwners
)

var (
	partsMu sync.RWMutex
	partDef []partEntry
	owners  = map[OwnerKind]map[string]Owner{}
)

type partEntry struct {
	part  Part
	label string
}

// RegisterPart adds a part with the name shown on screen. Parts keep the
// order they were registered in. Registering a part twice keeps the first.
func RegisterPart(p Part, label string) {
	partsMu.Lock()
	defer partsMu.Unlock()
	for _, e := range partDef {
		if e.part == p {
			return
		}
	}
	partDef = append(partDef, partEntry{p, label})
}

// Parts lists the registered parts in registration order.
func Parts() []Part {
	partsMu.RLock()
	defer partsMu.RUnlock()
	out := make([]Part, len(partDef))
	for i, e := range partDef {
		out[i] = e.part
	}
	return out
}

// PartKnown reports whether p is a registered part.
func PartKnown(p Part) bool {
	partsMu.RLock()
	defer partsMu.RUnlock()
	for _, e := range partDef {
		if e.part == p {
			return true
		}
	}
	return false
}

// Label is the part's name on screen, or its own name when it is not
// registered.
func (p Part) Label() string {
	partsMu.RLock()
	defer partsMu.RUnlock()
	for _, e := range partDef {
		if e.part == p {
			return e.label
		}
	}
	return string(p)
}

// RegisterOwners adds owners for routes, screens, loops or job kinds. A key
// registered again takes the new owner.
func RegisterOwners(kind OwnerKind, m map[string]Owner) {
	partsMu.Lock()
	defer partsMu.Unlock()
	if owners[kind] == nil {
		owners[kind] = map[string]Owner{}
	}
	maps.Copy(owners[kind], m)
}

// OwnerOf is the registered owner of key.
func OwnerOf(kind OwnerKind, key string) (Owner, bool) {
	partsMu.RLock()
	defer partsMu.RUnlock()
	o, ok := owners[kind][key]
	return o, ok
}

// OwnersOf is a copy of every registered owner of one kind.
func OwnersOf(kind OwnerKind) map[string]Owner {
	partsMu.RLock()
	defer partsMu.RUnlock()
	return maps.Clone(owners[kind])
}
