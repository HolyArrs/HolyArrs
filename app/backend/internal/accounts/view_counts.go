package accounts

import "sync/atomic"

// Viewing counts are per-person counters a page keeps about what it showed
// and what was opened. Accounts stores them under opaque string keys and
// knows nothing of what a key means; the page that owns the words registers
// a check, and its write routes ask ViewKeyOK before counting.

// View count kinds, the first argument of the registered check.
const (
	ViewRow      = "row"
	ViewLayer    = "layer"
	ViewCategory = "category"
)

var viewKeyCheck atomic.Pointer[func(kind, key string) bool]

// SetViewKeyCheck registers the check that says whether key is a known key
// of kind. Without one every key is accepted.
func SetViewKeyCheck(f func(kind, key string) bool) {
	viewKeyCheck.Store(&f)
}

// ViewKeyOK says whether key may be counted under kind.
func ViewKeyOK(kind, key string) bool {
	f := viewKeyCheck.Load()
	return f == nil || (*f)(kind, key)
}
