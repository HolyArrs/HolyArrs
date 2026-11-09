package suggest

import (
	"sort"
	"strconv"
	"strings"
	"sync"
)

// ScopeShared marks a household-wide Request.
const ScopeShared = "shared"

// Calls counts the outside calls of one pool build by type, split into
// shared and member-specific ones. Cache hits are not counted.
type Calls struct {
	mu     sync.Mutex
	ByType map[string]int
	Shared int
	Member int
}

// Shared reports whether every member asks for the same list, so it is
// fetched once per refresh and counted apart from the per-member budget.
func (q Request) Shared() bool {
	return q.Scope == ScopeShared
}

// Outside returns how many outside calls one Request costs.
func (q Request) Outside() int {
	// Similar needs recommendations plus similar; a person needs search plus credits.
	switch q.List {
	case "similar", "person":
		return 2
	}
	return 1
}

// Add counts one Request that went out. A nil Calls ignores it.
func (c *Calls) Add(q Request) {
	if c == nil {
		return
	}
	n := q.Outside()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ByType == nil {
		c.ByType = map[string]int{}
	}
	switch q.List {
	case "similar":
		c.ByType["recommendations"]++
		c.ByType["similar"]++
	default:
		c.ByType[q.List] += n
	}
	if q.Shared() {
		c.Shared += n
	} else {
		c.Member += n
	}
}

// String formats the counts as "member=N shared=M (type=n ...)".
func (c *Calls) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	keys := make([]string, 0, len(c.ByType))
	for k := range c.ByType {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+strconv.Itoa(c.ByType[k]))
	}
	return "member=" + strconv.Itoa(c.Member) + " shared=" + strconv.Itoa(c.Shared) + " (" + strings.Join(parts, " ") + ")"
}
