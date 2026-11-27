package plugins

import "context"

// KeepCycle runs one round of the Keep loop for tests.
func (l *Languages) KeepCycle(ctx context.Context, want []string) error {
	return l.keepCycle(ctx, want)
}

// ClearKeyForTest forgets id's signing key, as an install from before keys
// were kept has none.
func ClearKeyForTest(e *Engine, id string) error { return e.store.SetKey(id, "") }

// KeyForTest is id's recorded signing key.
func KeyForTest(e *Engine, id string) string {
	p, err := e.installed(id)
	if err != nil {
		return ""
	}
	return p.Key
}

// KeepForwardingForTest runs the 1.3.0 upgrade step again.
func KeepForwardingForTest(e *Engine) { e.keepForwarding() }

// MayTokenForTest says whether a start here may get an API token.
func MayTokenForTest(e *Engine) bool { return e.mayToken() }
