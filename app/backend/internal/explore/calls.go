package explore

import (
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Shared says every member of the household asks for the same list: the
// region-wide rows, the cold-start genre rows, and the person and decade
// rows drawn from the household's Library. It is fetched once per refresh
// and shared (plan 5.2: counted apart from the per-member budget of 30).
func (q Request) Shared() bool {
	return q.Scope == ScopeShared
}

// ScopeShared marks a household-wide Request.
const ScopeShared = "shared"

// Outside is how many outside (TMDB) calls one Request costs: similar is
// recommendations plus similar, a person is search plus credits.
func (q Request) Outside() int {
	switch q.List {
	case "similar", "person":
		return 2
	}
	return 1
}

// Calls counts the outside calls of one pool build by type, split into
// region-wide (shared) and member-specific. Cache hits are not calls.
type Calls struct {
	mu     sync.Mutex
	ByType map[string]int
	Shared int
	Member int
}

// Add counts one Request that went out.
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

// String is "member=N shared=M (type=n ...)".
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
